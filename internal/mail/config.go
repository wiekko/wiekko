package mail

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// Defaults for the mail configuration.
const (
	DefaultMaxPerHour     = 100
	DefaultSMTPTLS        = SMTPTLSStartTLS
	DefaultMailgunRegion  = "us"
	DefaultScalewayRegion = "fr-par"
)

// How the SMTP connection is secured.
const (
	SMTPTLSStartTLS = "starttls" // plain connection upgraded with STARTTLS, which is required
	SMTPTLSImplicit = "tls"      // TLS from the first byte (usually port 465)
	SMTPTLSNone     = "none"     // no TLS: only for a relay on the same host or network, without credentials
)

// scalewayRegions are the regions Transactional Email is available in.
var scalewayRegions = []string{"fr-par"}

// Config selects and configures the mail transport. The zero value (no
// Transport) means nexmap sends no mail.
type Config struct {
	Transport  string
	From       string // "nexmap <noreply@example.com>" or a bare address
	MaxPerHour int    // messages per hour across the instance

	SMTPHost     string
	SMTPPort     int
	SMTPTLS      string
	SMTPUsername string
	SMTPPassword string

	// APIKey is the key or token of an API transport.
	APIKey string

	MailgunDomain     string
	MailgunRegion     string
	ScalewayProjectID string
	ScalewayRegion    string
}

// Enabled reports whether a transport is configured.
func (c Config) Enabled() bool { return c.Transport != "" }

// Validate checks the configuration and fills in defaults. A configuration
// without a transport is valid: mail is off.
func (c *Config) Validate() error {
	c.Transport = strings.ToLower(strings.TrimSpace(c.Transport))
	if c.Transport == "" {
		return nil
	}

	from, err := mail.ParseAddress(c.From)
	if c.From == "" || err != nil || strings.ContainsAny(c.From, "\r\n") {
		return fmt.Errorf("mail: NEXMAP_MAIL_FROM must be an address like nexmap@example.com or \"nexmap <nexmap@example.com>\"")
	}
	if len(from.Address) > maxAddressLength {
		return fmt.Errorf("mail: NEXMAP_MAIL_FROM is too long")
	}
	if c.MaxPerHour == 0 {
		c.MaxPerHour = DefaultMaxPerHour
	}
	if c.MaxPerHour < 0 {
		return fmt.Errorf("mail: NEXMAP_MAIL_MAX_PER_HOUR must be a positive number")
	}

	switch c.Transport {
	case TransportSMTP:
		return c.validateSMTP()
	case TransportMailgun:
		if c.APIKey == "" || c.MailgunDomain == "" {
			return fmt.Errorf("mail: the mailgun transport needs NEXMAP_MAIL_API_KEY and NEXMAP_MAILGUN_DOMAIN")
		}
		c.MailgunRegion = strings.ToLower(strings.TrimSpace(c.MailgunRegion))
		if c.MailgunRegion == "" {
			c.MailgunRegion = DefaultMailgunRegion
		}
		if c.MailgunRegion != "us" && c.MailgunRegion != "eu" {
			return fmt.Errorf("mail: NEXMAP_MAILGUN_REGION must be us or eu")
		}
		if strings.ContainsAny(c.MailgunDomain, "/?#@ ") {
			return fmt.Errorf("mail: NEXMAP_MAILGUN_DOMAIN must be a domain name like mg.example.com")
		}
	case TransportSendGrid, TransportMailPace, TransportLettermint:
		if c.APIKey == "" {
			return fmt.Errorf("mail: the %s transport needs NEXMAP_MAIL_API_KEY", c.Transport)
		}
	case TransportScaleway:
		if c.APIKey == "" || c.ScalewayProjectID == "" {
			return fmt.Errorf("mail: the scaleway transport needs NEXMAP_MAIL_API_KEY (the secret key) and NEXMAP_SCALEWAY_PROJECT_ID")
		}
		c.ScalewayRegion = strings.ToLower(strings.TrimSpace(c.ScalewayRegion))
		if c.ScalewayRegion == "" {
			c.ScalewayRegion = DefaultScalewayRegion
		}
		if !slices.Contains(scalewayRegions, c.ScalewayRegion) {
			return fmt.Errorf("mail: NEXMAP_SCALEWAY_REGION must be one of %s", strings.Join(scalewayRegions, ", "))
		}
	default:
		return fmt.Errorf("mail: NEXMAP_MAIL_TRANSPORT %q is not one of smtp, mailgun, sendgrid, mailpace, scaleway, lettermint", c.Transport)
	}
	return nil
}

func (c *Config) validateSMTP() error {
	if c.SMTPHost == "" {
		return fmt.Errorf("mail: the smtp transport needs NEXMAP_SMTP_HOST")
	}
	c.SMTPTLS = strings.ToLower(strings.TrimSpace(c.SMTPTLS))
	if c.SMTPTLS == "" {
		c.SMTPTLS = DefaultSMTPTLS
	}
	switch c.SMTPTLS {
	case SMTPTLSStartTLS, SMTPTLSImplicit:
	case SMTPTLSNone:
		if c.SMTPUsername != "" || c.SMTPPassword != "" {
			return fmt.Errorf("mail: NEXMAP_SMTP_TLS=none cannot be combined with credentials; they would cross the network unencrypted")
		}
	default:
		return fmt.Errorf("mail: NEXMAP_SMTP_TLS must be starttls, tls or none")
	}
	if c.SMTPPort == 0 {
		c.SMTPPort = 587
		if c.SMTPTLS == SMTPTLSImplicit {
			c.SMTPPort = 465
		}
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return fmt.Errorf("mail: NEXMAP_SMTP_PORT must be a port number")
	}
	if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
		return fmt.Errorf("mail: NEXMAP_SMTP_USERNAME and NEXMAP_SMTP_PASSWORD must be set together")
	}
	return nil
}
