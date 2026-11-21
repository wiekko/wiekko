// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"nexmap.net/internal/httputil"
)

const (
	// DefaultMaxBody bounds an ordinary request body: a form or a JSON
	// document. Nothing the application accepts outside the two exceptions
	// below comes near it.
	DefaultMaxBody = 1 << 20
	// MaxTestRunBody bounds a test run upload (FR-118). Passing tests carry no
	// output and failures a capped amount each, so this leaves room for large
	// suites.
	MaxTestRunBody = 32 << 20
	// uploadEnvelope is the headroom for the multipart envelope around an
	// attachment.
	uploadEnvelope = 1 << 20
	// maxDrainBytes bounds how much of a rejected body is read and discarded.
	maxDrainBytes = 32 << 20
)

// LimitBody caps the size of every request body, so that no route — including
// the public login and registration endpoints — reads an unbounded body into
// memory or onto disk.
//
// A body whose Content-Length already exceeds the cap is answered with 413
// before anything reads it; browsers always send one, so this is where an
// oversized form or htmx request ends. A body without a Content-Length
// (chunked), or one longer than it declared, is cut off by an
// http.MaxBytesReader instead: reading past the cap fails with
// *http.MaxBytesError, which the readers report as 413 (CSRF, DecodeJSON).
//
// The cap depends on the route: API attachment uploads get maxUploadBytes plus
// the multipart envelope, test run uploads MaxTestRunBody, everything else
// DefaultMaxBody. The web UI's attachment uploads are left to the upload
// routes themselves (PageProductController.LimitUploadBody), which answer an
// oversized upload inside the attachment panel rather than as an error page.
func LimitBody(maxUploadBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := bodyLimit(r, maxUploadBytes)
			if limit > 0 && r.Body != nil && r.Body != http.NoBody {
				if r.ContentLength > limit {
					if r.ContentLength <= maxDrainBytes {
						_, _ = io.Copy(io.Discard, r.Body)
					} else {
						w.Header().Set("Connection", "close")
					}
					WriteBodyError(w, r, http.StatusRequestEntityTooLarge, TooLargeMessage(limit))
					return
				}
				r.Body = &limitedBody{ReadCloser: http.MaxBytesReader(w, r.Body, limit), orig: r.Body, limit: limit}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limitedBody remembers the body under the MaxBytesReader, so that DrainBody
// can still consume it once the cap has tripped.
type limitedBody struct {
	io.ReadCloser
	orig  io.ReadCloser
	limit int64
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

// DrainBody consumes what is left of a request body rejected as too large.
//
// When the body cap trips, Go sets Connection: close and stops reading. If the
// client is still uploading, the connection is torn down before it can read
// the response, so the rejection never reaches it. Consuming the rest lets the
// client finish and receive the reply. The bound keeps a deliberately huge
// body from costing us the whole transfer — past that the connection simply
// closes, which is the correct outcome for a client ignoring the limit.
//
// body is the reader under any MaxBytesReader; for a request that went
// through LimitBody, pass r.Body and the original is found automatically.
func DrainBody(body io.Reader) {
	if lb, ok := body.(*limitedBody); ok {
		body = lb.orig
	}
	_, _ = io.CopyN(io.Discard, body, maxDrainBytes)
}

// IsTooLarge reports whether err comes from reading past the body cap.
func IsTooLarge(err error) bool {
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig)
}

// TooLargeMessage is the message for a body over limit bytes (0 if unknown).
func TooLargeMessage(limit int64) string {
	if limit <= 0 {
		return "That is too large to send."
	}
	return "That is too large to send. The limit is " + humanBytes(limit) + "."
}

// limitOf returns the cap LimitBody put on r's body, or 0 if it set none.
func limitOf(r *http.Request) int64 {
	if lb, ok := r.Body.(*limitedBody); ok {
		return lb.limit
	}
	return 0
}

// unreadableMessage is the message for a body that arrived but cannot be
// parsed (a broken multipart envelope, a truncated upload).
const unreadableMessage = "The request could not be read. Please try again."

// WriteBodyError answers a request whose body was rejected (413) or could not
// be read (400), in the form the client can show:
//   - the JSON API gets the usual error envelope;
//   - htmx gets plain text, which htmx-errors.js shows as a toast;
//   - a plain browser form post gets a small HTML page, since the browser
//     replaces the form with whatever comes back.
func WriteBodyError(w http.ResponseWriter, r *http.Request, status int, message string) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/"):
		code := "INVALID_BODY"
		if status == http.StatusRequestEntityTooLarge {
			code = "TOO_LARGE"
		}
		httputil.Error(w, status, code, message)
	case r.Header.Get("HX-Request") == "true":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, message)
	default:
		heading := "Request could not be read"
		if status == http.StatusRequestEntityTooLarge {
			heading = "Too large to send"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, bodyErrorPage, html.EscapeString(heading), html.EscapeString(message))
	}
}

// bodyErrorPage is self-contained (no scripts, no external assets) so it
// renders under any CSP and without the layout's data.
const bodyErrorPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%[1]s · Nexmap</title>
<style>
body{font-family:system-ui,sans-serif;max-width:36rem;margin:4rem auto;padding:0 1rem;line-height:1.5;color:#1f2937;background:#fff}
h1{font-size:1.25rem}
@media (prefers-color-scheme:dark){body{color:#e5e7eb;background:#111827}}
</style>
</head>
<body>
<h1>%[1]s</h1>
<p>%[2]s</p>
<p>Use your browser's Back button: what you entered is still in the form.</p>
</body>
</html>
`

// humanBytes renders a byte count in binary units ("1 MiB", "1.5 MiB").
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	if n%div == 0 {
		return fmt.Sprintf("%d %ciB", n/div, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
