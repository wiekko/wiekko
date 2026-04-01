package config

const (
	DefaultHost     = "localhost"
	DefaultPort     = "8080"
	DefaultLogLevel = "info"
)

type Config struct {
	Host     string
	Port     string
	LogLevel string
}
