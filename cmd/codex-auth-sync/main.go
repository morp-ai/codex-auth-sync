package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/morp-ai/codex-auth-sync/internal/agent"
	"github.com/morp-ai/codex-auth-sync/internal/codex"
	"github.com/morp-ai/codex-auth-sync/internal/config"
	"github.com/morp-ai/codex-auth-sync/internal/hub"
	"github.com/morp-ai/codex-auth-sync/internal/secure"
)

var version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		printHelp()
		return nil
	case "version", "--version", "-V":
		fmt.Printf("codex-auth-sync %s\n", version)
		return nil
	case "keygen":
		return runKeygen(args[1:])
	case "init":
		return runInit(args[1:])
	case "sync":
		return runSync(args[1:])
	case "daemon":
		return runDaemon(args[1:])
	case "status":
		return runStatus(args[1:])
	case "serve":
		return runServe(args[1:])
	default:
		return fmt.Errorf("unknown command %q; run codex-auth-sync help", args[0])
	}
}

func runKeygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	asJSON := flags.Bool("json", false, "Print generated secrets as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("keygen does not accept positional arguments")
	}
	apiToken, err := secure.GenerateToken(32)
	if err != nil {
		return err
	}
	syncKey, err := secure.GenerateKey()
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"api_token": apiToken, "sync_key": syncKey})
	}
	fmt.Printf("API token: %s\n", apiToken)
	fmt.Printf("Sync key:  %s\n", syncKey)
	fmt.Println("Store both values securely. Every node in the same cluster needs the same sync key.")
	return nil
}

