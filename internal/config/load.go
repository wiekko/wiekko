package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
)

func Load(args []string) (*Config, error) {
	cfg := defaults()

	applyEnv(cfg)

	if err := applyFlags(cfg, args); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Host:             DefaultHost,
		Port:             DefaultPort,
		LogLevel:         DefaultLogLevel,
		LogFile:          DefaultLogFile,
		DBDriver:         DefaultDBDriver,
		DBDsn:            DefaultDBDsn,
		SessionTTL:       DefaultSessionTTL,
		PBKDF2Iterations: DefaultPBKDF2Iterations,
		CookieSecure:     DefaultCookieSecure,
		DevMode:          DefaultDevMode,
		UploadDir:        DefaultUploadDir,
		MaxUploadBytes:   DefaultMaxUploadBytes,
		AllowedUploads:   DefaultAllowedUploads,
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("NEXMAP_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("NEXMAP_PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("NEXMAP_LOGLEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("NEXMAP_LOGFILE"); v != "" {
		cfg.LogFile = v
	}
	if v := os.Getenv("NEXMAP_DB_DRIVER"); v != "" {
		cfg.DBDriver = v
	}
	if v := os.Getenv("NEXMAP_DB_DSN"); v != "" {
		cfg.DBDsn = v
	}
	if v := os.Getenv("NEXMAP_SESSION_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.SessionTTL = d
		}
	}
	if v := os.Getenv("NEXMAP_PBKDF2_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PBKDF2Iterations = n
		}
	}
	if v := os.Getenv("NEXMAP_COOKIE_SECURE"); v != "" {
		cfg.CookieSecure = v == "true" || v == "1"
	}
	if v := os.Getenv("NEXMAP_ENCRYPTION_KEY"); v != "" {
		cfg.EncryptionKey = v
	}
	if v := os.Getenv("NEXMAP_DEV_MODE"); v != "" {
		cfg.DevMode = v == "true" || v == "1"
	}
	if v := os.Getenv("NEXMAP_ADMIN_USER"); v != "" {
		cfg.InitialAdminUser = v
	}
	if v := os.Getenv("NEXMAP_ADMIN_PASSWORD"); v != "" {
		cfg.InitialAdminPass = v
	}
	if v := os.Getenv("NEXMAP_UPLOAD_DIR"); v != "" {
		cfg.UploadDir = v
	}
	if v := os.Getenv("NEXMAP_MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.MaxUploadBytes = n
		}
	}
	if v := os.Getenv("NEXMAP_ALLOWED_UPLOAD_TYPES"); v != "" {
		cfg.AllowedUploads = v
	}
}

func applyFlags(cfg *Config, args []string) error {
	fs := flag.NewFlagSet("nexmap", flag.ContinueOnError)

	// Pointers to shadow vars; only applied when `flag.Visit` reports them set.
	host := fs.String("host", "", "")
	port := fs.String("port", "", "")
	logLevel := fs.String("log-level", "", "")
	logFile := fs.String("log-file", "", "")
	dbDriver := fs.String("db-driver", "", "")
	dbDsn := fs.String("db-dsn", "", "")
	sessionTTL := fs.String("session-ttl", "", "")
	pbkdf2Iter := fs.String("pbkdf2-iterations", "", "")
	cookieSecure := fs.String("cookie-secure", "", "")
	uploadDir := fs.String("upload-dir", "", "")
	maxUpload := fs.String("max-upload-bytes", "", "")
	allowedUploads := fs.String("allowed-upload-types", "", "")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("config: flags: %w", err)
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			cfg.Host = *host
		case "port":
			cfg.Port = *port
		case "log-level":
			cfg.LogLevel = *logLevel
		case "log-file":
			cfg.LogFile = *logFile
		case "db-driver":
			cfg.DBDriver = *dbDriver
		case "db-dsn":
			cfg.DBDsn = *dbDsn
		case "session-ttl":
			if d, err := time.ParseDuration(*sessionTTL); err == nil {
				cfg.SessionTTL = d
			}
		case "pbkdf2-iterations":
			if n, err := strconv.Atoi(*pbkdf2Iter); err == nil && n > 0 {
				cfg.PBKDF2Iterations = n
			}
		case "cookie-secure":
			cfg.CookieSecure = *cookieSecure == "true" || *cookieSecure == "1"
		case "upload-dir":
			cfg.UploadDir = *uploadDir
		case "max-upload-bytes":
			if n, err := strconv.ParseInt(*maxUpload, 10, 64); err == nil && n > 0 {
				cfg.MaxUploadBytes = n
			}
		case "allowed-upload-types":
			cfg.AllowedUploads = *allowedUploads
		}
	})

	return nil
}
