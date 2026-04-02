package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"nexmap.net/internal/config"
)

const shutdownTimeout = 10 * time.Second

// Run starts the HTTP server and blocks until ctx is cancelled, then performs
// a graceful shutdown. It returns nil on clean shutdown.
func Run(ctx context.Context, cfg *config.Config, handler http.Handler) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start listening in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server started", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Block until context is cancelled or the server fails to start.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// Graceful shutdown.
	slog.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return nil
}
