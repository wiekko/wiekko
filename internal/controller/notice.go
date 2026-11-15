package controller

import (
	"net/http"
	"net/url"

	"nexmap.net/web/templates"
)

// Notices explain on the page a user lands on why they were sent there. They
// travel as a code in the query string (?notice=needs-owner); only the codes
// below are shown, so a link can never make a page display text of its own.
const (
	noticeNeedsOwner         = "needs-owner"
	noticeNeedsDeveloper     = "needs-developer"
	noticeNeedsTester        = "needs-tester"
	noticeProductCreationOff = "product-creation-off"
)

var noticeMessages = map[string]string{
	noticeNeedsOwner:         "You need the owner role in this product to create or edit that.",
	noticeNeedsDeveloper:     "You need at least the developer role in this product to create or edit that.",
	noticeNeedsTester:        "You need at least the tester role in this product to create that.",
	noticeProductCreationOff: "Creating products is turned off. Ask an administrator.",
}

// redirectWithNotice sends the user to target (a path without a query) and
// has the page there show the notice with the given code. Create and edit
// forms use it for users whose role cannot save them: create forms send them
// to the list, edit forms to the item's detail page, both of which they can
// read.
func redirectWithNotice(w http.ResponseWriter, r *http.Request, target, code string) {
	http.Redirect(w, r, target+"?"+url.Values{"notice": {code}}.Encode(), http.StatusSeeOther)
}

// WithNotice hands the notice named in a page request's query string to the
// templates (templates.WithNotice), which show it as a toast. htmx requests
// are skipped: they render fragments without the toast container. Unknown
// codes are ignored.
func WithNotice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.Header.Get("HX-Request") != "true" {
			if message, ok := noticeMessages[r.URL.Query().Get("notice")]; ok {
				r = r.WithContext(templates.WithNotice(r.Context(), message))
			}
		}
		next.ServeHTTP(w, r)
	})
}
