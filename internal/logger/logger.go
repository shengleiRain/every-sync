package logger

import (
	"fmt"
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
		writers = append(writers, consoleWriter(os.Stderr))
	} else {
		writers = append(writers, os.Stderr)
	}

	if cfg.Path != "" {
		if err := os.MkdirAll(cfg.Path, 0755); err == nil {
			if f, err := os.OpenFile(filepath.Join(cfg.Path, "every-sync.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				if cfg.Format == "console" {
					writers = append(writers, consoleWriter(f))
				} else {
					writers = append(writers, f)
				}
			}
		}
	}

	L = zerolog.New(io.MultiWriter(writers...)).With().Timestamp().Str("tag", "every-sync").Logger()
	log.Logger = L
}

func consoleWriter(out io.Writer) zerolog.ConsoleWriter {
	return zerolog.ConsoleWriter{
		Out:           out,
		TimeFormat:    time.DateTime,
		PartsOrder:    []string{"time", "level", "tag", "message"},
		FieldsExclude: []string{"tag"},
		FormatLevel: func(i interface{}) string {
			return fmt.Sprintf("level=%s", i)
		},
		FormatMessage: func(i interface{}) string {
			return fmt.Sprintf("event=%s", i)
		},
	}
}

func parseLevel(s string) zerolog.Level {
	if l, err := zerolog.ParseLevel(s); err == nil {
		return l
	}
	return zerolog.InfoLevel
}
