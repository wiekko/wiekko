// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// utcTime is a ReplaceAttr function that converts slog timestamps to UTC.
func utcTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.TimeValue(t.UTC())
		}
	}
	return a
}

// nopCloser wraps a writer that needs no closing (e.g. os.Stdout).
type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// Setup configures the global slog logger and returns a closer for any opened
// log file. The caller should defer the closer.
//
// Output destination:
//   - logFile == ""   → stdout, JSONHandler
//   - logFile != ""   → specified file, JSONHandler
func Setup(level slog.Level, logFile string) (io.Closer, error) {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: utcTime}

	if logFile == "" {
		handler := slog.NewJSONHandler(os.Stdout, opts)
		slog.SetDefault(slog.New(handler))
		return nopCloser{}, nil
	}

	if err := os.MkdirAll(filepath.Dir(logFile), 0o700); err != nil {
		return nil, fmt.Errorf("logging: create log dir: %w", err)
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("logging: open log file %s: %w", logFile, err)
	}

	handler := slog.NewJSONHandler(f, opts)
	slog.SetDefault(slog.New(handler))
	return f, nil
}

// ParseLevel maps a string to a slog.Level. Recognised values (case-insensitive):
// "debug", "info", "warn", "warning", "error".
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("logging: unknown level %q (want debug|info|warn|error)", s)
	}
}
