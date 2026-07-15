package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/morp-ai/codex-auth-sync/internal/model"
)

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
