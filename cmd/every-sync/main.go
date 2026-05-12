package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rain/every-sync/internal/config"
	"github.com/rain/every-sync/internal/engine"
	"github.com/rain/every-sync/internal/logger"
	"github.com/rain/every-sync/internal/provider"
	"github.com/rain/every-sync/internal/provider/local"
	"github.com/rain/every-sync/internal/provider/webdav"
	"github.com/rain/every-sync/internal/server"
	"github.com/rain/every-sync/internal/store"
)

var (
	version    = "dev"
	configPath string
	dataDir    string
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "sync":
		cmdSync(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "provider":
		cmdProvider(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "version":
		fmt.Printf("EverySync %s\n", version)
	case "stop":
		cmdStop(os.Args[2:])
	case "restart":
		cmdRestart(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`EverySync - Cloud storage sync tool

Usage:
  every-sync <command> [options]

Commands:
  serve       Start the sync daemon
  stop        Stop the running daemon
  restart     Restart the running daemon
  sync        Manually trigger sync
  pair        Manage sync pairs
  provider    Manage storage providers
  status      Show sync status
  version     Show version info

Use "every-sync <command> -h" for more info.`)
}

func initLogger() {
	cfg := loadConfig()
	logger.Init(cfg.Log)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "config file path")
	fs.Int("port", 10086, "HTTP server port")
	fs.StringVar(&dataDir, "data-dir", "", "data directory")
	fs.Parse(args)

	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = filepath.Join(homeDir, ".every-sync", "config.yaml")
	}

	initLogger()

	cfg := loadConfig()

	dbPath := cfg.Database.Path
	logPath := cfg.Log.Path
	if dataDir != "" {
		dbPath = filepath.Join(dataDir, "every-sync.db")
		logPath = filepath.Join(dataDir, "logs")
	}

	// Ensure data directory exists
	if dir := filepath.Dir(dbPath); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	if logPath != "" {
		os.MkdirAll(logPath, 0755)
	}

	fmt.Printf("EverySync data directory: %s\n", filepath.Dir(dbPath))
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Printf("Logs: %s\n", logPath)
	s, err := store.Open(dbPath)
	if err != nil {
		logger.L.Fatal().Err(err).Msg("opening database")
	}
	defer s.Close()

	eng := engine.New(s, engineConfigFromAppConfig(cfg, false))

	_ = local.LocalProvider{}
	_ = webdav.WebDAVProvider{}

	// Set up registrar for dynamic pair registration
	eng.WithRegistrar(makePairRegistrar(s, cfg))

	if err := registerAllPairs(eng, s, cfg); err != nil {
		logger.L.Warn().Err(err).Msg("registering pairs")
	}

	for _, pairConf := range cfg.Pairs {
		existing, _ := s.GetSyncPairByName(pairConf.Name)
		if existing == nil {
			pair := &store.SyncPair{
				Name:             pairConf.Name,
				LocalPath:        pairConf.LocalPath,
				RemotePath:       pairConf.RemotePath,
				Provider:         pairConf.Provider,
				Mode:             pairConf.Mode,
				Direction:        pairConf.Direction,
				Enabled:          pairConf.Enabled,
				Schedule:         pairConf.Schedule,
				IncludePatterns:  strings.Join(pairConf.IncludePatterns, "\n"),
				ExcludePatterns:  strings.Join(pairConf.ExcludePatterns, "\n"),
				ConflictStrategy: pairConf.ConflictStrategy,
			}
			if pair.Direction == "" {
				pair.Direction = "both"
			}
			if pair.Mode == "" {
				pair.Mode = "mirror"
			}
			if pair.ConflictStrategy == "" {
				pair.ConflictStrategy = "latest_wins"
			}
			s.CreateSyncPair(pair)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		logger.L.Fatal().Err(err).Msg("starting engine")
	}

	port := fs.Lookup("port").Value.String()
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, port)
	srv := server.New(s, eng, addr, cfg.Log.Path)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			logger.L.Error().Err(err).Msg("server error")
		}
	}()

	// Write PID file after successful startup.
	pFile := pidFilePath()
	if err := writePID(pFile); err != nil {
		logger.L.Fatal().Err(err).Msg("writing PID file")
	}

	<-sigCh
	logger.L.Info().Msg("shutting down")

	removePID(pFile)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	eng.Stop()
	srv.Shutdown(shutdownCtx)
}

func cmdSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	pairName := fs.String("pair", "", "sync pair name (empty = all)")
	direction := fs.String("direction", "", "sync direction: up/down/both")
	materialize := fs.String("materialize", "", "virtual file path to download on demand")
	dryRun := fs.Bool("dry-run", false, "preview mode")
	fs.Parse(args)

	initLogger()

	if *dryRun {
		logger.L.Info().Msg("dry run mode - no changes will be made")
	}

	cfg := loadConfig()
	s, err := store.Open(cfg.Database.Path)
	if err != nil {
		logger.L.Fatal().Err(err).Msg("opening database")
	}
	defer s.Close()

	eng := engine.New(s, engineConfigFromAppConfig(cfg, *dryRun))

	if err := registerAllPairs(eng, s, cfg); err != nil {
		logger.L.Fatal().Err(err).Msg("registering pairs")
	}

	var selectedPair *store.SyncPair
	if *pairName != "" {
		selectedPair, err = s.GetSyncPairByName(*pairName)
		if err != nil || selectedPair == nil {
			logger.L.Fatal().Str("pair", *pairName).Msg("pair not found")
		}
		if !selectedPair.Enabled {
			if err := registerPair(eng, s, cfg, selectedPair); err != nil {
				logger.L.Fatal().Err(err).Str("pair", *pairName).Msg("registering pair")
			}
		}
	}
	if *materialize != "" && selectedPair == nil {
		logger.L.Fatal().Msg("--materialize requires --pair")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		logger.L.Fatal().Err(err).Msg("starting engine")
	}

	if *pairName != "" {
		dir := *direction
		if dir == "" {
			dir = selectedPair.Direction
		}
		if *materialize != "" {
			logger.L.Info().Str("pair", selectedPair.Name).Str("path", *materialize).Msg("materializing virtual file")
			if err := eng.MaterializeVirtual(ctx, selectedPair.ID, *materialize); err != nil {
				logger.L.Fatal().Err(err).Msg("materialize error")
			}
			eng.Stop()
			logger.L.Info().Msg("materialize complete")
			return
		}
		logger.L.Info().Str("pair", selectedPair.Name).Str("direction", dir).Msg("syncing pair")
		if err := eng.SyncPair(ctx, selectedPair.ID, dir); err != nil {
			logger.L.Fatal().Err(err).Msg("sync error")
		}
	} else {
		logger.L.Info().Msg("syncing all pairs")
		if err := eng.SyncAll(ctx); err != nil {
			logger.L.Fatal().Err(err).Msg("sync error")
		}
	}

	eng.Drain(30 * time.Second)
	eng.Stop()

	logger.L.Info().Msg("sync complete")
}

// --- Pair CLI ---

