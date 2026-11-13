package middleware

import "net/http"

// contentSecurityPolicy restricts what a page may load and where it may send
// data. It is the second line of defence behind output escaping: should
// markup from user content ever reach a page, it cannot pull in a script from
// elsewhere, post a form to another site or be framed, and Markdown images
// cannot point at a third-party server that would learn who reads a comment.
//
// 'unsafe-inline' is still allowed for scripts and styles, because the
// templates use inline event handlers and inline scripts, and 'unsafe-eval'
// because htmx evaluates hx-on attributes (the comment and upload forms).
// Inline script injected through an escaping bug is therefore not stopped by
// this policy; it only cannot load code from or send data to another server.
// Tightening it means moving those handlers into static files.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// SecurityHeaders is middleware that sets standard security response headers.
// With https set (the server is reached over HTTPS, see NEXMAP_COOKIE_SECURE)
// it also tells browsers to use HTTPS only for the next year.
func SecurityHeaders(https bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
