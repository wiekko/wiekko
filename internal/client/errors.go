// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError is an error status returned by the server. Code and Message come
// from the {"error": {"code", "message"}} envelope when the body has one.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s (%s, HTTP %d)", msg, e.Code, e.StatusCode)
	}
	return fmt.Sprintf("%s (HTTP %d)", msg, e.StatusCode)
}

// TransportError is a failure to get any response from the server.
type TransportError struct {
	Method string
	URL    string
	Err    error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Method, e.URL, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsUnauthorized reports whether err is an API 401, i.e. a missing, invalid,
// expired or revoked token.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

// IsForbidden reports whether err is an API 403.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

func hasStatus(err error, status int) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == status
}

// ParseError builds an APIError from a raw error response.
func ParseError(resp *Response) *APIError { return parseAPIError(resp) }

func parseAPIError(resp *Response) *APIError {
	ae := &APIError{StatusCode: resp.StatusCode}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body, &env); err == nil && (env.Error.Code != "" || env.Error.Message != "") {
		ae.Code = env.Error.Code
		ae.Message = env.Error.Message
		return ae
	}
	// Not our envelope (e.g. a proxy error page or http.Error plain text):
	// keep a short single-line excerpt.
	text := strings.TrimSpace(string(resp.Body))
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if len(text) > 200 || strings.HasPrefix(text, "<") {
		text = ""
	}
	ae.Message = text
	return ae
}
