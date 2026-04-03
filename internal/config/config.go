package config

import "time"

const (
	DefaultHost             = "localhost"
	DefaultPort             = "8080"
	DefaultLogLevel         = "info"
	DefaultLogFile          = ""
	DefaultDBDriver         = "sqlite"
	DefaultDBDsn            = "nexmap.db"
	DefaultSessionTTL       = 24 * time.Hour
	DefaultPBKDF2Iterations = 600_000
	DefaultCookieSecure     = true
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
}
