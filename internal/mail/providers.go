package mail

import (
	"context"
	"net/http"
	"net/mail"
	"net/url"

	"github.com/google/uuid"
)

// The mailer APIs below each need one call: send one plain-text message to
// one recipient. Each is a few lines on net/http rather than the provider's
// SDK, which would bring in far more code (and dependencies) than that call.
//
// Click and open tracking are switched off wherever the API has a switch for
// it: tracking rewrites links through the provider's redirector, which would
// hand it every invitation and reset token.

// mailgun: https://documentation.mailgun.com/docs/mailgun/api-reference/send/mailgun/messages
type mailgunSender struct {
	client  *http.Client
	baseURL string
	domain  string
	apiKey  string
	from    *mail.Address
}

func newMailgun(cfg Config, from *mail.Address) *mailgunSender {
	base := "https://api.mailgun.net"
	if cfg.MailgunRegion == "eu" {
		base = "https://api.eu.mailgun.net"
	}
	return &mailgunSender{client: newAPIClient(), baseURL: base, domain: cfg.MailgunDomain, apiKey: cfg.APIKey, from: from}
}

func (s *mailgunSender) Name() string { return TransportMailgun }

func (s *mailgunSender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	form := url.Values{
		"from":              {s.from.String()},
		"to":                {msg.To},
		"subject":           {msg.Subject},
		"text":              {msg.Text},
		"o:tracking":        {"no"},
		"o:tracking-clicks": {"no"},
		"o:tracking-opens":  {"no"},
	}
	return do(ctx, s.client, apiRequest{
		name:        TransportMailgun,
		url:         s.baseURL + "/v3/" + url.PathEscape(s.domain) + "/messages",
		contentType: "application/x-www-form-urlencoded",
		body:        []byte(form.Encode()),
		basicUser:   "api",
		basicPass:   s.apiKey,
		okStatus:    []int{http.StatusOK},
	})
}

// sendgrid: https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send
type sendGridSender struct {
	client  *http.Client
	baseURL string
	apiKey  string
	from    *mail.Address
}

func newSendGrid(cfg Config, from *mail.Address) *sendGridSender {
	return &sendGridSender{client: newAPIClient(), baseURL: "https://api.sendgrid.com", apiKey: cfg.APIKey, from: from}
}

func (s *sendGridSender) Name() string { return TransportSendGrid }

func (s *sendGridSender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	from := map[string]string{"email": s.from.Address}
	if s.from.Name != "" {
		from["name"] = s.from.Name
	}
	body := map[string]any{
		"personalizations": []any{map[string]any{"to": []any{map[string]string{"email": msg.To}}}},
		"from":             from,
		"subject":          msg.Subject,
		"content":          []any{map[string]string{"type": "text/plain", "value": msg.Text}},
		"tracking_settings": map[string]any{
			"click_tracking": map[string]bool{"enable": false, "enable_text": false},
			"open_tracking":  map[string]bool{"enable": false},
		},
	}
	return do(ctx, s.client, apiRequest{
		name:        TransportSendGrid,
		url:         s.baseURL + "/v3/mail/send",
		contentType: "application/json",
		body:        jsonBody(body),
		header:      http.Header{"Authorization": {"Bearer " + s.apiKey}},
		okStatus:    []int{http.StatusOK, http.StatusAccepted},
	})
}

// mailpace: https://docs.mailpace.com/reference/send
type mailPaceSender struct {
	client  *http.Client
	baseURL string
	token   string
	from    *mail.Address
}

func newMailPace(cfg Config, from *mail.Address) *mailPaceSender {
	return &mailPaceSender{client: newAPIClient(), baseURL: "https://app.mailpace.com", token: cfg.APIKey, from: from}
}

func (s *mailPaceSender) Name() string { return TransportMailPace }

func (s *mailPaceSender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	body := map[string]string{
		"from":     s.from.String(),
		"to":       msg.To,
		"subject":  msg.Subject,
		"textbody": msg.Text,
	}
	return do(ctx, s.client, apiRequest{
		name:        TransportMailPace,
		url:         s.baseURL + "/api/v1/send",
		contentType: "application/json",
		body:        jsonBody(body),
		header: http.Header{
			"MailPace-Server-Token": {s.token},
			// A retry with the same key cannot deliver the message twice.
			"Idempotency-Key": {uuid.NewString()},
		},
		okStatus: []int{http.StatusOK},
	})
}

// scaleway: Transactional Email API, CreateEmail
// (https://www.scaleway.com/en/developers/api/transactional-email/).
type scalewaySender struct {
	client    *http.Client
	baseURL   string
	region    string
	projectID string
	secretKey string
	from      *mail.Address
}

func newScaleway(cfg Config, from *mail.Address) *scalewaySender {
	return &scalewaySender{
		client:    newAPIClient(),
		baseURL:   "https://api.scaleway.com",
		region:    cfg.ScalewayRegion,
		projectID: cfg.ScalewayProjectID,
		secretKey: cfg.APIKey,
		from:      from,
	}
}

func (s *scalewaySender) Name() string { return TransportScaleway }

func (s *scalewaySender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	from := map[string]string{"email": s.from.Address}
	if s.from.Name != "" {
		from["name"] = s.from.Name
	}
	body := map[string]any{
		"from":       from,
		"to":         []any{map[string]string{"email": msg.To}},
		"subject":    msg.Subject,
		"text":       msg.Text,
		"project_id": s.projectID,
	}
	return do(ctx, s.client, apiRequest{
		name:        TransportScaleway,
		url:         s.baseURL + "/transactional-email/v1alpha1/regions/" + url.PathEscape(s.region) + "/emails",
		contentType: "application/json",
		body:        jsonBody(body),
		header:      http.Header{"X-Auth-Token": {s.secretKey}},
		okStatus:    []int{http.StatusOK},
	})
}

// lettermint: https://lettermint.co/docs/api-reference/sending/send
type lettermintSender struct {
	client  *http.Client
	baseURL string
	token   string
	from    *mail.Address
}

func newLettermint(cfg Config, from *mail.Address) *lettermintSender {
	return &lettermintSender{client: newAPIClient(), baseURL: "https://api.lettermint.co", token: cfg.APIKey, from: from}
}

func (s *lettermintSender) Name() string { return TransportLettermint }

func (s *lettermintSender) Send(ctx context.Context, msg Message) error {
	if err := checkMessage(msg); err != nil {
		return err
	}
	body := map[string]any{
		"from":    s.from.String(),
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"text":    msg.Text,
		"settings": map[string]bool{
			"track_opens":  false,
			"track_clicks": false,
		},
	}
	return do(ctx, s.client, apiRequest{
		name:        TransportLettermint,
		url:         s.baseURL + "/v1/send",
		contentType: "application/json",
		body:        jsonBody(body),
		header: http.Header{
			"X-Lettermint-Token": {s.token},
			"Idempotency-Key":    {uuid.NewString()},
		},
		okStatus: []int{http.StatusOK, http.StatusAccepted},
	})
}
