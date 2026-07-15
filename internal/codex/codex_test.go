package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

func testAuthSnapshot(t *testing.T, email, userID, accountID, refreshedAt string) []byte {
	t.Helper()
	claims, err := json.Marshal(map[string]interface{}{
		"email": email,
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": accountID,
			"chatgpt_user_id":    userID,
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
			"account_id":   accountID,
		},
		"last_refresh": refreshedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestAccountSnapshotFileNameMatchesCodexAuthRules(t *testing.T) {
	if got := AccountSnapshotFileName("simple.key"); got != "simple.key.auth.json" {
		t.Fatalf("unexpected simple filename %q", got)
	}
	key := "user::workspace"
	want := base64.RawURLEncoding.EncodeToString([]byte(key)) + ".auth.json"
	if got := AccountSnapshotFileName(key); got != want {
		t.Fatalf("unexpected encoded filename %q, want %q", got, want)
	}
}

func TestManagerAppliesAccountAliasAndActiveState(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	manager := NewManager(home)
	record := json.RawMessage(`{"account_key":"user::workspace","chatgpt_account_id":"workspace","chatgpt_user_id":"user","email":"test@example.com","alias":"local","account_name":null,"plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`)
	snapshot := []byte(`{"tokens":{"access_token":"access","id_token":"id"},"last_refresh":"2026-07-15T00:00:00Z"}`)
	if err := manager.ApplyAccount(model.AccountUpsertPayload{
		AccountKey:    "user::workspace",
		Record:        record,
		Snapshot:      base64.RawStdEncoding.EncodeToString(snapshot),
		AuthFreshness: AuthFreshness(snapshot),
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyAlias(model.AliasSetPayload{AccountKey: "user::workspace", Alias: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyActive(model.ActiveSetPayload{AccountKey: "user::workspace", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	state, err := ReadLocalState(home)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveAccount != "user::workspace" || state.Accounts["user::workspace"].Alias != "shared" {
		t.Fatalf("unexpected state: %+v", state)
	}
	active, err := os.ReadFile(ActiveAuthPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if string(active) != string(snapshot) {
		t.Fatal("active auth file did not receive the synchronized snapshot")
	}
}

func TestOlderAuthSnapshotDoesNotReplaceNewerLocalSnapshot(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	manager := NewManager(home)
	record := json.RawMessage(`{"account_key":"key","chatgpt_account_id":"workspace","chatgpt_user_id":"user","email":"test@example.com","alias":"","account_name":null,"plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`)
	newer := []byte(`{"tokens":{"access_token":"new"},"last_refresh":"2026-07-15T02:00:00Z"}`)
	older := []byte(`{"tokens":{"access_token":"old"},"last_refresh":"2026-07-15T01:00:00Z"}`)
	for _, snapshot := range [][]byte{newer, older} {
		if err := manager.ApplyAccount(model.AccountUpsertPayload{
			AccountKey:    "key",
			Record:        record,
			Snapshot:      base64.RawStdEncoding.EncodeToString(snapshot),
			AuthFreshness: AuthFreshness(snapshot),
		}); err != nil {
			t.Fatal(err)
		}
	}
	stored, err := os.ReadFile(AccountSnapshotPath(home, "key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(newer) {
		t.Fatalf("older auth snapshot replaced newer data: %s", stored)
	}
}

func TestUnknownRemoteFreshnessDoesNotReplaceKnownNewerLocalSnapshot(t *testing.T) {
	local := []byte(`{"tokens":{"access_token":"new"},"last_refresh":"2026-07-15T02:00:00Z"}`)
	remote := []byte(`{"OPENAI_API_KEY":"sk-replacement-without-freshness"}`)
	if ShouldReplaceAuth(local, remote, 0) {
		t.Fatal("an auth snapshot with unknown freshness must not replace a known refreshed snapshot")
	}
}

func TestSchemaThreeRegistryIsPreserved(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	accountsDir := filepath.Join(home, "accounts")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		t.Fatal(err)
	}
	record := `{"account_key":"user::workspace","chatgpt_account_id":"workspace","chatgpt_user_id":"user","email":"test@example.com","alias":"local","account_name":null,"plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`
	registry := `{"schema_version":3,"active_account_key":null,"active_account_activated_at_ms":null,"auto_switch":{"enabled":true,"threshold_5h_percent":7,"threshold_weekly_percent":3},"api":{"usage":false,"account":true},"accounts":[` + record + `]}`
	if err := os.WriteFile(RegistryPath(home), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := testAuthSnapshot(t, "test@example.com", "user", "workspace", "2026-07-15T00:00:00Z")
	if err := os.WriteFile(AccountSnapshotPath(home, "user::workspace"), snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(home)
	if err := manager.ApplyAlias(model.AliasSetPayload{AccountKey: "user::workspace", Alias: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyActive(model.ActiveSetPayload{AccountKey: "user::workspace", Group: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplySettings(model.SettingsSetPayload{IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(RegistryPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["schema_version"]) != "3" {
		t.Fatalf("schema v3 was changed: %s", data)
	}
	if _, ok := root["interval_seconds"]; ok {
		t.Fatalf("schema v3 received a schema v4 interval field: %s", data)
	}
	if _, ok := root["previous_active_account_key"]; ok {
		t.Fatalf("schema v3 received a schema v4 previous-account field: %s", data)
	}
	if !strings.Contains(string(root["auto_switch"]), `"enabled": true`) && !strings.Contains(string(root["auto_switch"]), `"enabled":true`) {
		t.Fatalf("schema v3 auto_switch settings were not preserved: %s", data)
	}
	state, err := ReadLocalState(home)
	if err != nil {
		t.Fatal(err)
	}
	if state.RegistrySchema != 3 || state.SupportsInterval {
		t.Fatalf("unexpected schema v3 state: %+v", state)
	}
}

func TestSchemaTwoRegistryUsesLegacyLayoutWithoutMigration(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	accountsDir := filepath.Join(home, "accounts")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		t.Fatal(err)
	}
	email := "legacy@example.com"
	registry := `{"version":2,"active_email":"legacy@example.com","accounts":[{"email":"legacy@example.com","alias":"old","plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null},{"email":"unresolved@example.com","alias":"untouched","plan":"free","auth_mode":"chatgpt","created_at":2,"last_used_at":null,"last_usage":null,"last_usage_at":null}]}`
	if err := os.WriteFile(RegistryPath(home), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := testAuthSnapshot(t, email, "legacy-user", "legacy-account", "2026-07-15T00:00:00Z")
	if err := os.WriteFile(LegacyAccountSnapshotPath(home, email), snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := ReadLocalState(home)
	if err != nil {
		t.Fatal(err)
	}
	key := "legacy-user::legacy-account"
	if state.RegistrySchema != 2 || state.ActiveAccount != key || state.Accounts[key].Alias != "old" {
		t.Fatalf("schema v2 adapter did not expose the account: %+v", state)
	}
	manager := NewManager(home)
	if err := manager.ApplyAlias(model.AliasSetPayload{AccountKey: key, Alias: "shared"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplySettings(model.SettingsSetPayload{IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	remoteSnapshot := testAuthSnapshot(t, "remote@example.com", "remote-user", "remote-account", "2026-07-15T01:00:00Z")
	remoteRecord := json.RawMessage(`{"account_key":"remote-user::remote-account","chatgpt_account_id":"remote-account","chatgpt_user_id":"remote-user","email":"remote@example.com","alias":"remote","account_name":null,"plan":"plus","auth_mode":"chatgpt","created_at":2,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`)
	if err := manager.ApplyAccount(model.AccountUpsertPayload{
		AccountKey:    "remote-user::remote-account",
		Record:        remoteRecord,
		Snapshot:      base64.RawStdEncoding.EncodeToString(remoteSnapshot),
		AuthFreshness: AuthFreshness(remoteSnapshot),
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(RegistryPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["version"]) != "2" {
		t.Fatalf("schema v2 version field changed: %s", data)
	}
	if _, ok := root["schema_version"]; ok {
		t.Fatalf("schema v2 was migrated unexpectedly: %s", data)
	}
	if _, ok := root["interval_seconds"]; ok {
		t.Fatalf("schema v2 received a newer settings field: %s", data)
	}
	var accounts []map[string]json.RawMessage
	if err := json.Unmarshal(root["accounts"], &accounts); err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 3 || string(accounts[0]["alias"]) != `"shared"` || string(accounts[1]["alias"]) != `"untouched"` {
		t.Fatalf("schema v2 alias was not updated: %s", data)
	}
	for _, account := range accounts {
		for _, field := range []string{"account_key", "chatgpt_account_id", "chatgpt_user_id"} {
			if _, ok := account[field]; ok {
				t.Fatalf("schema adapter field %q leaked into schema v2: %s", field, data)
			}
		}
	}
	if _, err := os.Stat(LegacyAccountSnapshotPath(home, "remote@example.com")); err != nil {
		t.Fatalf("remote account did not use the schema v2 snapshot path: %v", err)
	}
	state, err = ReadLocalState(home)
	if err != nil {
		t.Fatal(err)
	}
	if state.Accounts["remote-user::remote-account"].Alias != "remote" {
		t.Fatalf("remote account was not readable through the schema v2 adapter: %+v", state)
	}
}

func TestSchemaFourRegistryKeepsVersionSpecificSettings(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	accountsDir := filepath.Join(home, "accounts")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		t.Fatal(err)
	}
	registry := `{"schema_version":4,"active_account_key":null,"previous_active_account_key":null,"active_account_activated_at_ms":null,"interval_seconds":60,"accounts":[]}`
	if err := os.WriteFile(RegistryPath(home), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	record := json.RawMessage(`{"account_key":"user::workspace","chatgpt_account_id":"workspace","chatgpt_user_id":"user","email":"test@example.com","alias":"","account_name":null,"plan":"pro","auth_mode":"chatgpt","created_at":1,"last_used_at":null,"last_usage":null,"last_usage_at":null,"last_local_rollout":null}`)
	snapshot := testAuthSnapshot(t, "test@example.com", "user", "workspace", "2026-07-15T00:00:00Z")
	manager := NewManager(home)
	if err := manager.ApplyAccount(model.AccountUpsertPayload{
		AccountKey:    "user::workspace",
		Record:        record,
		Snapshot:      base64.RawStdEncoding.EncodeToString(snapshot),
		AuthFreshness: AuthFreshness(snapshot),
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyActive(model.ActiveSetPayload{AccountKey: "user::workspace", Group: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplySettings(model.SettingsSetPayload{IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	state, err := ReadLocalState(home)
	if err != nil {
		t.Fatal(err)
	}
	if state.RegistrySchema != 4 || !state.SupportsInterval || state.IntervalSeconds != 15 {
		t.Fatalf("unexpected schema v4 state: %+v", state)
	}
	data, err := os.ReadFile(RegistryPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["schema_version"]) != "4" || string(root["interval_seconds"]) != "15" {
		t.Fatalf("schema v4 settings were not preserved: %s", data)
	}
	if _, ok := root["previous_active_account_key"]; !ok {
		t.Fatalf("schema v4 previous-account field is missing: %s", data)
	}
}

func TestUnknownFutureRegistrySchemaIsRejectedWithoutModification(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(home, "accounts"), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"schema_version":5,"accounts":[]}`)
	if err := os.WriteFile(RegistryPath(home), original, 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(home)
	err := manager.ApplySettings(model.SettingsSetPayload{IntervalSeconds: 15})
	if err == nil || !strings.Contains(err.Error(), "schema_version=5") {
		t.Fatalf("expected an unsupported schema error, got %v", err)
	}
	after, readErr := os.ReadFile(RegistryPath(home))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("future schema registry changed after rejection: %s", after)
	}
}
