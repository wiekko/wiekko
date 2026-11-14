package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/mail"
	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

type recordingSender struct {
	mu   sync.Mutex
	sent []mail.Message
	fail error
}

func (s *recordingSender) Send(_ context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.sent = append(s.sent, m)
	return nil
}
func (s *recordingSender) Name() string { return "recording" }

func decodeData(t *testing.T, body string) map[string]any {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return resp.Data
}

func TestInvitationAPIDelivery(t *testing.T) {
	admin := testutil.MakeUser(func(u *model.User) { u.IsAdmin = true })

	t.Run("email", func(t *testing.T) {
		sender := &recordingSender{}
		env := newTestEnvWithMail(t, sender)
		env.Invitations.CreateFunc = func(context.Context, *model.Invitation) error { return nil }
		ctrl := controller.NewInvitation(env.Services.Invitation)

		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com","delivery":"email"}`, admin)
		ctrl.Create(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
		}
		data := decodeData(t, w.Body.String())
		if data["delivery"] != "email" {
			t.Errorf("delivery = %v", data["delivery"])
		}
		// The link goes to the mailbox only.
		if _, ok := data["raw_token"]; ok {
			t.Error("raw_token returned for an emailed invitation")
		}
		if _, ok := data["url"]; ok {
			t.Error("url returned for an emailed invitation")
		}
		if len(sender.sent) != 1 || sender.sent[0].To != "new@test.com" {
			t.Errorf("sent = %+v", sender.sent)
		}
	})

	t.Run("link is the default", func(t *testing.T) {
		sender := &recordingSender{}
		env := newTestEnvWithMail(t, sender)
		env.Invitations.CreateFunc = func(context.Context, *model.Invitation) error { return nil }
		ctrl := controller.NewInvitation(env.Services.Invitation)

		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com"}`, admin)
		ctrl.Create(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
		}
		data := decodeData(t, w.Body.String())
		if data["delivery"] != "link" || data["raw_token"] == nil || !strings.HasPrefix(data["url"].(string), "https://nexmap.example.com/accept-invite/") {
			t.Errorf("data = %v", data)
		}
		if len(sender.sent) != 0 {
			t.Error("a link invitation sent an email")
		}
	})

	t.Run("email without mail", func(t *testing.T) {
		env := newTestEnv(t)
		ctrl := controller.NewInvitation(env.Services.Invitation)
		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com","delivery":"email"}`, admin)
		ctrl.Create(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "does not send email") {
			t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown delivery", func(t *testing.T) {
		env := newTestEnv(t)
		ctrl := controller.NewInvitation(env.Services.Invitation)
		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com","delivery":"pigeon"}`, admin)
		ctrl.Create(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_DELIVERY") {
			t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("send failure", func(t *testing.T) {
		sender := &recordingSender{fail: errors.New("relay down")}
		env := newTestEnvWithMail(t, sender)
		env.Invitations.CreateFunc = func(context.Context, *model.Invitation) error { return nil }
		var deleted bool
		env.Invitations.DeleteFunc = func(context.Context, string) error { deleted = true; return nil }
		ctrl := controller.NewInvitation(env.Services.Invitation)
		w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"new@test.com","delivery":"email"}`, admin)
		ctrl.Create(w, r)
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "MAIL_NOT_SENT") {
			t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "relay down") {
			t.Error("transport error leaked to the client")
		}
		if !deleted {
			t.Error("unsent invitation was not withdrawn")
		}
	})

	t.Run("recipient limit", func(t *testing.T) {
		env := newTestEnvWithMail(t, &recordingSender{})
		env.Invitations.CreateFunc = func(context.Context, *model.Invitation) error { return nil }
		ctrl := controller.NewInvitation(env.Services.Invitation)
		var last int
		for range 4 {
			w, r := authedRequest(http.MethodPost, "/admin/invitations", `{"email":"same@test.com","delivery":"email"}`, admin)
			ctrl.Create(w, r)
			last = w.Code
			if last == http.StatusTooManyRequests && w.Header().Get("Retry-After") == "" {
				t.Error("429 without Retry-After")
			}
		}
		if last != http.StatusTooManyRequests {
			t.Errorf("fourth invitation to one address: status %d, want 429", last)
		}
	})
}
