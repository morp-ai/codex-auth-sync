package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const ConfigVersion = 1

type Config struct {
	Version               int    `json:"version"`
	Backend               string `json:"backend"`
	ServerURL             string `json:"server_url"`
	APIToken              string `json:"api_token"`
	WebDAVUsername        string `json:"webdav_username,omitempty"`
	WebDAVPassword        string `json:"webdav_password,omitempty"`
	WebDAVBearerToken     string `json:"webdav_bearer_token,omitempty"`
	SyncKey               string `json:"sync_key"`
	ClusterID             string `json:"cluster_id"`
	NodeID                string `json:"node_id"`
	CodexHome             string `json:"codex_home,omitempty"`
	ActiveScope           string `json:"active_scope"`
	ActiveGroup           string `json:"active_group"`
	ScanIntervalSeconds   int    `json:"scan_interval_seconds"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	SyncUsageCache        bool   `json:"sync_usage_cache"`
}

func Default() Config {
	return Config{
		Version:               ConfigVersion,
		Backend:               "hub",
		ClusterID:             "default",
		ActiveScope:           "group",
		ActiveGroup:           "default",
		ScanIntervalSeconds:   5,
		RequestTimeoutSeconds: 30,
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-auth-sync", "config.json"), nil
}

func StatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "state.json")
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	applyEnvironment(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return writePrivateAtomic(path, data)
}

func (cfg Config) Validate() error {
	if cfg.Version != ConfigVersion {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return errors.New("server_url is required")
	}
	switch cfg.Backend {
	case "hub":
		if strings.TrimSpace(cfg.APIToken) == "" {
			return errors.New("api_token is required for the hub backend")
		}
	case "webdav":
		hasBasic := strings.TrimSpace(cfg.WebDAVUsername) != "" && strings.TrimSpace(cfg.WebDAVPassword) != ""
		hasBearer := strings.TrimSpace(cfg.WebDAVBearerToken) != ""
		if !hasBasic && !hasBearer {
			return errors.New("webdav requires username/password or webdav_bearer_token")
		}
	default:
		return errors.New("backend must be hub or webdav")
	}
	if strings.TrimSpace(cfg.SyncKey) == "" {
		return errors.New("sync_key is required")
	}
	if strings.TrimSpace(cfg.ClusterID) == "" {
		return errors.New("cluster_id is required")
	}
	if strings.TrimSpace(cfg.NodeID) == "" {
		return errors.New("node_id is required")
	}
	switch cfg.ActiveScope {
	case "local", "group", "global":
	default:
		return errors.New("active_scope must be local, group, or global")
	}
	if cfg.ActiveScope == "group" && strings.TrimSpace(cfg.ActiveGroup) == "" {
		return errors.New("active_group is required when active_scope is group")
	}
	if cfg.ScanIntervalSeconds < 1 || cfg.ScanIntervalSeconds > 3600 {
		return errors.New("scan_interval_seconds must be between 1 and 3600")
	}
	if cfg.RequestTimeoutSeconds < 1 || cfg.RequestTimeoutSeconds > 300 {
		return errors.New("request_timeout_seconds must be between 1 and 300")
	}
	return nil
}

func (cfg Config) EffectiveActiveGroup() string {
	switch cfg.ActiveScope {
	case "global":
		return "__global__"
	case "group":
		return cfg.ActiveGroup
	default:
		return ""
	}
}

func (cfg Config) ResolveCodexHome() (string, error) {
	if strings.TrimSpace(cfg.CodexHome) != "" {
		return filepath.Clean(expandHome(cfg.CodexHome)), nil
	}
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return filepath.Clean(expandHome(value)), nil
	}
	if runtime.GOOS == "windows" {
		if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
			return filepath.Join(profile, ".codex"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func NewNodeID() (string, error) {
	hostname, _ := os.Hostname()
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "node"
	}
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return sanitizeID(hostname) + "-" + hex.EncodeToString(buf), nil
}

func applyEnvironment(cfg *Config) {
	setString := func(name string, target *string) {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = value
		}
	}
	setString("CODEX_AUTH_SYNC_SERVER_URL", &cfg.ServerURL)
	setString("CODEX_AUTH_SYNC_BACKEND", &cfg.Backend)
	setString("CODEX_AUTH_SYNC_API_TOKEN", &cfg.APIToken)
	setString("CODEX_AUTH_SYNC_WEBDAV_USERNAME", &cfg.WebDAVUsername)
	setString("CODEX_AUTH_SYNC_WEBDAV_PASSWORD", &cfg.WebDAVPassword)
	setString("CODEX_AUTH_SYNC_WEBDAV_BEARER_TOKEN", &cfg.WebDAVBearerToken)
	setString("CODEX_AUTH_SYNC_KEY", &cfg.SyncKey)
	setString("CODEX_AUTH_SYNC_CLUSTER_ID", &cfg.ClusterID)
	setString("CODEX_AUTH_SYNC_NODE_ID", &cfg.NodeID)
	setString("CODEX_AUTH_SYNC_CODEX_HOME", &cfg.CodexHome)
	setString("CODEX_AUTH_SYNC_ACTIVE_SCOPE", &cfg.ActiveScope)
	setString("CODEX_AUTH_SYNC_ACTIVE_GROUP", &cfg.ActiveGroup)
	if value := strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_SCAN_INTERVAL")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.ScanIntervalSeconds = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_USAGE_CACHE")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.SyncUsageCache = parsed
		}
	}
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func sanitizeID(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	return strings.Trim(out.String(), "-")
}

func writePrivateAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil && runtime.GOOS != "windows" {
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
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0600)
	}
	return nil
}
