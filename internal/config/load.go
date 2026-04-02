package config

import (
	"flag"
	"fmt"
	"os"
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
		Host:     DefaultHost,
		Port:     DefaultPort,
		LogLevel: DefaultLogLevel,
		LogFile:  DefaultLogFile,
		DBDriver: DefaultDBDriver,
		DBDsn:    DefaultDBDsn,
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
		}
	})

	return nil
}
