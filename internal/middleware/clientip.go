// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var clientIPKey = &contextKey{"clientIP"}

// ClientIP determines the address of the client behind each request and stores
// it in the context, for sessions, the failed-login log and the login limits.
//
// The X-Forwarded-For header is only believed when the request arrives from a
// trusted proxy. Otherwise anyone reaching the server directly could claim a
// new address with every request and so evade the limits. From a trusted
// proxy, the header is read right to left across all of its lines, skipping
// further trusted proxies: the first other address is the one our own proxies
// saw, whatever the client put in front of it.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := resolveClientIP(r, trusted)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

// ClientIPFromContext returns the client address determined by ClientIP, or ""
// if the middleware did not run.
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := remoteHost(r.RemoteAddr)
	peerAddr, err := netip.ParseAddr(peer)
	if err != nil || !isTrusted(peerAddr, trusted) {
		return peer
	}

	// Several header lines are equivalent to one comma-separated line; a proxy
	// that appends its own line must not leave us reading the client's.
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(line, ",")...)
	}
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(remoteHost(strings.TrimSpace(hops[i])))
		if err != nil {
			break // not an address: nothing further left can be trusted
		}
		client = addr.Unmap().String()
		if !isTrusted(addr, trusted) {
			break
		}
	}
	return client
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// remoteHost strips the port from "host:port" or "[host]:port"; a bare address
// is returned unchanged.
func remoteHost(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return strings.Trim(addr, "[]")
}
