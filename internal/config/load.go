// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"flag"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"nexmap.net/internal/mail"
)

func Load(args []string) (*Config, error) {
	cfg := defaults()
	trustedProxies := DefaultTrustedProxies

	applyEnv(cfg)
	if v, ok := os.LookupEnv("NEXMAP_TRUSTED_PROXIES"); ok {
		trustedProxies = v
	}

	if err := applyFlags(cfg, args, &trustedProxies); err != nil {
		return nil, err
	}

	proxies, err := ParseTrustedProxies(trustedProxies)
	if err != nil {
		return nil, err
	}
	cfg.TrustedProxies = proxies

	publicURL, err := normalizePublicURL(cfg.PublicURL)
	if err != nil {
		return nil, err
	}
	cfg.PublicURL = publicURL

	if err := loadMail(cfg); err != nil {
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
	if v := os.Getenv("NEXMAP_PUBLIC_URL"); v != "" {
		cfg.PublicURL = v
	}
}

func applyFlags(cfg *Config, args []string, trustedProxies *string) error {
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
	publicURL := fs.String("public-url", "", "")
	proxies := fs.String("trusted-proxies", "", "")

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
		case "public-url":
			cfg.PublicURL = *publicURL
		case "trusted-proxies":
			*trustedProxies = *proxies
		}
	})

	return nil
}

// normalizePublicURL checks that raw is an absolute http(s) URL and returns it
// without a trailing slash. Empty stays empty.
func normalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("config: public URL %q must be an absolute http or https URL, like https://nexmap.example.com", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("config: public URL %q must not contain credentials, a query or a fragment", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// ParseTrustedProxies parses a comma-separated list of addresses and CIDR
// ranges ("10.0.0.0/8, 192.0.2.7"). Empty or "none" trusts no proxy.
func ParseTrustedProxies(spec string) ([]netip.Prefix, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "none") {
		return nil, nil
	}
	var out []netip.Prefix
	for _, raw := range strings.Split(spec, ",") {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if p, err := netip.ParsePrefix(item); err == nil {
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("config: trusted proxy %q is neither an IP address nor a CIDR range like 10.0.0.0/8", item)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// loadMail reads the mail settings, from the environment only: several are
// secrets, and command line flags are visible to every user of the machine.
func loadMail(cfg *Config) error {
	m := mail.Config{
		Transport:         os.Getenv("NEXMAP_MAIL_TRANSPORT"),
		From:              strings.TrimSpace(os.Getenv("NEXMAP_MAIL_FROM")),
		SMTPHost:          strings.TrimSpace(os.Getenv("NEXMAP_SMTP_HOST")),
		SMTPTLS:           os.Getenv("NEXMAP_SMTP_TLS"),
		SMTPUsername:      os.Getenv("NEXMAP_SMTP_USERNAME"),
		SMTPPassword:      os.Getenv("NEXMAP_SMTP_PASSWORD"),
		APIKey:            os.Getenv("NEXMAP_MAIL_API_KEY"),
		MailgunDomain:     strings.TrimSpace(os.Getenv("NEXMAP_MAILGUN_DOMAIN")),
		MailgunRegion:     os.Getenv("NEXMAP_MAILGUN_REGION"),
		ScalewayProjectID: strings.TrimSpace(os.Getenv("NEXMAP_SCALEWAY_PROJECT_ID")),
		ScalewayRegion:    os.Getenv("NEXMAP_SCALEWAY_REGION"),
	}
	for name, dst := range map[string]*int{
		"NEXMAP_MAIL_MAX_PER_HOUR": &m.MaxPerHour,
		"NEXMAP_SMTP_PORT":         &m.SMTPPort,
	} {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("config: %s must be a positive number", name)
		}
		*dst = n
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Links in emails must not be built from the request: on the public
	// "forgot password" form, the Host header is whatever the requester sends,
	// and a reset link pointing at their server would hand them the token.
	if m.Enabled() && cfg.PublicURL == "" {
		return fmt.Errorf("config: sending mail (NEXMAP_MAIL_TRANSPORT) requires NEXMAP_PUBLIC_URL, the address the links in the emails point to")
	}
	cfg.Mail = m
	return nil
}