func cmdPair(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage:
  every-sync pair list
  every-sync pair add    --name <name> --local <path> --remote <path> --provider <name> [--direction both] [--mode mirror]
  every-sync pair remove <name|id>
  every-sync pair enable <name|id>
  every-sync pair disable <name|id>`)
		fmt.Println("\nProvider 名称通过 'every-sync provider list' 查看。")
		return
	}

	cfg := loadConfig()
	s, err := store.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	switch args[0] {
	case "list":
		pairs, err := s.ListSyncPairs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(pairs) == 0 {
			fmt.Println("No sync pairs configured")
			return
		}
		fmt.Printf("%-4s %-20s %-10s %-9s %-6s %-12s %-10s %s -> %s\n", "ID", "Name", "Status", "Mode", "Dir", "Conflict", "Provider", "Local", "Remote")
		fmt.Printf("%-4s %-20s %-10s %-9s %-6s %-12s %-10s %s -> %s\n", "--", "----", "------", "----", "---", "--------", "--------", "-----", "------")
		for _, p := range pairs {
			status := "enabled"
			if !p.Enabled {
				status = "disabled"
			}
			fmt.Printf("%-4d %-20s %-10s %-9s %-6s %-12s %-10s %s -> %s\n",
				p.ID, p.Name, status, p.Mode, p.Direction, p.ConflictStrategy, p.Provider, p.LocalPath, p.RemotePath)
		}

	case "add":
		fs := flag.NewFlagSet("pair add", flag.ExitOnError)
		name := fs.String("name", "", "pair name")
		localPath := fs.String("local", "", "local path")
		remotePath := fs.String("remote", "", "remote path")
		prov := fs.String("provider", "", "provider name (use 'every-sync provider list' to see)")
		mode := fs.String("mode", "mirror", "sync mode")
		direction := fs.String("direction", "both", "sync direction")
		include := fs.String("include", "", "selective include patterns, comma or newline separated")
		exclude := fs.String("exclude", "", "selective exclude patterns, comma or newline separated")
		conflictStrategy := fs.String("conflict-strategy", "latest_wins", "conflict strategy: latest_wins/local_wins/remote_wins/manual")
		enable := fs.Bool("enable", false, "enable immediately and sync")
		fs.Parse(args[1:])

		if *name == "" || *localPath == "" || *remotePath == "" || *prov == "" {
			fmt.Fprintln(os.Stderr, "name, local, remote, and provider are required")
			os.Exit(1)
		}
		dir, err := engine.ResolveDirection(*direction, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		pair := &store.SyncPair{
			Name:             *name,
			LocalPath:        *localPath,
			RemotePath:       *remotePath,
			Provider:         *prov,
			Mode:             *mode,
			Direction:        string(dir),
			Enabled:          *enable,
			IncludePatterns:  *include,
			ExcludePatterns:  *exclude,
			ConflictStrategy: *conflictStrategy,
		}
		if err := s.CreateSyncPair(pair); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		status := "disabled"
		if pair.Enabled {
			status = "enabled"
		}
		fmt.Printf("Created sync pair '%s' (id: %d, %s)\n", pair.Name, pair.ID, status)

		if *enable {
			logger.Init(cfg.Log)
			if err := triggerSyncForPair(s, cfg, pair); err != nil {
				fmt.Fprintf(os.Stderr, "Initial sync error: %v\n", err)
			}
		}

	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync pair remove <name|id>")
			os.Exit(1)
		}
		pair := resolvePair(s, args[1])
		if pair == nil {
			fmt.Fprintf(os.Stderr, "Pair '%s' not found\n", args[1])
			os.Exit(1)
		}
		if err := s.DeleteSyncPair(pair.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Pair '%s' deleted\n", pair.Name)

	case "enable":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync pair enable <name|id>")
			os.Exit(1)
		}
		pair := resolvePair(s, args[1])
		if pair == nil {
			fmt.Fprintf(os.Stderr, "Pair '%s' not found\n", args[1])
			os.Exit(1)
		}
		if pair.Enabled {
			fmt.Printf("Pair '%s' is already enabled\n", pair.Name)
			return
		}
		pair.Enabled = true
		if err := s.UpdateSyncPair(pair); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Pair '%s' enabled, starting sync...\n", pair.Name)

		logger.Init(cfg.Log)
		if err := triggerSyncForPair(s, cfg, pair); err != nil {
			fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
		} else {
			fmt.Println("Sync complete")
		}

	case "disable":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync pair disable <name|id>")
			os.Exit(1)
		}
		pair := resolvePair(s, args[1])
		if pair == nil {
			fmt.Fprintf(os.Stderr, "Pair '%s' not found\n", args[1])
			os.Exit(1)
		}
		pair.Enabled = false
		if err := s.UpdateSyncPair(pair); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Pair '%s' disabled\n", pair.Name)

	default:
		fmt.Printf("Unknown pair command: %s\n", args[0])
	}
}

func resolvePair(s *store.Store, identifier string) *store.SyncPair {
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		p, _ := s.GetSyncPair(id)
		return p
	}
	p, _ := s.GetSyncPairByName(identifier)
	return p
}

// --- Provider CLI ---

func cmdProvider(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage:
  every-sync provider list
  every-sync provider add    --name <name> --endpoint <url> [--type webdav] [--username user] [--password pass]
  every-sync provider update <name|id> [--endpoint <url>] [--username user] [--password pass]
  every-sync provider remove <name|id>
  every-sync provider test   <name|id>`)
		return
	}

	cfg := loadConfig()
	s, err := store.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	switch args[0] {
	case "list":
		configs, err := s.ListProviderConfigs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(configs) == 0 {
			fmt.Println("No providers configured.")
			fmt.Println("Use 'every-sync provider add' or edit config.yaml to add one.")
			return
		}

		fmt.Printf("%-4s %-20s %-10s %s\n", "ID", "Name", "Type", "Endpoint")
		fmt.Printf("%-4s %-20s %-10s %s\n", "--", "----", "----", "--------")
		for _, pc := range configs {
			endpoint := pc.Params["endpoint"]
			if endpoint == "" {
				endpoint = "-"
			}
			fmt.Printf("%-4d %-20s %-10s %s\n", pc.ID, pc.Name, pc.Type, endpoint)
		}

	case "add":
		fs := flag.NewFlagSet("provider add", flag.ExitOnError)
		name := fs.String("name", "", "provider name")
		typ := fs.String("type", "webdav", "provider type (webdav)")
		endpoint := fs.String("endpoint", "", "server endpoint")
		username := fs.String("username", "", "username")
		password := fs.String("password", "", "password")
		fs.Parse(args[1:])

		if *name == "" {
			fmt.Fprintln(os.Stderr, "Error: --name is required")
			os.Exit(1)
		}
		if *endpoint == "" {
			fmt.Fprintln(os.Stderr, "Error: --endpoint is required")
			os.Exit(1)
		}

		pc := &store.ProviderConfig{
			Name: *name,
			Type: *typ,
			Params: map[string]string{
				"endpoint": *endpoint,
				"username": *username,
				"password": *password,
			},
		}

		if err := s.CreateProviderConfig(pc); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Provider '%s' created (id: %d, type: %s)\n", pc.Name, pc.ID, pc.Type)

	case "update":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync provider update <name|id> [--endpoint <url>] [--username user] [--password pass]")
			os.Exit(1)
		}
		pc := resolveProvider(s, args[1])
		if pc == nil {
			fmt.Fprintf(os.Stderr, "Provider '%s' not found\n", args[1])
			os.Exit(1)
		}

		fs := flag.NewFlagSet("provider update", flag.ExitOnError)
		endpoint := fs.String("endpoint", "", "server endpoint")
		username := fs.String("username", "", "username")
		password := fs.String("password", "", "password")
		fs.Parse(args[2:])

		if *endpoint != "" {
			pc.Params["endpoint"] = *endpoint
		}
		if *username != "" {
			pc.Params["username"] = *username
		}
		if *password != "" {
			pc.Params["password"] = *password
		}

		if err := s.UpdateProviderConfig(pc); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Provider '%s' updated\n", pc.Name)

	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync provider remove <name|id>")
			os.Exit(1)
		}
		pc := resolveProvider(s, args[1])
		if pc == nil {
			fmt.Fprintf(os.Stderr, "Provider '%s' not found\n", args[1])
			os.Exit(1)
		}
		if err := s.DeleteProviderConfig(pc.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Provider '%s' deleted\n", pc.Name)

	case "test":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: every-sync provider test <name|id>")
			os.Exit(1)
		}
		pc := resolveProvider(s, args[1])
		if pc == nil {
			fmt.Fprintf(os.Stderr, "Provider '%s' not found\n", args[1])
			os.Exit(1)
		}

		_ = local.LocalProvider{}
		_ = webdav.WebDAVProvider{}

		p, ok := provider.Create(pc.Type)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown provider type: %s\n", pc.Type)
			os.Exit(1)
		}

		fmt.Printf("Testing connection to %s (%s)...\n", pc.Name, pc.Params["endpoint"])
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.Init(ctx, provider.Config{Type: pc.Type, Params: pc.Params}); err != nil {
			fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
			os.Exit(1)
		}
		defer p.Close()

		fmt.Println("Connection successful!")

	default:
		fmt.Printf("Unknown provider command: %s\n", args[0])
	}
}

