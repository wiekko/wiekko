package config

const (
	DefaultHost     = "localhost"
	DefaultPort     = "8080"
	DefaultLogLevel = "info"
	DefaultLogFile  = ""
)

type Config struct {
	Host     string
	Port     string
	LogLevel string
	LogFile  string
}
