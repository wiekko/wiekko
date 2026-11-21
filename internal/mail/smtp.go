// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

// smtpTimeout bounds one whole delivery: connecting, the dialogue and the data.
const smtpTimeout = 30 * time.Second

type smtpSender struct {
	host     string
	port     int
	mode     string
	username string
	password string
	from     *mail.Address

	// tlsConfig overrides the TLS settings; tests use it to trust their own
	// server. nil: verify the server against the system roots.
	tlsConfig *tls.Config
}

func newSMTP(cfg Config, from *mail.Address) *smtpSender {
	return &smtpSender{
		host:     cfg.SMTPHost,
		port:     cfg.SMTPPort,
		mode:     cfg.SMTPTLS,
		username: cfg.SMTPUsername,
		password: cfg.SMTPPassword,
		from:     from,
	}
}

func (s *smtpSender) Name() string { return TransportSMTP }

func (s *smtpSender) tlsConf() *tls.Config {
	if s.tlsConfig != nil {
		return s.tlsConfig.Clone()
	}
	return &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
}

func (s *smtpSender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	data, err := buildMessage(s.from, msg, time.Now())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{}
	var conn net.Conn
	if s.mode == SMTPTLSImplicit {
		td := &tls.Dialer{NetDialer: dialer, Config: s.tlsConf()}
		conn, err = td.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: connect: %w", err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	// Close the connection when the caller gives up early.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return smtpError("greeting", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Hello(helloName(s.from.Address)); err != nil {
		return smtpError("hello", err)
	}
	if s.mode == SMTPTLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("smtp: the server does not offer STARTTLS, which WIEKKO_SMTP_TLS=starttls requires")
		}
		if err := c.StartTLS(s.tlsConf()); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	if s.username != "" {
		// PlainAuth refuses to send the password over a connection without TLS.
		if err := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return smtpError("auth", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return smtpError("mail from", err)
	}
	if err := c.Rcpt(msg.To); err != nil {
		return smtpError("rcpt to", err)
	}
	w, err := c.Data()
	if err != nil {
		return smtpError("data", err)
	}
	if _, err := w.Write(data); err != nil {
		return smtpError("data", err)
	}
	if err := w.Close(); err != nil {
		return smtpError("data", err)
	}
	_ = c.Quit()
	return nil
}

// smtpError keeps the stage and the server's reply code, not its text: a
// rejection often quotes the recipient's address.
func smtpError(stage string, err error) error {
	var tpErr *textproto.Error
	if errors.As(err, &tpErr) {
		return fmt.Errorf("smtp: %s: server replied %d", stage, tpErr.Code)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return fmt.Errorf("smtp: %s: %w", stage, netErr)
	}
	return fmt.Errorf("smtp: %s failed", stage)
}

// helloName is the name wiekko greets the relay with: the domain of its own
// sender address, which relays accept more readily than "localhost".
func helloName(from string) string {
	if d := domainOf(from); d != "" && d != from {
		return d
	}
	return "localhost"
}

// buildMessage renders msg as an RFC 5322 message with CRLF line endings.
// Header values come from the configured sender and from checkMessage-ed
// fields only; the subject is encoded, the body quoted-printable.
func buildMessage(from *mail.Address, msg Message, now time.Time) ([]byte, error) {
	if err := checkMessage(msg); err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("mail: message id: %w", err)
	}

	var b bytes.Buffer
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", from.String())
	header("To", msg.To)
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domainOf(from.Address)+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	// Ask auto-responders (vacation notices, out-of-office) not to answer.
	header("Auto-Submitted", "auto-generated")
	header("X-Auto-Response-Suppress", "All")
	b.WriteString("\r\n")

	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(msg.Text)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	b.WriteString("\r\n")
	return b.Bytes(), nil
}
