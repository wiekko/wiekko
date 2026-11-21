// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"net/netip"
	"time"

	"wiekko.net/internal/mail"
)

const (
	DefaultHost             = "localhost"
	DefaultPort             = "8080"
	DefaultLogLevel         = "info"
	DefaultLogFile          = ""
	DefaultDBDriver         = "sqlite"
	DefaultDBDsn            = "wiekko.db"
	DefaultSessionTTL       = 24 * time.Hour
	DefaultPBKDF2Iterations = 600_000
	DefaultCookieSecure     = true
	DefaultDevMode          = false
	DefaultUploadDir        = "./uploads"
	DefaultMaxUploadBytes   = 10 << 20 // 10 MiB
	DefaultAllowedUploads   = ""       // empty = every file type is accepted
	// DefaultTrustedProxies covers a reverse proxy on the same host or on a
	// private (container) network. Requests from public addresses are never
	// believed about who their client is.
	DefaultTrustedProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"
)

type Config struct {
	Host             string
	Port             string
	LogLevel         string
	LogFile          string
	DBDriver         string
	DBDsn            string
	SessionTTL       time.Duration
	PBKDF2Iterations int
	CookieSecure     bool
	EncryptionKey    string
	DevMode          bool
	InitialAdminUser string
	InitialAdminPass string
	UploadDir        string
	MaxUploadBytes   int64
	AllowedUploads   string
	// PublicURL is the address users reach the server at ("https://wiekko.example.com"),
	// without a trailing slash. Empty: links are built from the request.
	PublicURL string
	// TrustedProxies are the addresses whose X-Forwarded-For header is
	// believed when determining a client's address. Empty: none.
	TrustedProxies []netip.Prefix
	// Mail configures the transport for invitation and password reset emails
	// (AC-10). Its zero value means wiekko sends no mail.
	Mail mail.Config
}
