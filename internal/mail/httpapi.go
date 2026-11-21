// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

// apiTimeout bounds one request to a mailer API.
const apiTimeout = 15 * time.Second

// maxAPIResponse is how much of a response is read (and thrown away) so the
// connection can be reused; a misbehaving endpoint cannot make nexmap buffer
// more.
const maxAPIResponse = 64 << 10

// newAPIClient returns the HTTP client for the API transports. It never
// follows redirects: a redirect would carry the API key, or the message with
// its link, to wherever the response points.
func newAPIClient() *http.Client {
	return &http.Client{
		Timeout: apiTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// apiRequest is one POST to a mailer API.
type apiRequest struct {
	name        string // transport, for errors
	url         string
	contentType string
	body        []byte
	header      http.Header
	basicUser   string
	basicPass   string
	okStatus    []int
}

// do sends r and returns an error unless the API answered with one of
// r.okStatus. The error names the status only, never the response body,
// which may repeat the recipient.
func do(ctx context.Context, client *http.Client, r apiRequest) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(r.body))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", r.name, err)
	}
	for k, vs := range r.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Type", r.contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nexmap")
	if r.basicUser != "" {
		req.SetBasicAuth(r.basicUser, r.basicPass)
	}

	resp, err := client.Do(req)
	if err != nil {
		// The URL in a *url.Error holds no secret (keys travel in headers),
		// but keep the message short and uniform.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("%s: request: %w", r.name, err)
		}
		return fmt.Errorf("%s: request failed", r.name)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxAPIResponse))

	if !slices.Contains(r.okStatus, resp.StatusCode) {
		return fmt.Errorf("%s: API answered %d", r.name, resp.StatusCode)
	}
	return nil
}

func jsonBody(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Only plain strings and maps go in; Marshal cannot fail on them.
		panic(fmt.Sprintf("mail: marshal request: %v", err))
	}
	return b
}
