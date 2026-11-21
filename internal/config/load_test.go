// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"testing"
	"time"
)

func clearNexmapEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"NEXMAP_HOST", "NEXMAP_PORT", "NEXMAP_LOGLEVEL", "NEXMAP_LOGFILE",
		"NEXMAP_DB_DRIVER", "NEXMAP_DB_DSN", "NEXMAP_SESSION_TTL",
		"NEXMAP_PBKDF2_ITERATIONS", "NEXMAP_COOKIE_SECURE",
		"NEXMAP_ENCRYPTION_KEY", "NEXMAP_ADMIN_USER", "NEXMAP_ADMIN_PASSWORD",
		"NEXMAP_PUBLIC_URL",
		"NEXMAP_MAIL_TRANSPORT", "NEXMAP_MAIL_FROM", "NEXMAP_MAIL_MAX_PER_HOUR",
		"NEXMAP_SMTP_HOST", "NEXMAP_SMTP_PORT", "NEXMAP_SMTP_TLS",
		"NEXMAP_SMTP_USERNAME", "NEXMAP_SMTP_PASSWORD", "NEXMAP_MAIL_API_KEY",
		"NEXMAP_MAILGUN_DOMAIN", "NEXMAP_MAILGUN_REGION",
		"NEXMAP_SCALEWAY_PROJECT_ID", "NEXMAP_SCALEWAY_REGION",
	} {
		t.Setenv(key, "")
	}
}

func TestDefaults(t *testing.T) {
	clearNexmapEnv(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Host != DefaultHost {
		t.Errorf("Host = %q, want %q", cfg.Host, DefaultHost)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %q, want %q", cfg.Port, DefaultPort)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.LogFile != DefaultLogFile {
		t.Errorf("LogFile = %q, want %q", cfg.LogFile, DefaultLogFile)
	}
	if cfg.DBDriver != DefaultDBDriver {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, DefaultDBDriver)
	}
	if cfg.DBDsn != DefaultDBDsn {
		t.Errorf("DBDsn = %q, want %q", cfg.DBDsn, DefaultDBDsn)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, DefaultSessionTTL)
	}
	if cfg.PBKDF2Iterations != DefaultPBKDF2Iterations {
		t.Errorf("PBKDF2Iterations = %d, want %d", cfg.PBKDF2Iterations, DefaultPBKDF2Iterations)
	}
	if cfg.CookieSecure != DefaultCookieSecure {
		t.Errorf("CookieSecure = %v, want %v", cfg.CookieSecure, DefaultCookieSecure)
	}
	if cfg.EncryptionKey != "" {
		t.Errorf("EncryptionKey = %q, want empty", cfg.EncryptionKey)
	}
}

func TestEnvOverrides(t *testing.T) {
	clearNexmapEnv(t)
	t.Setenv("NEXMAP_HOST", "0.0.0.0")
	t.Setenv("NEXMAP_PORT", "9090")
	t.Setenv("NEXMAP_LOGLEVEL", "debug")
	t.Setenv("NEXMAP_LOGFILE", "/var/log/nexmap.log")
	t.Setenv("NEXMAP_DB_DRIVER", "postgres")
	t.Setenv("NEXMAP_DB_DSN", "postgres://localhost/nexmap")
	t.Setenv("NEXMAP_SESSION_TTL", "2h")
	t.Setenv("NEXMAP_PBKDF2_ITERATIONS", "100000")
	t.Setenv("NEXMAP_COOKIE_SECURE", "false")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.LogFile != "/var/log/nexmap.log" {
		t.Errorf("LogFile = %q, want %q", cfg.LogFile, "/var/log/nexmap.log")
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "postgres")
	}
	if cfg.DBDsn != "postgres://localhost/nexmap" {
		t.Errorf("DBDsn = %q, want %q", cfg.DBDsn, "postgres://localhost/nexmap")
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 2*time.Hour)
	}
	if cfg.PBKDF2Iterations != 100000 {
		t.Errorf("PBKDF2Iterations = %d, want %d", cfg.PBKDF2Iterations, 100000)
	}
	if cfg.CookieSecure != false {
		t.Errorf("CookieSecure = %v, want false", cfg.CookieSecure)
	}
}

