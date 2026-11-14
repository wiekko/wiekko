# Email decisions

Decisions behind transactional email (AC-104, SC-109 – SC-111): what was chosen, and why. How to configure
it is described in [Configuration](../admin/configuration.md#mail), what it guards against in
[Security](../admin/security.md#email).

## Mail is optional

Without a configured transport nexmap behaves as before: invitations are links the inviter passes on, and
administrators set new passwords. Small installations need no mail service, and nothing else depends on
it. The password reset pages answer 404 without mail, and the sign-in page keeps pointing to an
administrator.

## Two purposes, fixed text

Email exists for invitations and password resets only (SC-111). Both messages are fixed text; the only
variable parts are the recipient, the installation's address, the link and its expiry, and for an
invitation the role, which is one of a few fixed words. Product names and display names stay out of the
email, even though a product name would help the recipient recognise an invitation: a product owner can
send invitations to any address, and a name of their choosing would let them put a message of their own
into an email sent from the installation's domain. The product name is shown on the page the link
opens.

## Invitations: email or link, the inviter decides

With mail, the inviter chooses whether nexmap emails the invitation or shows the link. Links stay
useful for people reached through chat or forum messages. An emailed link is never shown to the
inviter, so an account created from it proves that its owner reads that mailbox. The API keeps `link` as
its default, so existing scripts keep working, and only answers with the token for links.

An invitation email is sent while the inviter waits: the inviter is signed in, and should learn right
away when the email could not be sent. The invitation is then withdrawn, so no usable invitation is left
that nobody received.

## Password reset: nothing to learn from the form

Asking for a reset link is the one public action that sends email. The form therefore:

- sends only to the address stored on an existing, active account, never to an address typed in, so it
  cannot reach a stranger's mailbox;
- answers the same for every input, and hands the email to a background queue, so neither the answer
  nor its timing tells whether an account exists;
- is limited per client address (which is reported, since it depends on the asker only) and per
  account (which is not, since that would reveal the account).

There is no API endpoint for it: the API is for authenticated clients, and a public endpoint that sends
email would only add a target.

A reset link works once and for an hour, asking again replaces it, and only its hash is stored. Using
it ends all sessions and leaves two-factor authentication on: a reset proves control of the mailbox, not
of the second factor. The password policy is checked before the link is used up, so a rejected password
does not cost the link.

## Links come from `NEXMAP_PUBLIC_URL`

Invitation links shown in the browser may still be built from the request when no public URL is set.
Links in emails may not: on the public reset form, the `Host` header is whatever the requester sends, and
a reset link pointing to their server would hand them the token. Mail therefore requires
`NEXMAP_PUBLIC_URL`, and the server does not start without it.

## Limits in memory, plus a cap

The limits per inviter, per recipient, per client address and per account live in memory like the login
limits: nexmap is a single process, and a restart clears them. Above them sits a cap on all emails per
hour, so that a mistake or an unforeseen path costs at most that many emails rather than the sending
domain's reputation.

The reset queue lives in memory as well, without persistence (twelve-factor: the process keeps no state
of its own). A reset email still queued when the process stops is lost; the user asks again.

## No SDKs

Each mailer API needs a single call: one plain-text message to one recipient. The transports are written
on `net/http` and `net/smtp` instead of the providers' SDKs, which would bring in far more code and
dependencies than that call, each with its own updates and vulnerability reports. Writing them by hand
also keeps the rules the same for every transport: timeouts, no redirects (they would carry the API key
elsewhere), no response bodies or server replies in errors (they may quote the recipient), tracking
switched off (it would send every link through the provider), and STARTTLS required for SMTP unless
explicitly turned off for a local relay. If a provider's API changes, its transport changes, without
touching the others.

Services without a transport of their own (Amazon SES, Postmark, Brevo and others) are reached through
their SMTP relays.
