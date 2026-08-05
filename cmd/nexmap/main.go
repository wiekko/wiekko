package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database"
	"nexmap.net/internal/devseed"
	"nexmap.net/internal/logging"
	"nexmap.net/internal/model"
	"nexmap.net/internal/router"
	"nexmap.net/internal/server"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

func main() {
	// Load configuration (defaults → env → flags).
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Set up structured logging.
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

	// Open database connection.
	db, err := database.Open(cfg)
	if err != nil {
		slog.Error("failed to open database", "driver", cfg.DBDriver, "err", err)
		os.Exit(1)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			slog.Error("failed to close db", "driver", cfg.DBDriver, "err", err)
		}
	}(db)

	// Run database migrations.
	if err := database.Migrate(db, cfg.DBDriver); err != nil {
		slog.Error("failed to run migrations", "err", err)
		os.Exit(1)
	}
	slog.Info("database ready", "driver", cfg.DBDriver)

	// Initialize encryption key ring for personal data.
	keys, err := crypto.NewKeyRing(cfg.EncryptionKey)
	if err != nil {
		slog.Error("failed to initialize encryption", "err", err)
		os.Exit(1)
	}

	// Initialize attachment storage.
	blobs, err := storage.NewFS(cfg.UploadDir, cfg.MaxUploadBytes)
	if err != nil {
		slog.Error("failed to initialize attachment storage", "err", err)
		os.Exit(1)
	}
	slog.Info("attachment storage ready", "dir", blobs.Root, "max_bytes", cfg.MaxUploadBytes)

	// Create store, auth provider, and services.
	st := store.New(db, keys)
	provider := builtin.New(st.User, cfg.PBKDF2Iterations)
	svc := service.New(st, provider, cfg, blobs)

	// Bootstrap admin account if no users exist.
	admin, err := bootstrapAdmin(st, provider, cfg.InitialAdminUser, cfg.InitialAdminPass)
	if err != nil {
		slog.Error("failed to bootstrap admin", "err", err)
		os.Exit(1)
	}

	// Populate demo data, but only in dev mode and only into a database that
	// was just created — a non-nil admin here means there were no users before.
	if cfg.DevMode && admin != nil {
		seedDevData(svc, st, provider, admin)
	}

	// Set up signal-based context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start session cleanup goroutine.
	go sessionCleanup(ctx, st.Session)

	// Build the handler and start the server.
	handler := router.New(svc, cfg.CookieSecure, cfg.DevMode, cfg.MaxUploadBytes)
	if err := server.Run(ctx, cfg, handler); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// bootstrapAdmin creates the initial admin account when the database holds no
// users. It returns the created admin, or nil when users already existed — the
// caller uses that as its "this database is brand new" signal.
func bootstrapAdmin(st *store.Store, provider *builtin.Provider, user string, password string) (*model.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	count, err := st.User.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, nil
	}

	logUser := false
	logPassword := false

	if user == "" {
		user = "admin"
		logUser = true
	}

	if password == "" {
		password, err = generateBootstrapPassword()
		if err != nil {
			return nil, err
		}
		logPassword = true
	}

	hash, err := provider.HashPassword(password)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	admin := &model.User{
		ID:           uuid.New().String(),
		Username:     user,
		Email:        "admin@localhost",
		DisplayName:  "Administrator",
		PasswordHash: hash,
		IsAdmin:      true,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := st.User.Create(ctx, admin); err != nil {
		return nil, err
	}

	if !logUser {
		user = "as provided by ENV"
	}
	if !logPassword {
		password = "as provided by ENV"
	}
	slog.Info("default admin account created", "username", user, "password", password)
	return admin, nil
}

// seedDevData populates a fresh dev-mode database with demo products. A failure
// here is logged rather than fatal: a broken fixture must not make the app
// unbootable, and the log points at the step that failed. The result is a
// partially seeded database, which deleting the DB file resets.
func seedDevData(svc *service.Services, st *store.Store, provider *builtin.Provider, admin *model.User) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	slog.Warn("dev mode: seeding demo data into the new database",
		"products", "Aviary, Finch Tracker",
		"demo_users", "ada, bo, cy",
		"demo_password", devseed.DemoPassword)

	if err := devseed.Seed(ctx, svc, st, provider, admin); err != nil {
		slog.Error("dev seed failed; database may be partially seeded", "err", err)
		return
	}
	slog.Info("dev demo data seeded")
}

func generateBootstrapPassword() (string, error) {
	const charset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

func sessionCleanup(ctx context.Context, sessions store.SessionStore) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sessions.DeleteExpired(context.Background()); err != nil {
				slog.Error("session cleanup failed", "err", err)
			}
		}
	}
}
