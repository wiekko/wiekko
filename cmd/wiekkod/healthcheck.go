// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"wiekko.net/internal/config"
)

// healthcheck implements `wiekkod healthcheck`: it asks the server running
// with the same configuration (environment and flags) whether it is up, and
// returns the process exit code. It exists for container HEALTHCHECKs, so the
// image needs no curl or wget.
func healthcheck(args []string) int {
	cfg, err := config.Load(args)
	if err != nil {
		fmt.Printf("healthcheck: %v\n", err)
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(healthURL(cfg.Host, cfg.Port))
	if err != nil {
		fmt.Printf("healthcheck: %v\n", err)
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("healthcheck: %s\n", resp.Status)
		return 1
	}
	return 0
}

// healthURL is the health endpoint of a server listening on host:port. A
// wildcard listen address is reached over loopback.
func healthURL(host, port string) string {
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::", "[::]":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/api/v1/health"
}
