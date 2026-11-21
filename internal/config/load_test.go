// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"testing"
	"time"
)

func clearWiekkoEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"WIEKKO_HOST", "WIEKKO_PORT", "WIEKKO_LOGLEVEL", "WIEKKO_LOGFILE",
		"WIEKKO_DB_DRIVER", "WIEKKO_DB_DSN", "WIEKKO_SESSION_TTL",
		"WIEKKO_PBKDF2_ITERATIONS", "WIEKKO_COOKIE_SECURE",
		"WIEKKO_ENCRYPTION_KEY", "WIEKKO_ADMIN_USER", "WIEKKO_ADMIN_PASSWORD",
		"WIEKKO_PUBLIC_URL",
		"WIEKKO_MAIL_TRANSPORT", "WIEKKO_MAIL_FROM", "WIEKKO_MAIL_MAX_PER_HOUR",
		"WIEKKO_SMTP_HOST", "WIEKKO_SMTP_PORT", "WIEKKO_SMTP_TLS",
		"WIEKKO_SMTP_USERNAME", "WIEKKO_SMTP_PASSWORD", "WIEKKO_MAIL_API_KEY",
		"WIEKKO_MAILGUN_DOMAIN", "WIEKKO_MAILGUN_REGION",
		"WIEKKO_SCALEWAY_PROJECT_ID", "WIEKKO_SCALEWAY_REGION",
	} {
		t.Setenv(key, "")
	}
}

