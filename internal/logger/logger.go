package logger

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/rain/every-sync/internal/config"
)

// L is the global logger instance.
var L zerolog.Logger

func Init(cfg config.LogConfig) {
	zerolog.TimeFieldFormat = time.DateTime
	zerolog.SetGlobalLevel(parseLevel(cfg.Level))

	var writers []io.Writer

	if cfg.Format == "console" {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.DateTime,
		})
	} else {
		writers = append(writers, os.Stderr)
	}

	if cfg.Path != "" {
		if err := os.MkdirAll(cfg.Path, 0755); err == nil {
			if f, err := os.OpenFile(filepath.Join(cfg.Path, "every-sync.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				writers = append(writers, f)
			}
		}
	}

	L = zerolog.New(io.MultiWriter(writers...)).With().Timestamp().Logger()
	log.Logger = L
}

func parseLevel(s string) zerolog.Level {
	if l, err := zerolog.ParseLevel(s); err == nil {
		return l
	}
	return zerolog.InfoLevel
}
