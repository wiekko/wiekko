package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Logging is middleware that logs each request with method, path, status,
// duration, and request ID.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.LogAttrs(r.Context(), slog.LevelInfo, "http request",
			slog.String("method", r.Method),
			slog.String("path", loggablePath(r.URL.Path)),
			slog.Int("status", rw.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("request_id", RequestIDFromContext(r.Context())),
		)
	})
}

// secretPathPrefixes are routes whose next path segment is a secret token.
// Logging it would let anyone who reads the log use the link (SC-303).
var secretPathPrefixes = []string{"/accept-invite/", "/reset-password/"}

// loggablePath returns path with secret tokens replaced.
func loggablePath(path string) string {
	for _, prefix := range secretPathPrefixes {
		if strings.HasPrefix(path, prefix) && len(path) > len(prefix) {
			return prefix + "[redacted]"
		}
	}
	return path
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, allowing http.ResponseController
// to access optional interfaces (Flusher, Hijacker, etc.).
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
