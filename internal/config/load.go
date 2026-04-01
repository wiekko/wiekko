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
}

func applyFlags(cfg *Config, args []string) error {
	fs := flag.NewFlagSet("nexmap", flag.ContinueOnError)

	// Pointers to shadow vars; only applied when flag.Visit reports them set.
	host := fs.String("host", "", "")
	port := fs.String("port", "", "")
	logLevel := fs.String("log-level", "", "")

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
		}
	})

	return nil
}
