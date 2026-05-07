package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/rain/every-sync/internal/config"
)

// L is the global logger instance.
var L zerolog.Logger

const maxLogFileSize = 10 * 1024 * 1024

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
			if f, err := newRotatingFileWriter(filepath.Join(cfg.Path, "every-sync.log"), maxLogFileSize); err == nil {
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

func Audit(action string) *zerolog.Event {
	return L.Info().Bool("audit", true).Str("action", action)
}

type rotatingFileWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
}

func newRotatingFileWriter(path string, maxBytes int64) (*rotatingFileWriter, error) {
	w := &rotatingFileWriter{path: path, maxBytes: maxBytes}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if info, err := w.file.Stat(); err == nil && info.Size()+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

func (w *rotatingFileWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = f
	return nil
}

func (w *rotatingFileWriter) rotate() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	rotated := w.path + ".1"
	_ = os.Remove(rotated)
	if _, err := os.Stat(w.path); err == nil {
		_ = os.Rename(w.path, rotated)
	}
	return w.open()
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
