package codex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

const maxBackups = 5

type Manager struct {
	CodexHome string
	mu        sync.Mutex
}

func NewManager(codexHome string) *Manager {
	return &Manager{CodexHome: codexHome}
}

func (manager *Manager) ApplyAccount(payload model.AccountUpsertPayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := ValidateAccountKey(payload.AccountKey); err != nil {
		return err
	}
	record, err := ParseRecord(payload.Record)
	if err != nil {
		return fmt.Errorf("parse synchronized account record: %w", err)
	}
	recordKey, err := record.String("account_key")
	if err != nil || recordKey != payload.AccountKey {
		return errors.New("synchronized record account_key does not match payload")
	}
	snapshot, err := base64.RawStdEncoding.DecodeString(payload.Snapshot)
	if err != nil {
		return fmt.Errorf("decode synchronized auth snapshot: %w", err)
	}
	if len(snapshot) == 0 || !json.Valid(snapshot) {
		return errors.New("synchronized auth snapshot is not valid JSON")
	}
	if err := manager.ensureAccountsDir(); err != nil {
		return err
	}
	_, currentRegistry, err := loadRegistry(manager.CodexHome)
	if err != nil {
		return err
	}
	if currentRegistry.SchemaVersion == 2 && currentRegistry.UsesLegacyEmailLayout() {
		identity, identityErr := ParseAuthIdentity(snapshot)
		recordEmail, _ := record.String("email")
		if identityErr != nil || identity.AccountKey != payload.AccountKey || !strings.EqualFold(identity.Email, recordEmail) {
			return errors.New("synchronized account cannot be represented safely in schema v2")
		}
	}
	snapshotPath, err := currentRegistry.AccountSnapshotPath(manager.CodexHome, record)
	if err != nil {
		return err
	}
	localSnapshot, readErr := os.ReadFile(snapshotPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	replacedSnapshot := ShouldReplaceAuth(localSnapshot, snapshot, payload.AuthFreshness)
	if replacedSnapshot {
		if err := writeAtomic(snapshotPath, snapshot, 0600, false); err != nil {
			return err
		}
	}
	if err := manager.mutateRegistry(func(registry *Registry) error {
		if registry.SchemaVersion == 2 && registry.UsesLegacyEmailLayout() {
			record.LegacyAdapter = true
		}
		index, existing := registry.Find(payload.AccountKey)
		if existing == nil && registry.SchemaVersion == 2 {
			email, _ := record.String("email")
			index, existing = registry.FindEmail(email)
		}
		if existing == nil {
			registry.Accounts = append(registry.Accounts, record)
			return nil
		}
		merged := mergeAccountRecord(existing, record)
		registry.Accounts[index] = merged
		return nil
	}); err != nil {
		return err
	}
	if replacedSnapshot {
		state, err := ReadLocalState(manager.CodexHome)
		if err == nil && state.ActiveAccount == payload.AccountKey {
			if err := manager.replaceActiveAuth(snapshot); err != nil {
				return err
			}
		}
	}
	return nil
}

func (manager *Manager) ApplyAlias(payload model.AliasSetPayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.mutateRegistry(func(registry *Registry) error {
		_, record := registry.Find(payload.AccountKey)
		if record == nil {
			return fmt.Errorf("account %q is not available locally", payload.AccountKey)
		}
		return record.SetString("alias", payload.Alias)
	})
}

func (manager *Manager) ApplyStatus(payload model.AccountStatusPayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.mutateRegistry(func(registry *Registry) error {
		_, record := registry.Find(payload.AccountKey)
		if record == nil {
			return fmt.Errorf("account %q is not available locally", payload.AccountKey)
		}
		localTime := record.Int64OrZero("last_usage_at")
		if localTime == 0 {
			localTime = record.Int64OrZero("last_used_at")
		}
		if payload.StatusUnixTime > 0 && localTime > payload.StatusUnixTime {
			return nil
		}
		setRawOrNull(record.Fields, "last_used_at", payload.LastUsedAt)
		setRawOrNull(record.Fields, "last_usage", payload.LastUsage)
		setRawOrNull(record.Fields, "last_usage_at", payload.LastUsageAt)
		return nil
	})
}

func (manager *Manager) DeleteAccount(payload model.AccountDeletePayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var newActive string
	var activeWasDeleted bool
	var deletedSnapshotPath string
	var newActiveSnapshotPath string
	if err := manager.mutateRegistry(func(registry *Registry) error {
		index, record := registry.Find(payload.AccountKey)
		if index < 0 {
			return nil
		}
		var err error
		deletedSnapshotPath, err = registry.AccountSnapshotPath(manager.CodexHome, record)
		if err != nil {
			return err
		}
		activeWasDeleted = registry.ActiveAccountKey() == payload.AccountKey
		registry.Accounts = append(registry.Accounts[:index], registry.Accounts[index+1:]...)
		if activeWasDeleted {
			oldActive := payload.AccountKey
			for _, candidate := range registry.Accounts {
				candidateKey, _ := candidate.String("account_key")
				if candidateKey == "" {
					continue
				}
				newActive = candidateKey
				newActiveSnapshotPath, err = registry.AccountSnapshotPath(manager.CodexHome, candidate)
				if err != nil {
					return err
				}
				break
			}
			if registry.SchemaVersion == 2 && registry.UsesLegacyEmailLayout() {
				setOptionalString(registry.Root, "active_email", registry.AccountEmail(newActive))
			} else {
				if registry.SchemaVersion >= 4 {
					setOptionalString(registry.Root, "previous_active_account_key", oldActive)
				}
				setOptionalString(registry.Root, "active_account_key", newActive)
			}
			if newActive == "" {
				if registry.SchemaVersion >= 3 {
					registry.Root["active_account_activated_at_ms"] = json.RawMessage("null")
				}
			} else {
				if registry.SchemaVersion >= 3 {
					registry.Root["active_account_activated_at_ms"] = json.RawMessage(fmt.Sprintf("%d", time.Now().UnixMilli()))
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if deletedSnapshotPath != "" {
		_ = os.Remove(deletedSnapshotPath)
	}
	if activeWasDeleted {
		if newActive == "" {
			return manager.removeActiveAuth()
		}
		snapshot, err := os.ReadFile(newActiveSnapshotPath)
		if err != nil {
			return err
		}
		return manager.replaceActiveAuth(snapshot)
	}
	return nil
}

func (manager *Manager) ApplyActive(payload model.ActiveSetPayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	var snapshot []byte
	if payload.AccountKey != "" {
		_, registry, err := loadRegistry(manager.CodexHome)
		if err != nil {
			return err
		}
		_, record := registry.Find(payload.AccountKey)
		if record == nil {
			return fmt.Errorf("active account %q is not available locally", payload.AccountKey)
		}
		snapshotPath, err := registry.AccountSnapshotPath(manager.CodexHome, record)
		if err != nil {
			return err
		}
		snapshot, err = os.ReadFile(snapshotPath)
		if err != nil {
			return fmt.Errorf("load active account snapshot: %w", err)
		}
	}
	if err := manager.mutateRegistry(func(registry *Registry) error {
		if payload.AccountKey != "" {
			if _, record := registry.Find(payload.AccountKey); record == nil {
				return fmt.Errorf("active account %q is not available locally", payload.AccountKey)
			}
		}
		current := registry.ActiveAccountKey()
		if current == payload.AccountKey {
			return nil
		}
		if registry.SchemaVersion == 2 && registry.UsesLegacyEmailLayout() {
			setOptionalString(registry.Root, "active_email", registry.AccountEmail(payload.AccountKey))
		} else {
			if registry.SchemaVersion >= 4 {
				setOptionalString(registry.Root, "previous_active_account_key", current)
			}
			setOptionalString(registry.Root, "active_account_key", payload.AccountKey)
		}
		if payload.AccountKey == "" {
			if registry.SchemaVersion >= 3 {
				registry.Root["active_account_activated_at_ms"] = json.RawMessage("null")
			}
		} else {
			if registry.SchemaVersion >= 3 {
				registry.Root["active_account_activated_at_ms"] = json.RawMessage(fmt.Sprintf("%d", time.Now().UnixMilli()))
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if payload.AccountKey == "" {
		return manager.removeActiveAuth()
	}
	return manager.replaceActiveAuth(snapshot)
}

func (manager *Manager) ApplySettings(payload model.SettingsSetPayload) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if payload.IntervalSeconds < 5 || payload.IntervalSeconds > 3600 {
		return errors.New("synchronized interval_seconds is outside the supported range")
	}
	return manager.mutateRegistry(func(registry *Registry) error {
		if registry.SchemaVersion < 4 {
			return nil
		}
		registry.Root["interval_seconds"] = json.RawMessage(fmt.Sprintf("%d", payload.IntervalSeconds))
		return nil
	})
}

func (manager *Manager) mutateRegistry(mutate func(*Registry) error) error {
	if err := manager.ensureAccountsDir(); err != nil {
		return err
	}
	path := RegistryPath(manager.CodexHome)
	for attempt := 0; attempt < 5; attempt++ {
		before, registry, err := loadRegistry(manager.CodexHome)
		if err != nil {
			return err
		}
		if err := mutate(registry); err != nil {
			return err
		}
		data, err := registry.Marshal()
		if err != nil {
			return err
		}
		if bytes.Equal(before, data) {
			return nil
		}
		current, err := readOptional(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(before, current) {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if len(before) > 0 {
			if err := manager.backupFile(path, "registry.json"); err != nil {
				return err
			}
		}
		return writeAtomic(path, data, 0600, false)
	}
	return errors.New("registry.json changed repeatedly while applying synchronized state")
}

func (manager *Manager) ensureAccountsDir() error {
	dir := filepath.Join(manager.CodexHome, "accounts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(dir, 0700)
	}
	return nil
}

func (manager *Manager) replaceActiveAuth(data []byte) error {
	path := ActiveAuthPath(manager.CodexHome)
	current, err := readOptional(path)
	if err != nil {
		return err
	}
	if bytes.Equal(current, data) {
		return nil
	}
	if len(current) > 0 {
		if err := manager.backupFile(path, "auth.json"); err != nil {
			return err
		}
	}
	return writeAtomic(path, data, 0600, true)
}

func (manager *Manager) removeActiveAuth() error {
	path := ActiveAuthPath(manager.CodexHome)
	current, err := readOptional(path)
	if err != nil || len(current) == 0 {
		return err
	}
	if err := manager.backupFile(path, "auth.json"); err != nil {
		return err
	}
	return os.Remove(path)
}

func (manager *Manager) backupFile(source, baseName string) error {
	data, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := filepath.Join(manager.CodexHome, "accounts")
	name := fmt.Sprintf("%s.bak.%s.sync", baseName, time.Now().UTC().Format("20060102-150405.000000000"))
	if err := writeAtomic(filepath.Join(dir, name), data, 0600, false); err != nil {
		return err
	}
	return pruneBackups(dir, baseName+".bak.", maxBackups)
}

func mergeAccountRecord(local, remote *Record) *Record {
	merged := remote.Clone()
	merged.LegacyAdapter = local.LegacyAdapter
	for _, name := range []string{"alias", "last_used_at", "last_usage", "last_usage_at", "last_local_rollout"} {
		if value, ok := local.Fields[name]; ok {
			merged.Fields[name] = append(json.RawMessage(nil), value...)
		}
	}
	return merged
}

func setRawOrNull(fields map[string]json.RawMessage, name string, value json.RawMessage) {
	if len(bytes.TrimSpace(value)) == 0 {
		fields[name] = json.RawMessage("null")
		return
	}
	fields[name] = append(json.RawMessage(nil), value...)
}

func setOptionalString(root map[string]json.RawMessage, name, value string) {
	if value == "" {
		root[name] = json.RawMessage("null")
		return
	}
	raw, _ := json.Marshal(value)
	root[name] = raw
}

func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func writeAtomic(path string, data []byte, defaultMode os.FileMode, preserveMode bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	mode := defaultMode
	if preserveMode {
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".sync-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil && runtime.GOOS != "windows" {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		backup := path + ".replace-backup"
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(tempPath, path); err != nil {
			_ = os.Rename(backup, path)
			return err
		}
		_ = os.Remove(backup)
	} else if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, mode)
	}
	return nil
}

func pruneBackups(dir, prefix string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		names = names[1:]
	}
	return nil
}