func resolveProvider(s *store.Store, identifier string) *store.ProviderConfig {
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		pc, _ := s.GetProviderConfig(id)
		return pc
	}
	pc, _ := s.GetProviderConfigByName(identifier)
	return pc
}

// --- Status ---

func cmdStatus(args []string) {
	cfg := loadConfig()
	s, err := store.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	pairs, err := s.ListSyncPairs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	provConfigs, _ := s.ListProviderConfigs()
	stats, _ := s.GetSyncStats()

	fmt.Printf("EverySync Status\n")
	fmt.Printf("================\n")
	fmt.Printf("Sync pairs: %d | Providers: %d\n\n", len(pairs), len(provConfigs))
	if stats != nil {
		fmt.Printf("Traffic: uploaded %d B | downloaded %d B | deleted %d | virtual %d | materialized %d | conflicts %d\n\n",
			stats.UploadedBytes, stats.DownloadedBytes, stats.DeletedFiles, stats.VirtualFiles, stats.MaterializedFiles, stats.Conflicts)
	}

	for _, p := range pairs {
		status := "enabled"
		if !p.Enabled {
			status = "disabled"
		}

		entries, _ := s.ListFileEntriesByPair(p.ID)
		pending, _ := s.ListFileEntriesByState(p.ID, "pending")

		fmt.Printf("  [%d] %s (%s)\n", p.ID, p.Name, status)
		fmt.Printf("      Direction: %s | Mode: %s | Provider: %s\n", p.Direction, p.Mode, p.Provider)
		fmt.Printf("      Local: %s -> Remote: %s\n", p.LocalPath, p.RemotePath)
		fmt.Printf("      Files: %d indexed, %d pending\n", len(entries), len(pending))
		fmt.Println()
	}
}