func runInit(args []string) error {
	defaultPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", defaultPath, "Configuration file path")
	backendType := flags.String("backend", environmentOrDefault("CODEX_AUTH_SYNC_BACKEND", "hub"), "Synchronization backend: hub or webdav")
	serverURL := flags.String("server", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_SERVER_URL")), "Sync server base URL")
	apiToken := flags.String("api-token", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_API_TOKEN")), "Sync server API token")
	webdavUsername := flags.String("webdav-username", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_WEBDAV_USERNAME")), "WebDAV username")
	webdavPassword := flags.String("webdav-password", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_WEBDAV_PASSWORD")), "WebDAV password or app password")
	webdavBearerToken := flags.String("webdav-bearer-token", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_WEBDAV_BEARER_TOKEN")), "WebDAV bearer token")
	syncKey := flags.String("sync-key", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_KEY")), "Base64 end-to-end encryption key")
	clusterID := flags.String("cluster", "default", "Synchronization cluster identifier")
	nodeID := flags.String("node-id", "", "Unique node identifier")
	codexHome := flags.String("codex-home", "", "Codex home directory; defaults to CODEX_HOME or ~/.codex")
	activeScope := flags.String("active-scope", "group", "Active account synchronization scope: local, group, or global")
	activeGroup := flags.String("active-group", "default", "Active account group name")
	interval := flags.Int("interval", 5, "Local scan interval in seconds")
	syncUsage := flags.Bool("sync-usage-cache", false, "Synchronize cached usage values")
	force := flags.Bool("force", false, "Replace an existing configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("init does not accept positional arguments")
	}
	if !*force {
		if _, err := os.Stat(*configPath); err == nil {
			return fmt.Errorf("configuration already exists at %s; use --force to replace it", *configPath)
		}
	}
	if *nodeID == "" {
		*nodeID, err = config.NewNodeID()
		if err != nil {
			return err
		}
	}
	cfg := config.Default()
	cfg.Backend = strings.TrimSpace(*backendType)
	cfg.ServerURL = strings.TrimSpace(*serverURL)
	cfg.APIToken = strings.TrimSpace(*apiToken)
	cfg.WebDAVUsername = strings.TrimSpace(*webdavUsername)
	cfg.WebDAVPassword = strings.TrimSpace(*webdavPassword)
	cfg.WebDAVBearerToken = strings.TrimSpace(*webdavBearerToken)
	cfg.SyncKey = strings.TrimSpace(*syncKey)
	cfg.ClusterID = strings.TrimSpace(*clusterID)
	cfg.NodeID = strings.TrimSpace(*nodeID)
	cfg.CodexHome = strings.TrimSpace(*codexHome)
	cfg.ActiveScope = strings.TrimSpace(*activeScope)
	cfg.ActiveGroup = strings.TrimSpace(*activeGroup)
	cfg.ScanIntervalSeconds = *interval
	cfg.SyncUsageCache = *syncUsage
	if err := config.Save(*configPath, cfg); err != nil {
		return err
	}
	resolvedHome, _ := cfg.ResolveCodexHome()
	fmt.Printf("Configuration written to %s\n", *configPath)
	fmt.Printf("Node ID: %s\n", cfg.NodeID)
	fmt.Printf("Codex home: %s\n", resolvedHome)
	fmt.Println("Run 'codex-auth-sync sync' once, then install the background service.")
	return nil
}

func runSync(args []string) error {
	configPath, err := parseConfigOnly("sync", args)
	if err != nil {
		return err
	}
	cfg, syncAgent, err := loadAgent(configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.RequestTimeoutSeconds+30)*time.Second)
	defer cancel()
	result, err := syncAgent.SyncOnce(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Synchronization complete: pulled=%d pushed=%d\n", result.Pulled, result.Pushed)
	return nil
}

func runDaemon(args []string) error {
	configPath, err := parseConfigOnly("daemon", args)
	if err != nil {
		return err
	}
	_, syncAgent, err := loadAgent(configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	log.Printf("Starting synchronization daemon for %s", syncAgent.CodexHome())
	err = syncAgent.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runStatus(args []string) error {
	configPath, err := parseConfigOnly("status", args)
	if err != nil {
		return err
	}
	cfg, syncAgent, err := loadAgent(configPath)
	if err != nil {
		return err
	}
	local, err := codex.ReadLocalState(syncAgent.CodexHome())
	if err != nil {
		return err
	}
	state := syncAgent.State()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	health, healthErr := syncAgent.Backend().Health(ctx)
	serverStatus := "unreachable"
	if healthErr == nil && health.OK {
		serverStatus = "online"
	}
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Configuration: %s\n", configPath)
	fmt.Printf("Server: %s (%s)\n", cfg.ServerURL, serverStatus)
	fmt.Printf("Backend: %s\n", cfg.Backend)
	fmt.Printf("Cluster: %s\n", cfg.ClusterID)
	fmt.Printf("Node: %s\n", cfg.NodeID)
	fmt.Printf("Codex home: %s\n", syncAgent.CodexHome())
	fmt.Printf("Codex auth registry schema: %d\n", local.RegistrySchema)
	fmt.Printf("Accounts: %d\n", len(local.Accounts))
	if local.ActiveAccount == "" {
		fmt.Println("Active account: none")
	} else {
		fmt.Printf("Active account: %s\n", local.ActiveAccount)
	}
	fmt.Printf("Last remote revision: %d\n", state.LastRevision)
	if state.LastSuccessfulAt == "" {
		fmt.Println("Last successful sync: never")
	} else {
		fmt.Printf("Last successful sync: %s\n", state.LastSuccessfulAt)
	}
	if healthErr != nil {
		return fmt.Errorf("server health check failed: %w", healthErr)
	}
	return nil
}

func runServe(args []string) error {
	defaultData, err := defaultServerDataPath()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	listen := flags.String("listen", "127.0.0.1:8787", "HTTP listen address")
	dataPath := flags.String("data", defaultData, "Persistent event store path")
	apiToken := flags.String("api-token", strings.TrimSpace(os.Getenv("CODEX_AUTH_SYNC_SERVER_TOKEN")), "Server API token")
	tlsCert := flags.String("tls-cert", "", "TLS certificate file")
	tlsKey := flags.String("tls-key", "", "TLS private key file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("serve does not accept positional arguments")
	}
	store, err := hub.OpenStore(*dataPath)
	if err != nil {
		return err
	}
	server, err := hub.NewServer(store, *apiToken)
	if err != nil {
		return err
	}
	if *tlsCert == "" && *tlsKey == "" && !isLoopbackAddress(*listen) {
		log.Println("Warning: the server is listening without TLS; expose it only through a private network such as Tailscale or a TLS reverse proxy")
	}
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	log.Printf("Sync server listening on %s with data at %s", *listen, *dataPath)
	err = hub.RunHTTPServer(ctx, *listen, *tlsCert, *tlsKey, server.Handler())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func parseConfigOnly(command string, args []string) (string, error) {
	defaultPath, err := config.DefaultPath()
	if err != nil {
		return "", err
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", defaultPath, "Configuration file path")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 {
		return "", fmt.Errorf("%s does not accept positional arguments", command)
	}
	return *configPath, nil
}

func loadAgent(configPath string) (config.Config, *agent.Agent, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, nil, err
	}
	syncAgent, err := agent.New(cfg, config.StatePath(configPath), log.Default())
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, syncAgent, nil
}

func defaultServerDataPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "codex-auth-sync", "server-events.json"), nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func environmentOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func printHelp() {
	fmt.Print(`codex-auth-sync securely synchronizes codex-auth accounts between machines.

Usage:
  codex-auth-sync <command> [options]

Commands:
  keygen   Generate an API token and an end-to-end encryption key
  init     Create a node configuration
  sync     Perform one pull-and-push synchronization cycle
  daemon   Continuously synchronize in the background
  status   Show local and remote synchronization status
  serve    Run the lightweight self-hosted synchronization server
  version  Print the program version
  help     Show this help

Run 'codex-auth-sync <command> --help' for command-specific options.
`)
}
