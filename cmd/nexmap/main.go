package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"nexmap.net/internal/config"
	"nexmap.net/internal/logging"
	"nexmap.net/internal/router"
	"nexmap.net/internal/server"
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

	// Set up signal-based context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Build the handler and start the server.
	handler := router.New()
	if err := server.Run(ctx, cfg, handler); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