// --- Helpers ---

func loadConfig() *config.Config {
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		configPath = filepath.Join(homeDir, ".every-sync", "config.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v, using defaults\n", err)
		return config.Default()
	}
	return cfg
}

func engineConfigFromAppConfig(cfg *config.Config, dryRun bool) engine.Config {
	uploadLimit, _ := parseByteRate(cfg.Sync.UploadLimit)
	downloadLimit, _ := parseByteRate(cfg.Sync.DownloadLimit)
	chunkSize, _ := parseByteSize(cfg.Sync.ChunkSize)
	chunkThreshold, _ := parseByteSize(cfg.Sync.ChunkThreshold)
	return engine.Config{
		MaxWorkers:      cfg.Sync.MaxWorkers,
		UploadWorkers:   cfg.Sync.UploadWorkers,
		DownloadWorkers: cfg.Sync.DownloadWorkers,
		QueueSize:       cfg.Sync.QueueSize,
		RetryMax:        cfg.Sync.RetryMax,
		RetryDelay:      cfg.Sync.RetryDelay,
		ScanInterval:    cfg.Sync.ScanInterval,
		UploadLimit:     uploadLimit,
		DownloadLimit:   downloadLimit,
		ChunkSize:       chunkSize,
		ChunkThreshold:  chunkThreshold,
		WebhookURL:      cfg.Notification.WebhookURL,
		EmailSMTPAddr:   cfg.Notification.Email.SMTPAddr,
		EmailUsername:   cfg.Notification.Email.Username,
		EmailPassword:   cfg.Notification.Email.Password,
		EmailFrom:       cfg.Notification.Email.From,
		EmailTo:         cfg.Notification.Email.To,
		DryRun:          dryRun,
	}
}

func parseByteRate(value string) (int64, error) {
	cleaned := strings.TrimSpace(strings.ToUpper(value))
	cleaned = strings.TrimSuffix(cleaned, "/S")
	cleaned = strings.TrimSuffix(cleaned, "PS")
	return parseByteSize(cleaned)
}

func parseByteSize(value string) (int64, error) {
	cleaned := strings.TrimSpace(strings.ToUpper(value))
	if cleaned == "" || cleaned == "0" {
		return 0, nil
	}

	units := []struct {
		suffix string
		mult   int64
	}{
		{"GIB", 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MIB", 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KIB", 1024},
		{"KB", 1024},
		{"B", 1},
	}
	mult := int64(1)
	for _, unit := range units {
		if strings.HasSuffix(cleaned, unit.suffix) {
			mult = unit.mult
			cleaned = strings.TrimSpace(strings.TrimSuffix(cleaned, unit.suffix))
			break
		}
	}

	n, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, nil
	}
	return int64(n * float64(mult)), nil
}

type providerEntry struct {
	Type   string
	Params map[string]string
}

func registerAllPairs(eng *engine.Engine, s *store.Store, cfg *config.Config) error {
	_ = local.LocalProvider{}
	_ = webdav.WebDAVProvider{}

	providers := map[string]providerEntry{}

	for _, pc := range cfg.Provider {
		providers[pc.Name] = providerEntry{Type: pc.Type, Params: pc.Params}
	}

	dbConfigs, _ := s.ListProviderConfigs()
	if dbConfigs != nil {
		for _, pc := range dbConfigs {
			providers[pc.Name] = providerEntry{Type: pc.Type, Params: pc.Params}
		}
	}

	pairs, err := s.ListSyncPairs()
	if err != nil {
		return err
	}

	for _, pair := range pairs {
		if !pair.Enabled {
			continue
		}
		if err := registerPairWithProviders(eng, pair, providers); err != nil {
			return err
		}
	}

	return nil
}

