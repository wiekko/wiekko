package main

import (
	"io"
	"log/slog"
	"os"

	"nexmap.net/internal/config"
	"nexmap.net/internal/logging"
)

func main() {
	// Load configuration (defaults →  env → flags)
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Set up structured logging
	level, err := logging.ParseLevel(cfg.LogLevel)
	if err != nil {
		slog.Error("invalid log level", "level", cfg.LogLevel, "err", err)
		os.Exit(1)
	}

	logCloser, err := logging.Setup(level, cfg.LogFile)
	if err != nil {
		slog.Error("failed to set up logging", "err", err)
		os.Exit(1)
	}
	defer func(logCloser io.Closer) {
		err := logCloser.Close()
		if err != nil {
			slog.Error("failed to close logCloser", "err", err)
		}
	}(logCloser)
}
