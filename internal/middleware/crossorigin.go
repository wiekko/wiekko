// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"wiekko.net/internal/httputil"
)

// CrossOrigin rejects state-changing requests that a browser sends on behalf
// of another site. It complements the CSRF token, which only protects requests
// made with a session: the sign-in, registration and invitation forms have no
// session yet, so without this another site could, for example, sign a visitor
// in to an account of its own choosing.
//
// The check is the standard library's (http.CrossOriginProtection): browsers
// say in Sec-Fetch-Site where a request comes from, older ones in Origin,
// which is compared with the Host header. Requests without either header — the
// CLI, CI jobs, scripts — are not from a browser and pass.
//
// publicURL (WIEKKO_PUBLIC_URL), if set, is accepted as an origin as well, for
// the Origin comparison behind a proxy that does not pass the Host header on.
func CrossOrigin(publicURL string) (func(http.Handler) http.Handler, error) {
	protection := http.NewCrossOriginProtection()
	if publicURL != "" {
		if err := protection.AddTrustedOrigin(publicURL); err != nil {
			return nil, fmt.Errorf("middleware: public URL %q is not usable as an origin: %w", publicURL, err)
		}
	}
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const message = "Cross-origin request refused"
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			httputil.Error(w, http.StatusForbidden, "CROSS_ORIGIN", message)
			return
		}
		http.Error(w, message, http.StatusForbidden)
	}))
	return protection.Handler, nil
}
