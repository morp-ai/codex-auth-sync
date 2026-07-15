package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebDAVConfigValidation(t *testing.T) {
	cfg := Default()
	cfg.Backend = "webdav"
	cfg.ServerURL = "https://example.test/dav"
	cfg.APIToken = ""
	cfg.WebDAVUsername = "user"
	cfg.WebDAVPassword = "app-password"
	cfg.SyncKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	cfg.NodeID = "node"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAppliesWebDAVCredentialsFromEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{
  "version": 1,
  "backend": "webdav",
  "server_url": "https://example.test/dav",
  "api_token": "",
  "sync_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
  "cluster_id": "test",
  "node_id": "node",
  "active_scope": "local",
  "active_group": "default",
  "scan_interval_seconds": 5,
  "request_timeout_seconds": 30,
  "sync_usage_cache": false
}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_AUTH_SYNC_WEBDAV_USERNAME", "user")
	t.Setenv("CODEX_AUTH_SYNC_WEBDAV_PASSWORD", "password")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WebDAVUsername != "user" || cfg.WebDAVPassword != "password" {
		t.Fatal("environment credentials were not applied")
	}
}
