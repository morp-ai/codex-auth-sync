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
