package middleware

import (
	"net/http"
	"strings"
)

const (
	// DefaultMaxBody bounds an ordinary request body: a form or a JSON
	// document. Nothing the application accepts outside the two exceptions
	// below comes near it.
	DefaultMaxBody = 1 << 20
	// MaxTestRunBody bounds a test run upload (FR-409). Passing tests carry no
	// output and failures a capped amount each, so this leaves room for large
	// suites.
	MaxTestRunBody = 32 << 20
	// uploadEnvelope is the headroom for the multipart envelope around an
	// attachment.
	uploadEnvelope = 1 << 20
)

// LimitBody caps the size of every request body, so that no route — including
// the public login and registration endpoints — reads an unbounded body into
// memory or onto disk. Reading past the cap fails with *http.MaxBytesError,
// which handlers report as 413.
//
// The cap depends on the route: API attachment uploads get maxUploadBytes plus
// the multipart envelope, test run uploads MaxTestRunBody, everything else
// DefaultMaxBody. The web UI's attachment uploads are left to the upload
// routes themselves (PageProductController.LimitUploadBody), which cap the
// body the same way but also drain an oversized one, so the browser receives
// the error message instead of a reset connection.
func LimitBody(maxUploadBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit := bodyLimit(r, maxUploadBytes); limit > 0 && r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bodyLimit returns the cap for r's body, or 0 where the route caps it itself.
func bodyLimit(r *http.Request, maxUploadBytes int64) int64 {
	if r.Method != http.MethodPost {
		return DefaultMaxBody
	}
	api := strings.HasPrefix(r.URL.Path, "/api/")
	switch {
	case strings.HasSuffix(r.URL.Path, "/attachments") && api:
		return maxUploadBytes + uploadEnvelope
	case strings.HasSuffix(r.URL.Path, "/attachments") && strings.HasPrefix(r.URL.Path, "/products/"):
		return 0
	case strings.HasSuffix(r.URL.Path, "/test-runs") && api:
		return MaxTestRunBody
	}
	return DefaultMaxBody
}
