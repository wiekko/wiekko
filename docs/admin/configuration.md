# Server configuration

`wiekkod` is configured through environment variables, most of which also have a command line flag.
Each setting is taken from the first source that has it: flag, then environment variable, then the
built-in default. There is no configuration file.

The container image sets some variables differently from the built-in defaults; both are listed below.

## Reference

### Network

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `WIEKKO_HOST` | `--host` | `localhost` | `0.0.0.0` | Address to listen on. `0.0.0.0` listens on all interfaces |
| `WIEKKO_PORT` | `--port` | `8080` | `8080` | Port to listen on |
| `WIEKKO_PUBLIC_URL` | `--public-url` | empty | | The address users open Wiekko at, such as `https://wiekko.example.com`. Used for the links Wiekko hands out (invitations, password resets). Required when [mail](#mail) is enabled |
| `WIEKKO_TRUSTED_PROXIES` | `--trusted-proxies` | loopback and private networks | | Addresses and CIDR ranges of the reverse proxies whose `X-Forwarded-For` header is believed, comma-separated. `none` (or empty) trusts no proxy |

Without `WIEKKO_PUBLIC_URL`, invitation links are built from the address the inviter's browser used,
taking the scheme from the proxy's `X-Forwarded-Proto` header. That is right behind a typical proxy, but
set the variable if Wiekko is reachable under more than one name, or if links come out with `http://`.
The server does not start if the value is not an absolute `http` or `https` URL.

`WIEKKO_TRUSTED_PROXIES` decides whose word Wiekko takes for a client's address, which the login limits
count by. The default (`127.0.0.0/8`, `::1`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`)
fits a proxy on the same host or container network. Narrow it to your proxy's address if other machines
on those networks can reach `wiekkod` directly. The server does not start if an entry is not an IP
address or CIDR range. See [Security](security.md#tls-and-the-reverse-proxy).

`wiekkod` serves plain HTTP. Put it behind a reverse proxy that terminates TLS; see
[Security](security.md#tls-and-the-reverse-proxy).

### Database

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `WIEKKO_DB_DRIVER` | `--db-driver` | `sqlite` | `sqlite` | `sqlite` or `postgres` |
| `WIEKKO_DB_DSN` | `--db-dsn` | `wiekko.db` | `/data/wiekko.db` | For SQLite, the database file path. For PostgreSQL, a connection URL (`postgres://user:password@host:5432/wiekko?sslmode=require`) or a key/value string (`host=… user=… dbname=…`) |

SQLite suits small installations: one file, no separate server, run in WAL mode with a single connection.
PostgreSQL suits larger ones; `wiekkod` opens up to 25 connections. Either way, the database is created
or migrated automatically at startup, and the server refuses to start if it cannot reach it.

### Encryption

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `WIEKKO_ENCRYPTION_KEY` | — | none, **required** | Key ring for encrypting personal data, as JSON. See [Security](security.md#the-encryption-key) |

The server does not start without it.

### First administrator

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `WIEKKO_ADMIN_USER` | — | `admin` | Username of the administrator created on first start |
| `WIEKKO_ADMIN_PASSWORD` | — | generated | Password of that administrator |

These are only read when the database has no users yet, i.e. on the very first start. See
[Security](security.md#the-first-administrator).

### Sessions and passwords

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `WIEKKO_SESSION_TTL` | `--session-ttl` | `24h` | How long a web UI session lasts, as a Go duration (`8h`, `30m`, `168h`) |
| `WIEKKO_COOKIE_SECURE` | `--cookie-secure` | `true` | Mark session cookies `Secure`, so browsers send them over HTTPS only. Set to `false` only for local development over plain HTTP |
| `WIEKKO_PBKDF2_ITERATIONS` | `--pbkdf2-iterations` | `600000` | PBKDF2-SHA256 iterations for password hashes |

Each password hash records its own iteration count, so changing `WIEKKO_PBKDF2_ITERATIONS` applies to
passwords set from then on; existing passwords keep working.

### Attachments

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `WIEKKO_UPLOAD_DIR` | `--upload-dir` | `./uploads` | `/data/uploads` | Directory for uploaded attachments |
| `WIEKKO_MAX_UPLOAD_BYTES` | `--max-upload-bytes` | `10485760` (10 MiB) | | Largest accepted attachment, in bytes |
| `WIEKKO_ALLOWED_UPLOAD_TYPES` | `--allowed-upload-types` | empty: all types | | Comma-separated presets and/or file extensions |

The presets are `images` (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`), `logs` (`.txt`, `.log`, `.json`,
`.xml`, `.csv`, `.md`, `.yaml`, `.yml`) and `documents` (`.pdf`). For example,
`WIEKKO_ALLOWED_UPLOAD_TYPES=images,logs,zip` allows those presets plus `.zip` files.

Besides the extension, the server checks the file content: a file named `.png` must really be an image,
and a text extension rejects content recognised as an archive, PDF or media file. This stops mislabelled
uploads but is not a malware scanner. Attachments are always served as downloads, never rendered or
executed by the browser.

### Mail

Wiekko can send two kinds of email, and no others: invitations and password reset links. Mail is
optional. Without it, invitations are links that the inviter passes on, and an administrator sets a new
password for someone who forgot theirs.

| Variable | Default | Meaning |
|---|---|---|
| `WIEKKO_MAIL_TRANSPORT` | empty: no mail | `smtp`, `mailgun`, `sendgrid`, `mailpace`, `scaleway` or `lettermint` |
| `WIEKKO_MAIL_FROM` | none, required with a transport | Sender, such as `wiekko@example.com` or `"Wiekko <wiekko@example.com>"`. Its domain must be one your mail service may send for |
| `WIEKKO_MAIL_MAX_PER_HOUR` | `100` | Most emails the server sends in an hour, all kinds together. A safety net; the limits below normally apply first |
| `WIEKKO_SMTP_HOST` | | SMTP relay (transport `smtp`) |
| `WIEKKO_SMTP_PORT` | `587`, or `465` with `tls` | |
| `WIEKKO_SMTP_TLS` | `starttls` | `starttls`: the relay must offer STARTTLS, or nothing is sent. `tls`: TLS from the start (port 465). `none`: no encryption, only for a relay on the same host or network, and only without credentials |
| `WIEKKO_SMTP_USERNAME`, `WIEKKO_SMTP_PASSWORD` | empty: no login | Credentials for the relay (AUTH PLAIN, over TLS only) |
| `WIEKKO_MAIL_API_KEY` | | Key or token of the mailer API: the Mailgun API key, the SendGrid API key, the MailPace server token, the Scaleway secret key or the Lettermint project token |
| `WIEKKO_MAILGUN_DOMAIN` | | Sending domain at Mailgun (transport `mailgun`) |
| `WIEKKO_MAILGUN_REGION` | `us` | `us` or `eu` |
| `WIEKKO_SCALEWAY_PROJECT_ID` | | Project of the Transactional Email domain (transport `scaleway`) |
| `WIEKKO_SCALEWAY_REGION` | `fr-par` | Region of Scaleway Transactional Email |

The mail settings have no command line flags: they include secrets, and a process's command line is
visible to other users of the machine. The server does not start if the mail configuration is incomplete
or invalid, or if a transport is set without `WIEKKO_PUBLIC_URL`. The links in emails are always built
from `WIEKKO_PUBLIC_URL`, never from the request, so nobody can make Wiekko send a reset link that points
to another site.

Services that are not in the list can be used through their SMTP relay, such as Amazon SES, Postmark or
Brevo. Scaleway, MailPace and Lettermint can also be reached by SMTP, if you prefer that.

Examples:

```sh
# An SMTP relay with STARTTLS on port 587
WIEKKO_MAIL_TRANSPORT=smtp
WIEKKO_MAIL_FROM='Wiekko <wiekko@example.com>'
WIEKKO_SMTP_HOST=smtp.example.com
WIEKKO_SMTP_USERNAME=wiekko
WIEKKO_SMTP_PASSWORD='<password>'

# Scaleway Transactional Email through its API
WIEKKO_MAIL_TRANSPORT=scaleway
WIEKKO_MAIL_FROM=wiekko@example.com
WIEKKO_MAIL_API_KEY='<secret key>'
WIEKKO_SCALEWAY_PROJECT_ID='<project id>'
```

At startup the log says `mail enabled` with the transport, or `mail disabled`. Each email is logged as
`mail sent` or `mail not sent` with its purpose (`invitation` or `password_reset`), never with the
address or the link. See [Security](security.md#email) for what the emails contain and how they are
limited.

### Logging

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `WIEKKO_LOGLEVEL` | `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `WIEKKO_LOGFILE` | `--log-file` | empty: stdout | Write the log to this file (appended, mode 0600) instead of stdout |

Logs are JSON, one entry per line.

### Development

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `WIEKKO_DEV_MODE` | — | `false` | Enables the component catalogue at `/dev/catalogue` and fills a newly created database with demo products and users. **Never enable it in production** |

## Value formats

- Boolean variables (`WIEKKO_COOKIE_SECURE`, `WIEKKO_DEV_MODE`) are true for `true` or `1`; any other
  value means false.
- An invalid duration or number (`WIEKKO_SESSION_TTL`, `WIEKKO_PBKDF2_ITERATIONS`,
  `WIEKKO_MAX_UPLOAD_BYTES`) is ignored and the default is used, without a warning. Check the values
  carefully.
- An unknown log level or database driver stops the server with an error.
- The mail settings are checked strictly: an invalid number, port or option stops the server with an
  error.

## Health check

`wiekkod healthcheck` requests `/api/v1/health` from the server running with the same configuration and
exits with 0 if it answers. The container image uses it as its `HEALTHCHECK`; outside a container, the
endpoint can be polled directly.

## Minimal production setup

```sh
WIEKKO_ENCRYPTION_KEY='[{"id":"2026-01","key":"<base64 key>"}]'
WIEKKO_ADMIN_USER=jdoe
WIEKKO_ADMIN_PASSWORD='<long random password>'
# for PostgreSQL instead of SQLite:
WIEKKO_DB_DRIVER=postgres
WIEKKO_DB_DSN='postgres://wiekko:<password>@db:5432/wiekko?sslmode=require'
```

Behind a TLS-terminating proxy, the remaining defaults are suitable for production.
