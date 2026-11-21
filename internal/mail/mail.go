// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package mail sends wiekko's transactional emails (AC-10): invitations and
// password resets, nothing else (SC-38). It talks to an SMTP relay or to one
// of a few mailer HTTP APIs, all implemented on the standard library.
//
// Every message goes to exactly one recipient and is plain text. The package
// never logs and never puts a recipient, subject or body into an error, so a
// failed delivery cannot leak personal data or a token into the log (SC-47).
package mail

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// Message is one plain-text email to one recipient.
type Message struct {
	To      string // a bare address, "ada@example.com"
	Subject string
	Text    string
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
	// Name names the transport for the log ("smtp", "mailgun", …).
	Name() string
}

// Transports wiekko can send through.
const (
	TransportSMTP       = "smtp"
	TransportMailgun    = "mailgun"
	TransportSendGrid   = "sendgrid"
	TransportMailPace   = "mailpace"
	TransportScaleway   = "scaleway"
	TransportLettermint = "lettermint"
)

// ErrInvalidMessage is returned for a message that cannot be sent as it is: a
// recipient that is not a plain address, or a line break in a header value.
var ErrInvalidMessage = errors.New("mail: invalid message")

// maxAddressLength is the longest address SMTP allows (RFC 5321).
const maxAddressLength = 254

// New returns the Sender cfg describes, limited to cfg.MaxPerHour messages an
// hour, or nil when no transport is configured. cfg must have passed
// Validate.
func New(cfg Config) (Sender, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("mail: sender address: %w", err)
	}
	var s Sender
	switch cfg.Transport {
	case TransportSMTP:
		s = newSMTP(cfg, from)
	case TransportMailgun:
		s = newMailgun(cfg, from)
	case TransportSendGrid:
		s = newSendGrid(cfg, from)
	case TransportMailPace:
		s = newMailPace(cfg, from)
	case TransportScaleway:
		s = newScaleway(cfg, from)
	case TransportLettermint:
		s = newLettermint(cfg, from)
	default:
		return nil, fmt.Errorf("mail: unknown transport %q", cfg.Transport)
	}
	return NewCapped(s, cfg.MaxPerHour), nil
}

// checkMessage rejects what could change the message's headers: a recipient
// that is not one plain address, and line breaks in the subject.
func checkMessage(msg Message) error {
	if err := checkAddress(msg.To); err != nil {
		return err
	}
	if strings.ContainsAny(msg.Subject, "\r\n") {
		return fmt.Errorf("%w: line break in subject", ErrInvalidMessage)
	}
	return nil
}

// checkAddress accepts one bare address ("ada@example.com"), not a list and
// not a display name, without line breaks.
func checkAddress(addr string) error {
	if addr == "" || len(addr) > maxAddressLength || strings.ContainsAny(addr, "\r\n,;") {
		return fmt.Errorf("%w: recipient is not a plain address", ErrInvalidMessage)
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil || parsed.Address != addr {
		return fmt.Errorf("%w: recipient is not a plain address", ErrInvalidMessage)
	}
	return nil
}

// domainOf returns the part of addr after the last "@".
func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return addr
}