func registerPair(eng *engine.Engine, s *store.Store, cfg *config.Config, pair *store.SyncPair) error {
	providers := map[string]providerEntry{}
	for _, pc := range cfg.Provider {
		providers[pc.Name] = providerEntry{Type: pc.Type, Params: pc.Params}
	}
	dbConfigs, _ := s.ListProviderConfigs()
	if dbConfigs != nil {
		for _, pc := range dbConfigs {
			providers[pc.Name] = providerEntry{Type: pc.Type, Params: pc.Params}
		}
	}
	return registerPairWithProviders(eng, pair, providers)
}

func registerPairWithProviders(eng *engine.Engine, pair *store.SyncPair, providers map[string]providerEntry) error {
	localProv, ok := provider.Create("local")
	if !ok {
		return fmt.Errorf("local provider not registered")
	}
	if err := localProv.Init(context.Background(), provider.Config{
		Params: map[string]string{"root_path": pair.LocalPath},
	}); err != nil {
		return fmt.Errorf("init local provider for pair %s: %w", pair.Name, err)
	}

	if pair.Provider == "local" {
		remoteProv, ok := provider.Create("local")
		if !ok {
			return fmt.Errorf("local provider not registered")
		}
		if err := remoteProv.Init(context.Background(), provider.Config{
			Params: map[string]string{"root_path": pair.RemotePath},
		}); err != nil {
			return fmt.Errorf("init remote provider for pair %s: %w", pair.Name, err)
		}
		eng.RegisterPair(pair, localProv, remoteProv)
		return nil
	}

	entry, found := providers[pair.Provider]
	if !found {
		return fmt.Errorf("provider '%s' not found — use 'every-sync provider add' to create it", pair.Provider)
	}
	remoteProv, ok := provider.Create(entry.Type)
	if !ok {
		return fmt.Errorf("provider type '%s' not registered", entry.Type)
	}
	params := scopedRemoteParams(entry.Type, entry.Params, pair.RemotePath)
	if err := remoteProv.Init(context.Background(), provider.Config{
		Type:   entry.Type,
		Params: params,
	}); err != nil {
		return fmt.Errorf("init provider '%s' for pair %s: %w", pair.Provider, pair.Name, err)
	}
	eng.RegisterPair(pair, localProv, remoteProv)
	return nil
}

// makePairRegistrar returns a callback that creates providers for a pair.
func makePairRegistrar(s *store.Store, cfg *config.Config) engine.PairRegistrar {
	_ = local.LocalProvider{}
	_ = webdav.WebDAVProvider{}

	return func(pair *store.SyncPair) (provider.Provider, provider.Provider, error) {
		localProv, ok := provider.Create("local")
		if !ok {
			return nil, nil, fmt.Errorf("local provider not registered")
		}
		if err := localProv.Init(context.Background(), provider.Config{
			Params: map[string]string{"root_path": pair.LocalPath},
		}); err != nil {
			return nil, nil, fmt.Errorf("init local provider: %w", err)
		}

		if pair.Provider == "local" {
			remoteProv, ok := provider.Create("local")
			if !ok {
				return nil, nil, fmt.Errorf("local provider not registered")
			}
			if err := remoteProv.Init(context.Background(), provider.Config{
				Params: map[string]string{"root_path": pair.RemotePath},
			}); err != nil {
				return nil, nil, fmt.Errorf("init remote provider: %w", err)
			}
			return localProv, remoteProv, nil
		}

		// Look up provider config
		pc, _ := s.GetProviderConfigByName(pair.Provider)
		if pc == nil {
			for _, p := range cfg.Provider {
				if p.Name == pair.Provider {
					pc = &store.ProviderConfig{Type: p.Type, Params: p.Params}
					break
				}
			}
		}
		if pc == nil {
			return nil, nil, fmt.Errorf("provider '%s' not found", pair.Provider)
		}

		remoteProv, ok := provider.Create(pc.Type)
		if !ok {
			return nil, nil, fmt.Errorf("provider type '%s' not registered", pc.Type)
		}
		params := scopedRemoteParams(pc.Type, pc.Params, pair.RemotePath)
		if err := remoteProv.Init(context.Background(), provider.Config{
			Type:   pc.Type,
			Params: params,
		}); err != nil {
			return nil, nil, fmt.Errorf("init provider '%s': %w", pair.Provider, err)
		}

		return localProv, remoteProv, nil
	}
}