func TestEnvDurationParsing(t *testing.T) {
	clearNexmapEnv(t)

	t.Run("valid duration", func(t *testing.T) {
		t.Setenv("NEXMAP_SESSION_TTL", "30m")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SessionTTL != 30*time.Minute {
			t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 30*time.Minute)
		}
	})

	t.Run("invalid duration keeps default", func(t *testing.T) {
		t.Setenv("NEXMAP_SESSION_TTL", "garbage")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SessionTTL != DefaultSessionTTL {
			t.Errorf("SessionTTL = %v, want default %v", cfg.SessionTTL, DefaultSessionTTL)
		}
	})
}

func TestEnvIntegerParsing(t *testing.T) {
	clearNexmapEnv(t)

	t.Run("valid integer", func(t *testing.T) {
		t.Setenv("NEXMAP_PBKDF2_ITERATIONS", "200000")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.PBKDF2Iterations != 200000 {
			t.Errorf("PBKDF2Iterations = %d, want %d", cfg.PBKDF2Iterations, 200000)
		}
	})

	t.Run("negative keeps default", func(t *testing.T) {
		t.Setenv("NEXMAP_PBKDF2_ITERATIONS", "-1")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.PBKDF2Iterations != DefaultPBKDF2Iterations {
			t.Errorf("PBKDF2Iterations = %d, want default %d", cfg.PBKDF2Iterations, DefaultPBKDF2Iterations)
		}
	})

	t.Run("non-numeric keeps default", func(t *testing.T) {
		t.Setenv("NEXMAP_PBKDF2_ITERATIONS", "abc")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.PBKDF2Iterations != DefaultPBKDF2Iterations {
			t.Errorf("PBKDF2Iterations = %d, want default %d", cfg.PBKDF2Iterations, DefaultPBKDF2Iterations)
		}
	})
}

func TestEnvCookieSecure(t *testing.T) {
	clearNexmapEnv(t)

	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
	} {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("NEXMAP_COOKIE_SECURE", tc.env)
			cfg, err := Load(nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.CookieSecure != tc.want {
				t.Errorf("CookieSecure = %v, want %v", cfg.CookieSecure, tc.want)
			}
		})
	}
}

func TestEnvEncryptionKey(t *testing.T) {
	clearNexmapEnv(t)
	t.Setenv("NEXMAP_ENCRYPTION_KEY", `[{"id":"1","key":"dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1wYWQtaGVyZSE="}]`)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EncryptionKey == "" {
		t.Error("EncryptionKey should not be empty")
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	clearNexmapEnv(t)
	t.Setenv("NEXMAP_HOST", "envhost")

	cfg, err := Load([]string{"-host", "flaghost"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "flaghost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "flaghost")
	}
}

func TestFlagsAllFields(t *testing.T) {
	clearNexmapEnv(t)

	cfg, err := Load([]string{
		"-host", "flaghost",
		"-port", "3000",
		"-log-level", "error",
		"-log-file", "/tmp/test.log",
		"-db-driver", "postgres",
		"-db-dsn", "pg://db",
		"-session-ttl", "4h",
		"-pbkdf2-iterations", "50000",
		"-cookie-secure", "false",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Host != "flaghost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "flaghost")
	}
	if cfg.Port != "3000" {
		t.Errorf("Port = %q, want %q", cfg.Port, "3000")
	}
	if cfg.LogLevel != "error" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "error")
	}
	if cfg.LogFile != "/tmp/test.log" {
		t.Errorf("LogFile = %q, want %q", cfg.LogFile, "/tmp/test.log")
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "postgres")
	}
	if cfg.DBDsn != "pg://db" {
		t.Errorf("DBDsn = %q, want %q", cfg.DBDsn, "pg://db")
	}
	if cfg.SessionTTL != 4*time.Hour {
		t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 4*time.Hour)
	}
	if cfg.PBKDF2Iterations != 50000 {
		t.Errorf("PBKDF2Iterations = %d, want %d", cfg.PBKDF2Iterations, 50000)
	}
	if cfg.CookieSecure != false {
		t.Errorf("CookieSecure = %v, want false", cfg.CookieSecure)
	}
}

