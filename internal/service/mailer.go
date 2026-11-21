// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"nexmap.net/internal/mail"
)

// Mail purposes, for the log. Email is limited to these two (SC-38).
const (
	mailPurposeInvitation    = "invitation"
	mailPurposePasswordReset = "password_reset"
)

const (
	// mailQueueSize bounds the messages waiting for the background sender. The
	// per-address and hourly limits keep it far from full; should it fill
	// anyway, further messages are dropped rather than piling up in memory.
	mailQueueSize = 256
	// mailSendTimeout bounds the delivery of one queued message.
	mailSendTimeout = time.Minute
	// mailDrainTimeout is how long queued messages may still be delivered
	// once the server shuts down.
	mailDrainTimeout = 10 * time.Second
)

// ErrMailDisabled is returned when an email is asked for but no mail
// transport is configured.
var ErrMailDisabled = errors.New("this installation does not send email; create an invitation link instead")

// Mailer composes nexmap's transactional emails and hands them to the
// configured transport. A nil Mailer, or one without a transport, sends
// nothing and reports Enabled() == false.
//
// Every message is fixed text: nothing a user typed (product names, display
// names, notes) ends up in an email, so nexmap cannot be used to carry
// someone else's message to an address of their choosing. The only variable
// parts are the recipient, the instance address, a role and the link.
type Mailer struct {
	sender    mail.Sender
	publicURL string
	queue     chan queuedMail
}

type queuedMail struct {
	purpose string
	msg     mail.Message
}

// NewMailer returns a Mailer sending through sender (nil: mail is off), with
// links pointing to publicURL.
func NewMailer(sender mail.Sender, publicURL string) *Mailer {
	m := &Mailer{sender: sender, publicURL: strings.TrimRight(publicURL, "/")}
	if sender != nil {
		m.queue = make(chan queuedMail, mailQueueSize)
	}
	return m
}

// Enabled reports whether nexmap can send email.
func (m *Mailer) Enabled() bool { return m != nil && m.sender != nil }

// link returns the absolute URL of path on this installation.
func (m *Mailer) link(path string) string { return m.publicURL + path }

// send delivers msg now and logs the outcome, without the recipient or the
// message (SC-47).
func (m *Mailer) send(ctx context.Context, purpose string, msg mail.Message) error {
	if !m.Enabled() {
		return ErrMailDisabled
	}
	if err := m.sender.Send(ctx, msg); err != nil {
		slog.WarnContext(ctx, "mail not sent", "purpose", purpose, "transport", m.sender.Name(), "err", err)
		return err
	}
	slog.InfoContext(ctx, "mail sent", "purpose", purpose, "transport", m.sender.Name())
	return nil
}

// enqueue hands msg to the background sender (Run) and returns at once, so
// the caller's response time does not depend on the mail transport. Used where
// timing must not tell anything: the password reset form answers alike for
// known and unknown accounts.
func (m *Mailer) enqueue(purpose string, msg mail.Message) {
	if !m.Enabled() {
		return
	}
	select {
	case m.queue <- queuedMail{purpose: purpose, msg: msg}:
	default:
		slog.Warn("mail queue full, message dropped", "purpose", purpose)
	}
}

// Run delivers queued messages until ctx is cancelled, then for up to
// mailDrainTimeout delivers what is still queued. The queue lives in memory
// only: a message still queued when the process stops is lost, and the person
// asks again.
func (m *Mailer) Run(ctx context.Context) {
	if !m.Enabled() {
		return
	}
	for {
		select {
		case q := <-m.queue:
			m.deliver(q)
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), mailDrainTimeout)
			defer cancel()
			m.drain(drainCtx)
			return
		}
	}
}

// drain delivers the queued messages until the queue is empty or ctx ends.
func (m *Mailer) drain(ctx context.Context) {
	for {
		select {
		case q := <-m.queue:
			m.deliver(q)
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

func (m *Mailer) deliver(q queuedMail) {
	ctx, cancel := context.WithTimeout(context.Background(), mailSendTimeout)
	defer cancel()
	_ = m.send(ctx, q.purpose, q.msg)
}

// recipientKey is the limiter key for an email address: a hash, so the
// limiter's memory holds no addresses.
func recipientKey(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "rcpt:" + hex.EncodeToString(h[:])
}

// invitationMessage is the invitation email. role is a validated role name
// (empty for an invitation to the installation only).
func (m *Mailer) invitationMessage(to, rawToken, role string, expires time.Time) mail.Message {
	what := "to create an account on Nexmap at " + m.publicURL
	if role != "" {
		what += " and to join a product there as " + role
	}
	text := "Hello,\n\n" +
		"you have been invited " + what + ".\n\n" +
		"To accept, open this link and choose a username and password:\n\n" +
		m.link("/accept-invite/"+rawToken) + "\n\n" +
		"The link works once and expires on " + expires.UTC().Format("2 January 2006 at 15:04 UTC") + ".\n\n" +
		"If you did not expect this invitation, ignore this email: no account is\n" +
		"created without the link.\n\n" +
		"-- \nNexmap\n"
	return mail.Message{To: to, Subject: "Your invitation to Nexmap", Text: text}
}

// passwordResetMessage is the password reset email.
func (m *Mailer) passwordResetMessage(to, rawToken string, ttl time.Duration) mail.Message {
	text := "Hello,\n\n" +
		"someone asked to reset the password of your Nexmap account at\n" +
		m.publicURL + ". If that was you, open this link to choose a new password:\n\n" +
		m.link("/reset-password/"+rawToken) + "\n\n" +
		"The link works once and expires in " + strconv.Itoa(int(ttl.Minutes())) + " minutes.\n\n" +
		"If you did not ask for this, ignore this email: your password stays as it\n" +
		"is until someone sets a new one through the link.\n\n" +
		"-- \nNexmap\n"
	return mail.Message{To: to, Subject: "Reset your Nexmap password", Text: text}
}
