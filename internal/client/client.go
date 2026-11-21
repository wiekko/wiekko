// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package client is a typed client for the Wiekko REST API (/api/v1). It is the
// only way the CLI talks to a server (AC-16) and must not import any server
// package: the JSON wire format is the contract.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// apiPrefix is the path every API endpoint lives under.
const apiPrefix = "/api/v1"

// Defaults for New.
const (
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3
)

// ErrInsecureServer is returned by New for a plain-HTTP server that is not on
// the local machine: a personal access token must not cross the network
// unencrypted (SC-26).
var ErrInsecureServer = errors.New("refusing to send credentials over plain HTTP to a non-local server; use https:// (or allow plain HTTP explicitly)")

// Client talks to one Wiekko server.
type Client struct {
	base       *url.URL // server root, e.g. https://wiekko.example.com
	token      string
	httpClient *http.Client
	userAgent  string
	maxRetries int
	backoff    func(attempt int) time.Duration
	insecure   bool // plain HTTP to non-local hosts permitted
}

// Option configures a Client.
type Option func(*Client)

// WithToken sets the personal access token sent as Bearer credential.
func WithToken(token string) Option { return func(c *Client) { c.token = token } }

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithMaxRetries sets how often an idempotent request is retried after a
// transient failure (network error, 429, 502, 503, 504).
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithBackoff replaces the delay between retries (tests use a zero backoff).
func WithBackoff(f func(attempt int) time.Duration) Option {
	return func(c *Client) { c.backoff = f }
}

// AllowInsecure permits plain HTTP to non-local hosts.
func AllowInsecure() Option { return func(c *Client) { c.insecure = true } }

// New creates a Client for server, the root URL of a Wiekko installation. A
// trailing /api/v1 is tolerated and stripped.
func New(server string, opts ...Option) (*Client, error) {
	base, err := NormalizeServer(server)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(base) // NormalizeServer already validated it

	c := &Client{
		base:       u,
		httpClient: &http.Client{Timeout: DefaultTimeout},
		userAgent:  "wiekko-client",
		maxRetries: DefaultMaxRetries,
		backoff:    defaultBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	if u.Scheme == "http" && !c.insecure && !isLoopback(u.Hostname()) {
		return nil, ErrInsecureServer
	}
	return c, nil
}

// Server returns the normalized server root URL.
func (c *Client) Server() string { return c.base.String() }

// NormalizeServer validates a server URL and returns its canonical form: scheme
// and host lowercased, no trailing slash, no /api/v1 suffix, no query. It is
// also the key under which credentials are stored.
func NormalizeServer(server string) (string, error) {
	s := strings.TrimSpace(server)
	if s == "" {
		return "", errors.New("no server configured")
	}
	if !strings.Contains(s, "://") {
		return "", fmt.Errorf("server %q must start with https:// or http://", server)
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid server URL %q: %w", server, err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("server %q must use https:// or http://", server)
	}
	if u.Host == "" {
		return "", fmt.Errorf("server %q has no host", server)
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), apiPrefix)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String(), nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func defaultBackoff(attempt int) time.Duration {
	// 0.5s, 1s, 2s, … capped at 8s.
	d := 500 * time.Millisecond << attempt
	if d > 8*time.Second || d <= 0 {
		d = 8 * time.Second
	}
	return d
}

// Response is the raw outcome of a request, for callers that print the body
// as is (the `wiekko api` command).
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Raw performs a request against an API path (relative to /api/v1, e.g.
// "/user/me") and returns the response without interpreting it. Transport
// failures are returned as errors; HTTP error statuses are not.
func (c *Client) Raw(ctx context.Context, method, path string, body []byte, contentType string) (*Response, error) {
	return c.send(ctx, method, path, body, contentType)
}

// Do performs a JSON request. body, when non-nil, is encoded as the request
// payload; out, when non-nil, receives the "data" member of the response
// envelope. Error statuses are returned as *APIError.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	resp, err := c.send(ctx, method, path, payload, "application/json")
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(resp.Body)) == 0 {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("decode response from %s %s: %w", method, path, err)
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("decode response from %s %s: missing data envelope", method, path)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode response from %s %s: %w", method, path, err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, method, path string, body []byte, contentType string) (*Response, error) {
	target, err := c.resolve(path)
	if err != nil {
		return nil, err
	}
	method = strings.ToUpper(method)
	retryable := isIdempotent(method)

	for attempt := 0; ; attempt++ {
		resp, err := c.once(ctx, method, target, body, contentType)
		if !retryable || attempt >= c.maxRetries || ctx.Err() != nil {
			return resp, err
		}
		wait, retry := retryDelay(resp, err, c.backoff(attempt))
		if !retry {
			return resp, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			if err == nil {
				return resp, nil
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) once(ctx context.Context, method, target string, body []byte, contentType string) (*Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// *url.Error repeats method and URL, which TransportError already carries.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, &TransportError{Method: method, URL: target, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &TransportError{Method: method, URL: target, Err: err}
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// resolve turns an API path into an absolute URL under /api/v1. The path may
// carry a query string.
func (c *Client) resolve(path string) (string, error) {
	if strings.Contains(path, "://") {
		return "", fmt.Errorf("API path %q must be relative to %s", path, apiPrefix)
	}
	p := "/" + strings.TrimLeft(path, "/")
	p = strings.TrimPrefix(p, apiPrefix) // tolerate callers passing the prefix
	if p == "" {
		p = "/"
	}
	ref, err := url.Parse(c.base.Path + apiPrefix + p)
	if err != nil {
		return "", fmt.Errorf("invalid API path %q: %w", path, err)
	}
	return c.base.ResolveReference(ref).String(), nil
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// retryDelay decides whether a failed attempt is worth repeating and how long
// to wait. A server-provided Retry-After (seconds) takes precedence.
func retryDelay(resp *Response, err error, fallback time.Duration) (time.Duration, bool) {
	if err != nil {
		var te *TransportError
		if errors.As(err, &te) && !errors.Is(te.Err, context.Canceled) && !errors.Is(te.Err, context.DeadlineExceeded) {
			return fallback, true
		}
		return 0, false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return 0, false
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, perr := strconv.Atoi(ra); perr == nil && secs >= 0 && secs <= 60 {
			return time.Duration(secs) * time.Second, true
		}
	}
	return fallback, true
}