func TestFlagsInvalid(t *testing.T) {
	clearNexmapEnv(t)

	_, err := Load([]string{"-unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestPublicURL(t *testing.T) {
	clearNexmapEnv(t)

	t.Setenv("NEXMAP_PUBLIC_URL", "https://nexmap.example.com/")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PublicURL != "https://nexmap.example.com" {
		t.Errorf("PublicURL = %q, want trailing slash removed", cfg.PublicURL)
	}

	cfg, err = Load([]string{"--public-url", "http://localhost:8080"})
	if err != nil {
		t.Fatalf("Load with flag: %v", err)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Errorf("flag PublicURL = %q", cfg.PublicURL)
	}

	for _, bad := range []string{"nexmap.example.com", "ftp://nexmap.example.com", "https://", "https://u:p@nexmap.example.com", "https://x.example.com/?a=b"} {
		t.Setenv("NEXMAP_PUBLIC_URL", bad)
		if _, err := Load(nil); err == nil {
			t.Errorf("Load accepted public URL %q", bad)
		}
	}
}

func TestMailDisabledByDefault(t *testing.T) {
	clearNexmapEnv(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mail.Enabled() {
		t.Errorf("mail enabled without configuration: %+v", cfg.Mail)
	}
}

func TestMailSMTP(t *testing.T) {
	clearNexmapEnv(t)
	t.Setenv("NEXMAP_PUBLIC_URL", "https://nexmap.example.com")
	t.Setenv("NEXMAP_MAIL_TRANSPORT", "smtp")
	t.Setenv("NEXMAP_MAIL_FROM", "nexmap <noreply@example.com>")
	t.Setenv("NEXMAP_SMTP_HOST", "mail.example.com")
	t.Setenv("NEXMAP_SMTP_PORT", "2525")
	t.Setenv("NEXMAP_SMTP_USERNAME", "u")
	t.Setenv("NEXMAP_SMTP_PASSWORD", "p")
	t.Setenv("NEXMAP_MAIL_MAX_PER_HOUR", "10")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m := cfg.Mail
	if !m.Enabled() || m.SMTPPort != 2525 || m.SMTPTLS != "starttls" || m.MaxPerHour != 10 || m.SMTPUsername != "u" {
		t.Errorf("mail config = %+v", m)
	}
}

// Without a public URL, reset links would be built from the Host header of
// whoever asks for a reset.
func TestMailRequiresPublicURL(t *testing.T) {
	clearNexmapEnv(t)
	t.Setenv("NEXMAP_MAIL_TRANSPORT", "sendgrid")
	t.Setenv("NEXMAP_MAIL_FROM", "noreply@example.com")
	t.Setenv("NEXMAP_MAIL_API_KEY", "k")
	if _, err := Load(nil); err == nil {
		t.Fatal("Load accepted mail without NEXMAP_PUBLIC_URL")
	}
}

func TestMailInvalid(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"unknown transport": {"NEXMAP_MAIL_TRANSPORT": "pigeon"},
		"missing from":      {"NEXMAP_MAIL_TRANSPORT": "sendgrid", "NEXMAP_MAIL_API_KEY": "k"},
		"missing key":       {"NEXMAP_MAIL_TRANSPORT": "mailpace", "NEXMAP_MAIL_FROM": "a@example.com"},
		"bad port":          {"NEXMAP_MAIL_TRANSPORT": "smtp", "NEXMAP_MAIL_FROM": "a@example.com", "NEXMAP_SMTP_HOST": "h", "NEXMAP_SMTP_PORT": "x"},
		"bad cap":           {"NEXMAP_MAIL_TRANSPORT": "lettermint", "NEXMAP_MAIL_FROM": "a@example.com", "NEXMAP_MAIL_API_KEY": "k", "NEXMAP_MAIL_MAX_PER_HOUR": "-5"},
	} {
		t.Run(name, func(t *testing.T) {
			clearNexmapEnv(t)
			t.Setenv("NEXMAP_PUBLIC_URL", "https://nexmap.example.com")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(nil); err == nil {
				t.Error("Load accepted an invalid mail configuration")
			}
		})
	}
}
