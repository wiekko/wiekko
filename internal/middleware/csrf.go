// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"nexmap.net/internal/httputil"
)

const csrfHeader = "X-CSRF-Token"

// formMemory matches what r.FormValue uses when it parses a multipart body.
const formMemory = 32 << 20

// CSRF validates the CSRF token for session-authenticated state-changing requests.
// PAT-authenticated requests are exempt (bearer tokens are inherently CSRF-safe).
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStateChanging(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// PAT-authenticated requests are exempt.
		if AuthMethodFromContext(r.Context()) != "session" {
			next.ServeHTTP(w, r)
			return
		}

		expected := CSRFTokenFromContext(r.Context())
		provided := r.Header.Get(csrfHeader)
		if provided == "" {
			// Parse the form explicitly: r.FormValue would swallow a parse
			// error and report a body over the cap as a missing token.
			// ParseForm first: ParseMultipartForm reports ErrNotMultipart for a
			// url-encoded body even when reading it failed.
			err := r.ParseForm()
			if err == nil {
				if err = r.ParseMultipartForm(formMemory); errors.Is(err, http.ErrNotMultipart) {
					err = nil
				}
			}
			if err != nil {
				if IsTooLarge(err) {
					DrainBody(r.Body)
					WriteBodyError(w, r, http.StatusRequestEntityTooLarge, TooLargeMessage(limitOf(r)))
					return
				}
				WriteBodyError(w, r, http.StatusBadRequest, unreadableMessage)
				return
			}
			provided = r.FormValue("_csrf")
		}
		if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			httputil.Error(w, http.StatusForbidden, "CSRF_INVALID", "Invalid or missing CSRF token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}
