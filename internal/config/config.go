package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `yaml:"server" json:"server"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Sync     SyncConfig     `yaml:"sync" json:"sync"`
	Log      LogConfig      `yaml:"log" json:"log"`
	Pairs    []SyncPair     `yaml:"pairs" json:"pairs"`
	Provider []ProviderConf `yaml:"providers" json:"providers"`
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

type SyncConfig struct {
	MaxWorkers      int           `yaml:"max_workers" json:"max_workers"`
	UploadWorkers   int           `yaml:"upload_workers" json:"upload_workers"`
	DownloadWorkers int           `yaml:"download_workers" json:"download_workers"`
	QueueSize       int           `yaml:"queue_size" json:"queue_size"`
	RetryMax        int           `yaml:"retry_max" json:"retry_max"`
	RetryDelay      time.Duration `yaml:"retry_delay" json:"retry_delay"`
	ScanInterval    time.Duration `yaml:"scan_interval" json:"scan_interval"`
	UploadLimit     string        `yaml:"upload_limit" json:"upload_limit"`
	DownloadLimit   string        `yaml:"download_limit" json:"download_limit"`
}

type LogConfig struct {
	Level  string `yaml:"level" json:"level"`
	Format string `yaml:"format" json:"format"`
	Path   string `yaml:"path" json:"path"`
}

type SyncPair struct {
	Name       string `yaml:"name" json:"name"`
	LocalPath  string `yaml:"local_path" json:"local_path"`
	RemotePath string `yaml:"remote_path" json:"remote_path"`
	Provider   string `yaml:"provider" json:"provider"`
	Mode       string `yaml:"mode" json:"mode"`           // mirror, selective, virtual
	Direction  string `yaml:"direction" json:"direction"` // up, down, both
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	Schedule   string `yaml:"schedule" json:"schedule"`
}

type ProviderConf struct {
	Name   string            `yaml:"name" json:"name"`
	Type   string            `yaml:"type" json:"type"`
	Params map[string]string `yaml:"params" json:"params"`
}

func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".every-sync")

	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 10086,
		},
		Database: DatabaseConfig{
			Path: filepath.Join(dataDir, "every-sync.db"),
		},
		Sync: SyncConfig{
			MaxWorkers:      0,
			UploadWorkers:   4,
			DownloadWorkers: 8,
			QueueSize:       10000,
			RetryMax:        3,
			RetryDelay:      5 * time.Second,
			ScanInterval:    5 * time.Minute,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
			Path:   filepath.Join(dataDir, "logs"),
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(*os.PathError); ok {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func GenerateExample(path string) error {
	cfg := Default()
	cfg.Pairs = []SyncPair{
		{
			Name:       "photos",
			LocalPath:  "/home/user/photos",
			RemotePath: "/photos",
			Provider:   "webdav",
			Mode:       "mirror",
			Direction:  "both",
			Enabled:    true,
		},
	}
	cfg.Provider = []ProviderConf{
		{
			Name: "webdav",
			Type: "webdav",
			Params: map[string]string{
				"endpoint": "https://dav.example.com",
				"username": "user",
				"password": "pass",
			},
		},
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Use reflection-free approach: write YAML directly
	return writeYAML(path, cfg)
}

func writeYAML(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	content := `# EverySync Configuration
server:
  host: "` + cfg.Server.Host + `"
  port: ` + fmt.Sprintf("%d", cfg.Server.Port) + `

database:
  path: "` + cfg.Database.Path + `"

sync:
  max_workers: ` + fmt.Sprintf("%d", cfg.Sync.MaxWorkers) + `
  upload_workers: ` + fmt.Sprintf("%d", cfg.Sync.UploadWorkers) + `
  download_workers: ` + fmt.Sprintf("%d", cfg.Sync.DownloadWorkers) + `
  queue_size: ` + fmt.Sprintf("%d", cfg.Sync.QueueSize) + `
  retry_max: ` + fmt.Sprintf("%d", cfg.Sync.RetryMax) + `
  retry_delay: ` + cfg.Sync.RetryDelay.String() + `
  scan_interval: ` + cfg.Sync.ScanInterval.String() + `
  upload_limit: "0"
  download_limit: "0"

log:
  level: "` + cfg.Log.Level + `"
  format: "` + cfg.Log.Format + `"
  path: "` + cfg.Log.Path + `"

providers: []

pairs: []
`

	_, err = f.WriteString(content)
	return err
}
