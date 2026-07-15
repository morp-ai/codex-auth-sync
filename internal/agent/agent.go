package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/backend"
	"github.com/morp-ai/codex-auth-sync/internal/codex"
	"github.com/morp-ai/codex-auth-sync/internal/config"
	"github.com/morp-ai/codex-auth-sync/internal/model"
	"github.com/morp-ai/codex-auth-sync/internal/secure"
)

type Result struct {
	Pulled int
	Pushed int
}

type Agent struct {
	config    config.Config
	statePath string
	key       []byte
	backend   backend.Remote
	manager   *codex.Manager
	state     PersistentState
	logger    *log.Logger
}

func New(cfg config.Config, statePath string, logger *log.Logger) (*Agent, error) {
	key, err := secure.ParseKey(cfg.SyncKey)
	if err != nil {
		return nil, err
	}
	codexHome, err := cfg.ResolveCodexHome()
	if err != nil {
		return nil, err
	}
	var client backend.Remote
	switch cfg.Backend {
	case "hub":
		client, err = backend.New(
			cfg.ServerURL,
			cfg.APIToken,
			time.Duration(cfg.RequestTimeoutSeconds)*time.Second,
			"codex-auth-sync/0.1.0",
		)
	case "webdav":
		client, err = backend.NewWebDAV(
			cfg.ServerURL,
			backend.WebDAVAuth{
				Username:    cfg.WebDAVUsername,
				Password:    cfg.WebDAVPassword,
				BearerToken: cfg.WebDAVBearerToken,
			},
			time.Duration(cfg.RequestTimeoutSeconds)*time.Second,
			"codex-auth-sync/0.1.0",
		)
	default:
		err = fmt.Errorf("unsupported backend %q", cfg.Backend)
	}
	if err != nil {
		return nil, err
	}
	state, err := loadState(statePath)
	if err != nil {
		return nil, fmt.Errorf("load sync state: %w", err)
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Agent{
		config:    cfg,
		statePath: statePath,
		key:       key,
		backend:   client,
		manager:   codex.NewManager(codexHome),
		state:     state,
		logger:    logger,
	}, nil
}

func (agent *Agent) Backend() backend.Remote {
	return agent.backend
}

func (agent *Agent) State() PersistentState {
	return agent.state
}

func (agent *Agent) CodexHome() string {
	return agent.manager.CodexHome
}

func (agent *Agent) SyncOnce(ctx context.Context) (Result, error) {
	result := Result{}
	pulled, err := agent.pull(ctx)
	if err != nil {
		return result, err
	}
	result.Pulled = pulled
	pushed, err := agent.pushLocalChanges(ctx)
	if err != nil {
		return result, err
	}
	result.Pushed = pushed
	agent.state.Initialized = true
	agent.state.LastSuccessfulAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := saveState(agent.statePath, agent.state); err != nil {
		return result, fmt.Errorf("save sync state: %w", err)
	}
	return result, nil
}

func (agent *Agent) Run(ctx context.Context) error {
	interval := time.Duration(agent.config.ScanIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result, err := agent.SyncOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			agent.logger.Printf("Sync failed: %v", err)
		} else if result.Pulled > 0 || result.Pushed > 0 {
			agent.logger.Printf("Sync complete: pulled=%d pushed=%d", result.Pulled, result.Pushed)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (agent *Agent) pull(ctx context.Context) (int, error) {
	total := 0
	for {
		response, err := agent.backend.Pull(ctx, agent.config.ClusterID, agent.state.LastRevision, 500, 0)
		if err != nil {
			return total, fmt.Errorf("pull synchronized events: %w", err)
		}
		for _, event := range response.Events {
			if event.Revision <= agent.state.LastRevision {
				continue
			}
			if err := agent.applyEvent(event); err != nil {
				return total, fmt.Errorf("apply event revision %d: %w", event.Revision, err)
			}
			agent.state.LastRevision = event.Revision
			total++
		}
		if len(response.Events) < 500 {
			if response.LatestRevision > agent.state.LastRevision {
				agent.state.LastRevision = response.LatestRevision
			}
			break
		}
	}
	if total > 0 {
		if err := saveState(agent.statePath, agent.state); err != nil {
			return total, err
		}
	}
	return total, nil
}

func (agent *Agent) applyEvent(event model.Event) error {
	if event.ClusterID != agent.config.ClusterID {
		return errors.New("event cluster does not match local configuration")
	}
	plaintext, err := secure.Decrypt(agent.key, secure.EventAAD(event.ClusterID, event.Kind, event.EntityID), event.Ciphertext)
	if err != nil {
		return err
	}
	if event.NodeID == agent.config.NodeID {
		return nil
	}
	switch event.Kind {
	case model.KindAccountUpsert:
		var payload model.AccountUpsertPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if err := agent.validateAccountEntity(event.EntityID, payload.AccountKey); err != nil {
			return err
		}
		if err := agent.manager.ApplyAccount(payload); err != nil {
			return err
		}
		return agent.observeAccount(payload.AccountKey)
	case model.KindAccountDelete:
		var payload model.AccountDeletePayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if err := agent.validateAccountEntity(event.EntityID, payload.AccountKey); err != nil {
			return err
		}
		if err := agent.manager.DeleteAccount(payload); err != nil {
			return err
		}
		delete(agent.state.Accounts, payload.AccountKey)
		return nil
	case model.KindAliasSet:
		var payload model.AliasSetPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if err := agent.validateAccountEntity(event.EntityID, payload.AccountKey); err != nil {
			return err
		}
		if err := agent.manager.ApplyAlias(payload); err != nil {
			return err
		}
		return agent.observeAccount(payload.AccountKey)
	case model.KindAccountStatus:
		var payload model.AccountStatusPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if err := agent.validateAccountEntity(event.EntityID, payload.AccountKey); err != nil {
			return err
		}
		if agent.config.SyncUsageCache {
			if err := agent.manager.ApplyStatus(payload); err != nil {
				return err
			}
		}
		return agent.observeAccount(payload.AccountKey)
	case model.KindActiveSet:
		var payload model.ActiveSetPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if secure.EntityID(agent.key, "active", payload.Group) != event.EntityID {
			return errors.New("active event entity identifier does not match payload")
		}
		if agent.config.ActiveScope == "local" || payload.Group != agent.config.EffectiveActiveGroup() {
			return nil
		}
		if err := agent.manager.ApplyActive(payload); err != nil {
			return err
		}
		agent.state.ActiveAccount = payload.AccountKey
		return nil
	case model.KindSettingsSet:
		var payload model.SettingsSetPayload
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			return err
		}
		if secure.EntityID(agent.key, "settings", "registry") != event.EntityID {
			return errors.New("settings event entity identifier does not match payload")
		}
		if err := agent.manager.ApplySettings(payload); err != nil {
			return err
		}
		agent.state.SettingsHash = strconv.Itoa(payload.IntervalSeconds)
		return nil
	default:
		return fmt.Errorf("unsupported event kind %q", event.Kind)
	}
}

func (agent *Agent) pushLocalChanges(ctx context.Context) (int, error) {
	local, err := codex.ReadLocalState(agent.manager.CodexHome)
	if err != nil {
		return 0, err
	}
	pushed := 0
	keys := codex.CanonicalAccountKeys(local)
	for _, key := range keys {
		account := local.Accounts[key]
		observed, exists := agent.state.Accounts[key]
		originalObserved := observed
		if !exists || observed.RecordHash != account.RecordHash || observed.AuthHash != account.AuthHash {
			if len(account.Snapshot) == 0 {
				agent.logger.Printf("Skipping account %q because its managed auth snapshot is missing", key)
			} else {
				payload := model.AccountUpsertPayload{
					AccountKey:    key,
					Record:        account.Record,
					Snapshot:      base64.RawStdEncoding.EncodeToString(account.Snapshot),
					AuthFreshness: account.AuthFreshness,
				}
				if err := agent.pushPayload(ctx, model.KindAccountUpsert, secure.EntityID(agent.key, "account", key), payload); err != nil {
					return pushed, err
				}
				pushed++
				observed.RecordHash = account.RecordHash
				observed.AuthHash = account.AuthHash
			}
		}
		if !exists {
			observed.Alias = account.Alias
			observed.StatusHash = account.StatusHash
		} else {
			if originalObserved.Alias != account.Alias {
				payload := model.AliasSetPayload{AccountKey: key, Alias: account.Alias}
				if err := agent.pushPayload(ctx, model.KindAliasSet, secure.EntityID(agent.key, "account", key), payload); err != nil {
					return pushed, err
				}
				pushed++
				observed.Alias = account.Alias
			}
			if agent.config.SyncUsageCache && originalObserved.StatusHash != account.StatusHash {
				payload, err := statusPayload(account)
				if err != nil {
					return pushed, err
				}
				if err := agent.pushPayload(ctx, model.KindAccountStatus, secure.EntityID(agent.key, "account", key), payload); err != nil {
					return pushed, err
				}
				pushed++
				observed.StatusHash = account.StatusHash
			}
		}
		agent.state.Accounts[key] = observed
	}

	if agent.state.Initialized {
		deleted := make([]string, 0)
		for key := range agent.state.Accounts {
			if _, ok := local.Accounts[key]; !ok {
				deleted = append(deleted, key)
			}
		}
		sort.Strings(deleted)
		for _, key := range deleted {
			payload := model.AccountDeletePayload{AccountKey: key}
			if err := agent.pushPayload(ctx, model.KindAccountDelete, secure.EntityID(agent.key, "account", key), payload); err != nil {
				return pushed, err
			}
			delete(agent.state.Accounts, key)
			pushed++
		}
	}

	if agent.config.ActiveScope != "local" && agent.state.ActiveAccount != local.ActiveAccount {
		group := agent.config.EffectiveActiveGroup()
		payload := model.ActiveSetPayload{AccountKey: local.ActiveAccount, Group: group}
		if err := agent.pushPayload(ctx, model.KindActiveSet, secure.EntityID(agent.key, "active", group), payload); err != nil {
			return pushed, err
		}
		agent.state.ActiveAccount = local.ActiveAccount
		pushed++
	}
	settingsHash := strconv.Itoa(local.IntervalSeconds)
	if agent.state.SettingsHash != settingsHash {
		payload := model.SettingsSetPayload{IntervalSeconds: local.IntervalSeconds}
		if err := agent.pushPayload(ctx, model.KindSettingsSet, secure.EntityID(agent.key, "settings", "registry"), payload); err != nil {
			return pushed, err
		}
		agent.state.SettingsHash = settingsHash
		pushed++
	}
	return pushed, nil
}

func (agent *Agent) pushPayload(ctx context.Context, kind, entityID string, payload interface{}) error {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ciphertext, err := secure.Encrypt(agent.key, secure.EventAAD(agent.config.ClusterID, kind, entityID), plaintext)
	if err != nil {
		return err
	}
	eventID, err := secure.RandomID()
	if err != nil {
		return err
	}
	_, err = agent.backend.Push(ctx, model.Event{
		EventID:    eventID,
		ClusterID:  agent.config.ClusterID,
		NodeID:     agent.config.NodeID,
		Kind:       kind,
		EntityID:   entityID,
		Ciphertext: ciphertext,
	})
	if err != nil {
		return fmt.Errorf("push %s event: %w", kind, err)
	}
	return nil
}

func (agent *Agent) observeAccount(accountKey string) error {
	local, err := codex.ReadLocalState(agent.manager.CodexHome)
	if err != nil {
		return err
	}
	account, ok := local.Accounts[accountKey]
	if !ok {
		delete(agent.state.Accounts, accountKey)
		return nil
	}
	agent.state.Accounts[accountKey] = ObservedAccount{
		RecordHash: account.RecordHash,
		AuthHash:   account.AuthHash,
		Alias:      account.Alias,
		StatusHash: account.StatusHash,
	}
	return nil
}

func (agent *Agent) validateAccountEntity(entityID, accountKey string) error {
	if err := codex.ValidateAccountKey(accountKey); err != nil {
		return err
	}
	if secure.EntityID(agent.key, "account", accountKey) != entityID {
		return errors.New("account event entity identifier does not match payload")
	}
	return nil
}

func statusPayload(account codex.LocalAccount) (model.AccountStatusPayload, error) {
	record, err := codex.ParseRecord(account.Record)
	if err != nil {
		return model.AccountStatusPayload{}, err
	}
	value := func(name string) json.RawMessage {
		if raw, ok := record.Fields[name]; ok {
			return append(json.RawMessage(nil), raw...)
		}
		return json.RawMessage("null")
	}
	return model.AccountStatusPayload{
		AccountKey:     account.AccountKey,
		LastUsedAt:     value("last_used_at"),
		LastUsage:      value("last_usage"),
		LastUsageAt:    value("last_usage_at"),
		StatusUnixTime: account.StatusUnixTime,
	}, nil
}
