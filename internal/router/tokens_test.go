// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"wiekko.net/internal/access"
	"wiekko.net/internal/database/dbtest"
	"wiekko.net/internal/service"
)

// TestRestrictedTokens drives restricted personal access tokens over HTTP
// (SC-51 – SC-56).
func TestRestrictedTokens(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, db *sql.DB) {
		e := newAccountEnv(t, driver, db)
		ctx := context.Background()
		mint := func(in service.CreatePATInput) string {
			t.Helper()
			in.Name = "test"
			in.Password = goodPassword // confirms tokens with the account or admin scope
			res, err := e.svc.PAT.Create(ctx, e.admin, in)
			if err != nil {
				t.Fatalf("create token: %v", err)
			}
			return res.RawToken
		}
		call := func(token, method, path, body string) (int, string) {
			t.Helper()
			req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			if body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := e.client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, readBody(t, resp)
		}
		products := "/api/v1/products/" + e.product.ID

		full := mint(service.CreatePATInput{})

		// Pages never accept tokens: no account management through the web UI.
		if status, _ := call(full, http.MethodPost, "/user/tokens", ""); status != http.StatusSeeOther {
			t.Errorf("POST /user/tokens with a token: status %d, want a redirect to the login page", status)
		}
		if status, _ := call(full, http.MethodGet, "/user/tokens", ""); status != http.StatusSeeOther {
			t.Errorf("GET /user/tokens with a token: status %d, want a redirect to the login page", status)
		}

		// Without the account scope a token cannot create tokens.
		status, body := call(full, http.MethodPost, "/api/v1/user/me/tokens", `{"name":"escalate"}`)
		if status != http.StatusForbidden || !strings.Contains(body, "TOKEN_SCOPE") {
			t.Errorf("token creation without account scope: %d %s", status, body)
		}
		// Without the admin scope an administrator's token has no admin rights.
		if status, _ := call(full, http.MethodGet, "/api/v1/admin/users", ""); status != http.StatusForbidden {
			t.Errorf("admin route without admin scope: status %d", status)
		}
		// GET /user/me stays open and shows the restriction.
		status, body = call(full, http.MethodGet, "/api/v1/user/me", "")
		if status != http.StatusOK || !strings.Contains(body, `"is_admin":false`) || !strings.Contains(body, "backlog:write") {
			t.Errorf("GET /user/me: %d %s", status, body)
		}

		// A token with the account scope creates only narrower tokens.
		acct := mint(service.CreatePATInput{Grants: access.Grants{access.Account: access.Write, access.Tests: access.Write}})
		if status, body := call(acct, http.MethodPost, "/api/v1/user/me/tokens", `{"name":"wider","scopes":["defects:read"]}`); status != http.StatusForbidden {
			t.Errorf("wider token from a token: %d %s", status, body)
		}
		status, body = call(acct, http.MethodPost, "/api/v1/user/me/tokens", `{"name":"narrower","scopes":["tests:read"],"role_cap":"tester"}`)
		if status != http.StatusCreated || !strings.Contains(body, `"role_cap":"tester"`) {
			t.Errorf("narrower token from a token: %d %s", status, body)
		}

		// defects:read reads defects and nothing else.
		defects := mint(service.CreatePATInput{Grants: access.Grants{access.Defects: access.Read}})
		if status, body := call(defects, http.MethodGet, products+"/defects", ""); status != http.StatusOK {
			t.Errorf("list defects with defects:read: %d %s", status, body)
		}
		if status, _ := call(defects, http.MethodPost, products+"/defects", `{"title":"x"}`); status != http.StatusForbidden {
			t.Errorf("create defect with defects:read: status %d", status)
		}
		if status, _ := call(defects, http.MethodGet, products+"/requirements", ""); status != http.StatusForbidden {
			t.Errorf("list requirements with defects:read: status %d", status)
		}

		// A product restriction hides other products, also from listings.
		elsewhere := mint(service.CreatePATInput{ProductsRestricted: true, ProductIDs: []string{e.product.ID}})
		other, err := e.svc.Product.Create(ctx, e.admin, service.CreateProductInput{Name: "Elsewhere"})
		if err != nil {
			t.Fatal(err)
		}
		if status, _ := call(elsewhere, http.MethodGet, "/api/v1/products/"+other.ID+"/defects", ""); status != http.StatusForbidden {
			t.Errorf("product outside the token: status %d", status)
		}
		status, body = call(elsewhere, http.MethodGet, "/api/v1/products", "")
		if status != http.StatusOK || strings.Contains(body, other.ID) || !strings.Contains(body, e.product.ID) {
			t.Errorf("product listing: %d %s", status, body)
		}
		if status, _ := call(elsewhere, http.MethodPost, "/api/v1/products", `{"name":"New"}`); status != http.StatusForbidden {
			t.Errorf("product creation with a product-restricted token: status %d", status)
		}
	})
}