// triggerSyncForPair creates an engine, registers the pair, and runs sync.
func triggerSyncForPair(s *store.Store, cfg *config.Config, pair *store.SyncPair) error {
	eng := engine.New(s, engineConfigFromAppConfig(cfg, false))

	if err := registerAllPairs(eng, s, cfg); err != nil {
		return fmt.Errorf("register pairs: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}

	if err := eng.SyncPair(ctx, pair.ID, pair.Direction); err != nil {
		eng.Stop()
		return fmt.Errorf("sync: %w", err)
	}

	eng.Drain(30 * time.Second)
	eng.Stop()
	return nil
}

func scopedRemoteParams(providerType string, params map[string]string, remotePath string) map[string]string {
	scoped := make(map[string]string, len(params)+1)
	for k, v := range params {
		scoped[k] = v
	}
	if providerType != "webdav" {
		return scoped
	}

	pairPrefix := cleanRemotePrefix(remotePath)
	if pairPrefix == "" {
		return scoped
	}
	if existing := cleanRemotePrefix(scoped["prefix"]); existing != "" {
		scoped["prefix"] = path.Join(existing, pairPrefix)
	} else {
		scoped["prefix"] = pairPrefix
	}
	return scoped
}

func cleanRemotePrefix(remotePath string) string {
	cleaned := path.Clean("/" + remotePath)
	if cleaned == "/" || cleaned == "." {
		return ""
	}
	return cleaned
}

// --- PID file management ---

func pidFilePath() string {
	dir := pidDir()
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "every-sync.pid")
}

func pidDir() string {
	if dataDir != "" {
		return dataDir
	}
	if configPath != "" {
		return filepath.Dir(configPath)
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".every-sync")
}

func writePID(path string) error {
	// Check if an existing process is still alive.
	data, err := os.ReadFile(path)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && pid > 0 {
			if proc, _ := os.FindProcess(pid); proc != nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					return fmt.Errorf("every-sync already running (pid %d)", pid)
				}
			}
		}
		_ = os.Remove(path)
	}

	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func removePID(path string) {
	os.Remove(path)
}

// --- Stop & Restart ---

func cmdStop(_ []string) {
	pidFile := pidFilePath()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "every-sync is not running (pid file not found)")
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid pid file content: %s\n", string(data))
		os.Remove(pidFile)
		os.Exit(1)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find process %d: %v\n", pid, err)
		os.Remove(pidFile)
		os.Exit(1)
	}

	// Check if process is alive.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		fmt.Fprintln(os.Stderr, "every-sync is not running")
		os.Remove(pidFile)
		os.Exit(1)
	}

	fmt.Printf("Stopping every-sync (pid %d)...\n", pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "failed to send SIGTERM: %v\n", err)
		os.Exit(1)
	}

	// Wait for process to exit, up to 30s.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if proc.Signal(syscall.Signal(0)) != nil {
			fmt.Println("Stopped.")
			os.Remove(pidFile)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Fprintln(os.Stderr, "timed out waiting for process to exit, force killing...")
	proc.Signal(syscall.SIGKILL)
	os.Remove(pidFile)
}

func cmdRestart(args []string) {
	pidFile := pidFilePath()

	// Try to stop existing process.
	if data, err := os.ReadFile(pidFile); err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && pid > 0 {
			if proc, _ := os.FindProcess(pid); proc != nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					fmt.Printf("Stopping every-sync (pid %d)...\n", pid)
					proc.Signal(syscall.SIGTERM)
					deadline := time.Now().Add(30 * time.Second)
					for time.Now().Before(deadline) {
						if proc.Signal(syscall.Signal(0)) != nil {
							break
						}
						time.Sleep(200 * time.Millisecond)
					}
					os.Remove(pidFile)
					fmt.Println("Stopped. Restarting...")
				}
			}
		}
	} else {
		fmt.Println("No running instance found. Starting...")
	}

	// Re-exec current binary with "serve" + original args.
	execArgs := append([]string{"serve"}, args...)
	binary, _ := os.Executable()
	syscall.Exec(binary, append([]string{os.Args[0]}, execArgs...), os.Environ())
}