func TestDefaults(t *testing.T) {
	clearWiekkoEnv(t)

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
	clearWiekkoEnv(t)
	t.Setenv("WIEKKO_HOST", "0.0.0.0")
	t.Setenv("WIEKKO_PORT", "9090")
	t.Setenv("WIEKKO_LOGLEVEL", "debug")
	t.Setenv("WIEKKO_LOGFILE", "/var/log/wiekko.log")
	t.Setenv("WIEKKO_DB_DRIVER", "postgres")
	t.Setenv("WIEKKO_DB_DSN", "postgres://localhost/wiekko")
	t.Setenv("WIEKKO_SESSION_TTL", "2h")
	t.Setenv("WIEKKO_PBKDF2_ITERATIONS", "100000")
	t.Setenv("WIEKKO_COOKIE_SECURE", "false")

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
	if cfg.LogFile != "/var/log/wiekko.log" {
		t.Errorf("LogFile = %q, want %q", cfg.LogFile, "/var/log/wiekko.log")
	}
	if cfg.DBDriver != "postgres" {
		t.Errorf("DBDriver = %q, want %q", cfg.DBDriver, "postgres")
	}
	if cfg.DBDsn != "postgres://localhost/wiekko" {
		t.Errorf("DBDsn = %q, want %q", cfg.DBDsn, "postgres://localhost/wiekko")
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
	clearWiekkoEnv(t)

	t.Run("valid duration", func(t *testing.T) {
		t.Setenv("WIEKKO_SESSION_TTL", "30m")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SessionTTL != 30*time.Minute {
			t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 30*time.Minute)
		}
	})

	t.Run("invalid duration keeps default", func(t *testing.T) {
		t.Setenv("WIEKKO_SESSION_TTL", "garbage")
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
	clearWiekkoEnv(t)

	t.Run("valid integer", func(t *testing.T) {
		t.Setenv("WIEKKO_PBKDF2_ITERATIONS", "200000")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.PBKDF2Iterations != 200000 {
			t.Errorf("PBKDF2Iterations = %d, want %d", cfg.PBKDF2Iterations, 200000)
		}
	})

	t.Run("negative keeps default", func(t *testing.T) {
		t.Setenv("WIEKKO_PBKDF2_ITERATIONS", "-1")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.PBKDF2Iterations != DefaultPBKDF2Iterations {
			t.Errorf("PBKDF2Iterations = %d, want default %d", cfg.PBKDF2Iterations, DefaultPBKDF2Iterations)
		}
	})

	t.Run("non-numeric keeps default", func(t *testing.T) {
		t.Setenv("WIEKKO_PBKDF2_ITERATIONS", "abc")
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
	clearWiekkoEnv(t)

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
			t.Setenv("WIEKKO_COOKIE_SECURE", tc.env)
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
	clearWiekkoEnv(t)
	t.Setenv("WIEKKO_ENCRYPTION_KEY", `[{"id":"1","key":"dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1wYWQtaGVyZSE="}]`)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EncryptionKey == "" {
		t.Error("EncryptionKey should not be empty")
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	clearWiekkoEnv(t)
	t.Setenv("WIEKKO_HOST", "envhost")

	cfg, err := Load([]string{"-host", "flaghost"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "flaghost" {
		t.Errorf("Host = %q, want %q", cfg.Host, "flaghost")
	}
}

func TestFlagsAllFields(t *testing.T) {
	clearWiekkoEnv(t)

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
	clearWiekkoEnv(t)

	_, err := Load([]string{"-unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestPublicURL(t *testing.T) {
	clearWiekkoEnv(t)

	t.Setenv("WIEKKO_PUBLIC_URL", "https://wiekko.example.com/")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PublicURL != "https://wiekko.example.com" {
		t.Errorf("PublicURL = %q, want trailing slash removed", cfg.PublicURL)
	}

	cfg, err = Load([]string{"--public-url", "http://localhost:8080"})
	if err != nil {
		t.Fatalf("Load with flag: %v", err)
	}
	if cfg.PublicURL != "http://localhost:8080" {
		t.Errorf("flag PublicURL = %q", cfg.PublicURL)
	}

	for _, bad := range []string{"wiekko.example.com", "ftp://wiekko.example.com", "https://", "https://u:p@wiekko.example.com", "https://x.example.com/?a=b"} {
		t.Setenv("WIEKKO_PUBLIC_URL", bad)
		if _, err := Load(nil); err == nil {
			t.Errorf("Load accepted public URL %q", bad)
		}
	}
}

func TestMailDisabledByDefault(t *testing.T) {
	clearWiekkoEnv(t)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mail.Enabled() {
		t.Errorf("mail enabled without configuration: %+v", cfg.Mail)
	}
}

func TestMailSMTP(t *testing.T) {
	clearWiekkoEnv(t)
	t.Setenv("WIEKKO_PUBLIC_URL", "https://wiekko.example.com")
	t.Setenv("WIEKKO_MAIL_TRANSPORT", "smtp")
	t.Setenv("WIEKKO_MAIL_FROM", "wiekko <noreply@example.com>")
	t.Setenv("WIEKKO_SMTP_HOST", "mail.example.com")
	t.Setenv("WIEKKO_SMTP_PORT", "2525")
	t.Setenv("WIEKKO_SMTP_USERNAME", "u")
	t.Setenv("WIEKKO_SMTP_PASSWORD", "p")
	t.Setenv("WIEKKO_MAIL_MAX_PER_HOUR", "10")

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
	clearWiekkoEnv(t)
	t.Setenv("WIEKKO_MAIL_TRANSPORT", "sendgrid")
	t.Setenv("WIEKKO_MAIL_FROM", "noreply@example.com")
	t.Setenv("WIEKKO_MAIL_API_KEY", "k")
	if _, err := Load(nil); err == nil {
		t.Fatal("Load accepted mail without WIEKKO_PUBLIC_URL")
	}
}

func TestMailInvalid(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"unknown transport": {"WIEKKO_MAIL_TRANSPORT": "pigeon"},
		"missing from":      {"WIEKKO_MAIL_TRANSPORT": "sendgrid", "WIEKKO_MAIL_API_KEY": "k"},
		"missing key":       {"WIEKKO_MAIL_TRANSPORT": "mailpace", "WIEKKO_MAIL_FROM": "a@example.com"},
		"bad port":          {"WIEKKO_MAIL_TRANSPORT": "smtp", "WIEKKO_MAIL_FROM": "a@example.com", "WIEKKO_SMTP_HOST": "h", "WIEKKO_SMTP_PORT": "x"},
		"bad cap":           {"WIEKKO_MAIL_TRANSPORT": "lettermint", "WIEKKO_MAIL_FROM": "a@example.com", "WIEKKO_MAIL_API_KEY": "k", "WIEKKO_MAIL_MAX_PER_HOUR": "-5"},
	} {
		t.Run(name, func(t *testing.T) {
			clearWiekkoEnv(t)
			t.Setenv("WIEKKO_PUBLIC_URL", "https://wiekko.example.com")
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(nil); err == nil {
				t.Error("Load accepted an invalid mail configuration")
			}
		})
	}
}
