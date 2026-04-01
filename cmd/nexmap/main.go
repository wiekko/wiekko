package main

import (
	"log/slog"
	"os"

	"nexmap.net/internal/config"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
}
