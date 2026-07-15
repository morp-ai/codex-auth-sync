package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/codex"
	"github.com/morp-ai/codex-auth-sync/internal/config"
	"github.com/morp-ai/codex-auth-sync/internal/hub"
	"github.com/morp-ai/codex-auth-sync/internal/model"
	"github.com/morp-ai/codex-auth-sync/internal/secure"
)

func TestTwoAgentsSynchronizeAccountAliasAndActiveSelection(t *testing.T) {
	token := "test-api-token-with-enough-characters"
	key, err := secure.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(t.TempDir(), "hub.json")
	store, err := hub.OpenStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	hubServer, err := hub.NewServer(store, token)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(hubServer.Handler())
	defer server.Close()

	homeA := filepath.Join(t.TempDir(), ".codex")
	homeB := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(homeB, "accounts"), 0700); err != nil {
		t.Fatal(err)
	}
	registryB := []byte(`{"schema_version":4,"active_account_key":null,"previous_active_account_key":null,"active_account_activated_at_ms":null,"interval_seconds":60,"accounts":[]}`)
	if err := os.WriteFile(filepath.Join(homeB, "accounts", "registry.json"), registryB, 0600); err != nil {
		t.Fatal(err)
	}
	managerA := codex.NewManager(homeA)
	record := json.RawMessage(`{"account_key":"user::workspace","chatgpt_account_id":"workspace","chatgpt_user_id":"user","email":"test@example.com","alias":"origin","account_name":null,"plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`)
	snapshot := []byte(`{"tokens":{"access_token":"access","id_token":"id"},"last_refresh":"2026-07-15T00:00:00Z"}`)
	if err := managerA.ApplyAccount(model.AccountUpsertPayload{
		AccountKey:    "user::workspace",
		Record:        record,
		Snapshot:      base64.RawStdEncoding.EncodeToString(snapshot),
		AuthFreshness: codex.AuthFreshness(snapshot),
	}); err != nil {
		t.Fatal(err)
	}
	if err := managerA.ApplyActive(model.ActiveSetPayload{AccountKey: "user::workspace", Group: "shared"}); err != nil {
		t.Fatal(err)
	}

	newConfig := func(node, home string) config.Config {
		cfg := config.Default()
		cfg.ServerURL = server.URL
		cfg.APIToken = token
		cfg.SyncKey = key
		cfg.ClusterID = "test"
		cfg.NodeID = node
		cfg.CodexHome = home
		cfg.ActiveScope = "group"
		cfg.ActiveGroup = "shared"
		return cfg
	}
	logger := log.New(io.Discard, "", 0)
	agentA, err := New(newConfig("node-a", homeA), filepath.Join(t.TempDir(), "state-a.json"), logger)
	if err != nil {
		t.Fatal(err)
	}
	agentB, err := New(newConfig("node-b", homeB), filepath.Join(t.TempDir(), "state-b.json"), logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := agentA.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	storedEvents, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"test@example.com", "user::workspace", "access"} {
		if strings.Contains(string(storedEvents), secret) {
			t.Fatalf("hub storage leaked plaintext value %q", secret)
		}
	}
	if _, err := agentB.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stateB, err := codex.ReadLocalState(homeB)
	if err != nil {
		t.Fatal(err)
	}
	if stateB.ActiveAccount != "user::workspace" || stateB.Accounts["user::workspace"].Alias != "origin" {
		t.Fatalf("node B did not receive initial state: %+v", stateB)
	}
	stateA, err := codex.ReadLocalState(homeA)
	if err != nil {
		t.Fatal(err)
	}
	if stateA.RegistrySchema != 3 || stateB.RegistrySchema != 4 {
		t.Fatalf("cross-schema sync changed a local registry schema: schemaA=%d schemaB=%d", stateA.RegistrySchema, stateB.RegistrySchema)
	}

	if err := managerA.ApplyAlias(model.AliasSetPayload{AccountKey: "user::workspace", Alias: "renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentA.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := agentB.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stateB, err = codex.ReadLocalState(homeB)
	if err != nil {
		t.Fatal(err)
	}
	if stateB.Accounts["user::workspace"].Alias != "renamed" {
		t.Fatalf("node B did not receive alias update: %+v", stateB.Accounts["user::workspace"])
	}

	if err := managerA.DeleteAccount(model.AccountDeletePayload{AccountKey: "user::workspace"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentA.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := agentB.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stateB, err = codex.ReadLocalState(homeB)
	if err != nil {
		t.Fatal(err)
	}
	if len(stateB.Accounts) != 0 || stateB.ActiveAccount != "" {
		t.Fatalf("node B did not receive account deletion: %+v", stateB)
	}
}

func TestSchemaTwoAndFourAgentsSynchronizeWithoutChangingLocalSchemas(t *testing.T) {
	token := "test-api-token-with-enough-characters"
	key, err := secure.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	store, err := hub.OpenStore(filepath.Join(t.TempDir(), "hub.json"))
	if err != nil {
		t.Fatal(err)
	}
	hubServer, err := hub.NewServer(store, token)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(hubServer.Handler())
	defer server.Close()

	homeV2 := filepath.Join(t.TempDir(), ".codex")
	homeV4 := filepath.Join(t.TempDir(), ".codex")
	for _, home := range []string{homeV2, homeV4} {
		if err := os.MkdirAll(filepath.Join(home, "accounts"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	legacyRegistry := []byte(`{"version":2,"active_email":"legacy@example.com","accounts":[{"email":"legacy@example.com","alias":"legacy","plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null}]}`)
	if err := os.WriteFile(filepath.Join(homeV2, "accounts", "registry.json"), legacyRegistry, 0600); err != nil {
		t.Fatal(err)
	}
	claims, err := json.Marshal(map[string]interface{}{
		"email": "legacy@example.com",
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": "legacy-account",
			"chatgpt_user_id":    "legacy-user",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	idToken := "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
	snapshot, err := json.Marshal(map[string]interface{}{
		"tokens": map[string]string{
			"access_token": "access",
			"id_token":     idToken,
			"account_id":   "legacy-account",
		},
		"last_refresh": "2026-07-15T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex.LegacyAccountSnapshotPath(homeV2, "legacy@example.com"), snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	registryV4 := []byte(`{"schema_version":4,"active_account_key":null,"previous_active_account_key":null,"active_account_activated_at_ms":null,"interval_seconds":60,"accounts":[]}`)
	if err := os.WriteFile(filepath.Join(homeV4, "accounts", "registry.json"), registryV4, 0600); err != nil {
		t.Fatal(err)
	}

	newConfig := func(node, home string) config.Config {
		cfg := config.Default()
		cfg.ServerURL = server.URL
		cfg.APIToken = token
		cfg.SyncKey = key
		cfg.ClusterID = "schema-compat"
		cfg.NodeID = node
		cfg.CodexHome = home
		cfg.ActiveScope = "group"
		cfg.ActiveGroup = "main"
		return cfg
	}
	logger := log.New(io.Discard, "", 0)
	agentV2, err := New(newConfig("node-v2", homeV2), filepath.Join(t.TempDir(), "state-v2.json"), logger)
	if err != nil {
		t.Fatal(err)
	}
	agentV4, err := New(newConfig("node-v4", homeV4), filepath.Join(t.TempDir(), "state-v4.json"), logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := agentV2.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := agentV4.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stateV4, err := codex.ReadLocalState(homeV4)
	if err != nil {
		t.Fatal(err)
	}
	accountKey := "legacy-user::legacy-account"
	if stateV4.RegistrySchema != 4 || stateV4.Accounts[accountKey].Alias != "legacy" {
		t.Fatalf("schema v4 node did not receive the v2 account: %+v", stateV4)
	}
	managerV4 := codex.NewManager(homeV4)
	if err := managerV4.ApplyAlias(model.AliasSetPayload{AccountKey: accountKey, Alias: "shared"}); err != nil {
		t.Fatal(err)
	}
	if _, err := agentV4.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := agentV2.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stateV2, err := codex.ReadLocalState(homeV2)
	if err != nil {
		t.Fatal(err)
	}
	if stateV2.RegistrySchema != 2 || stateV2.Accounts[accountKey].Alias != "shared" {
		t.Fatalf("schema v2 node did not receive the v4 alias: %+v", stateV2)
	}
	legacyData, err := os.ReadFile(filepath.Join(homeV2, "accounts", "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyData), "account_key") || strings.Contains(string(legacyData), "schema_version") {
		t.Fatalf("schema v2 registry was migrated or polluted: %s", legacyData)
	}
}
