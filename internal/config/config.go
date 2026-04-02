package config

const (
	DefaultHost     = "localhost"
	DefaultPort     = "8080"
	DefaultLogLevel = "info"
	DefaultLogFile  = ""
	DefaultDBDriver = "sqlite"
	DefaultDBDsn    = "nexmap.db"
)

type Config struct {
	Host     string
	Port     string
	LogLevel string
	LogFile  string
	DBDriver string
	DBDsn    string
}
