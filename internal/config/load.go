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
		}
	})

	return nil
}
