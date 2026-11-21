// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeSMTP is a minimal SMTP server for one test: it speaks just enough of
// the protocol for net/smtp and records what it was given.
type fakeSMTP struct {
	t          *testing.T
	ln         net.Listener
	tlsConf    *tls.Config
	offerTLS   bool
	implicit   bool
	rejectRcpt bool

	mu       sync.Mutex
	auth     string // decoded AUTH PLAIN payload
	from     string
	rcpt     []string
	data     string
	secured  bool // the dialogue ran over TLS when MAIL FROM came
	finished chan struct{}
}

// testCert returns a certificate for 127.0.0.1 and a pool that trusts it.
func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	srv.StartTLS()
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return srv.TLS.Certificates[0], pool
}

func startFakeSMTP(t *testing.T, f *fakeSMTP) (*fakeSMTP, *tls.Config) {
	t.Helper()
	cert, pool := testCert(t)
	f.t = t
	f.tlsConf = &tls.Config{Certificates: []tls.Certificate{cert}}
	f.finished = make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if f.implicit {
		ln = tls.NewListener(ln, f.tlsConf)
	}
	f.ln = ln
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f, &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	defer close(f.finished)
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	secured := f.implicit
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			ext := []string{"250-fake"}
			if f.offerTLS && !secured {
				ext = append(ext, "250-STARTTLS")
			}
			ext = append(ext, "250 AUTH PLAIN")
			for _, l := range ext {
				_ = tp.PrintfLine("%s", l)
			}
		case cmd == "STARTTLS":
			_ = tp.PrintfLine("220 go ahead")
			tlsConn := tls.Server(conn, f.tlsConf)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn, secured = tlsConn, true
			tp = textproto.NewConn(conn)
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			payload := strings.TrimSpace(line[len("AUTH PLAIN"):])
			dec, _ := base64.StdEncoding.DecodeString(payload)
			f.mu.Lock()
			f.auth = string(dec)
			f.mu.Unlock()
			_ = tp.PrintfLine("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			f.mu.Lock()
			f.from, f.secured = line[len("MAIL FROM:"):], secured
			f.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if f.rejectRcpt {
				_ = tp.PrintfLine("550 5.1.1 %s: recipient rejected", line[len("RCPT TO:"):])
				continue
			}
			f.mu.Lock()
			f.rcpt = append(f.rcpt, line[len("RCPT TO:"):])
			f.mu.Unlock()
			_ = tp.PrintfLine("250 ok")
		case cmd == "DATA":
			_ = tp.PrintfLine("354 go")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.data = string(b)
			f.mu.Unlock()
			_ = tp.PrintfLine("250 queued")
		case cmd == "QUIT":
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
	}
}

func smtpSenderFor(t *testing.T, f *fakeSMTP, clientTLS *tls.Config, mode, user, pass string) *smtpSender {
	t.Helper()
	cfg := Config{Transport: "smtp", From: "nexmap <noreply@example.com>", SMTPHost: "127.0.0.1", SMTPPort: f.port(), SMTPTLS: mode, SMTPUsername: user, SMTPPassword: pass}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	s := newSMTP(cfg, mustAddr(t, cfg.From))
	s.tlsConfig = clientTLS
	return s
}

func TestSMTPStartTLSWithAuth(t *testing.T) {
	f, clientTLS := startFakeSMTP(t, &fakeSMTP{offerTLS: true})
	s := smtpSenderFor(t, f, clientTLS, SMTPTLSStartTLS, "user", "secret")
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	<-f.finished
	if !f.secured {
		t.Error("message was sent before STARTTLS")
	}
	if f.auth != "\x00user\x00secret" {
		t.Errorf("auth = %q", f.auth)
	}
	if f.from != "<noreply@example.com>" || len(f.rcpt) != 1 || f.rcpt[0] != "<ada@example.com>" {
		t.Errorf("from %q, rcpt %v", f.from, f.rcpt)
	}
	if !strings.Contains(f.data, "Subject: =?utf-8?q?Your_invitation?=") && !strings.Contains(f.data, "Subject: Your invitation") {
		t.Errorf("data lacks subject:\n%s", f.data)
	}
}

func TestSMTPImplicitTLS(t *testing.T) {
	f, clientTLS := startFakeSMTP(t, &fakeSMTP{implicit: true})
	s := smtpSenderFor(t, f, clientTLS, SMTPTLSImplicit, "user", "secret")
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	<-f.finished
	if !f.secured || f.auth == "" {
		t.Errorf("secured=%v auth=%q", f.secured, f.auth)
	}
}

// STARTTLS mode refuses a server that does not offer it rather than sending
// the message (and the password) in the clear.
func TestSMTPRequiresStartTLS(t *testing.T) {
	f, clientTLS := startFakeSMTP(t, &fakeSMTP{offerTLS: false})
	s := smtpSenderFor(t, f, clientTLS, SMTPTLSStartTLS, "user", "secret")
	err := s.Send(context.Background(), testMsg)
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v, want STARTTLS refusal", err)
	}
	<-f.finished
	if f.auth != "" || f.data != "" {
		t.Error("credentials or data were sent without TLS")
	}
}

func TestSMTPNoTLSLocalRelay(t *testing.T) {
	f, _ := startFakeSMTP(t, &fakeSMTP{})
	s := smtpSenderFor(t, f, nil, SMTPTLSNone, "", "")
	if err := s.Send(context.Background(), testMsg); err != nil {
		t.Fatal(err)
	}
	<-f.finished
	if !strings.Contains(f.data, "accept-invite/tok") {
		t.Errorf("data:\n%s", f.data)
	}
}

// A rejection is reported by its code; the server's text (which quotes the
// address) stays out of the error.
func TestSMTPRejectionHidesAddress(t *testing.T) {
	f, _ := startFakeSMTP(t, &fakeSMTP{rejectRcpt: true})
	s := smtpSenderFor(t, f, nil, SMTPTLSNone, "", "")
	err := s.Send(context.Background(), testMsg)
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(550)) {
		t.Fatalf("err = %v, want code 550", err)
	}
	if strings.Contains(err.Error(), "ada@") {
		t.Errorf("error leaks the address: %v", err)
	}
}
