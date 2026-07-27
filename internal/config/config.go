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
	DefaultDevMode          = false
	DefaultUploadDir        = "./uploads"
	DefaultMaxUploadBytes   = 10 << 20 // 10 MiB
	DefaultAllowedUploads   = ""       // empty = every file type is accepted
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
}
