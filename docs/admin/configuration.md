# Server configuration

`nexmapd` is configured through environment variables, most of which also have a command line flag.
Each setting is taken from the first source that has it: flag, then environment variable, then the
built-in default. There is no configuration file.

The container image sets some variables differently from the built-in defaults; both are listed below.

## Reference

### Network

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `NEXMAP_HOST` | `--host` | `localhost` | `0.0.0.0` | Address to listen on. `0.0.0.0` listens on all interfaces |
| `NEXMAP_PORT` | `--port` | `8080` | `8080` | Port to listen on |

`nexmapd` serves plain HTTP. Put it behind a reverse proxy that terminates TLS; see
[Security](security.md#tls-and-the-reverse-proxy).

### Database

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `NEXMAP_DB_DRIVER` | `--db-driver` | `sqlite` | `sqlite` | `sqlite` or `postgres` |
| `NEXMAP_DB_DSN` | `--db-dsn` | `nexmap.db` | `/data/nexmap.db` | For SQLite, the database file path. For PostgreSQL, a connection URL (`postgres://user:password@host:5432/nexmap?sslmode=require`) or a key/value string (`host=… user=… dbname=…`) |

SQLite suits small installations: one file, no separate server, run in WAL mode with a single connection.
PostgreSQL suits larger ones; `nexmapd` opens up to 25 connections. Either way, the database is created
or migrated automatically at startup, and the server refuses to start if it cannot reach it.

### Encryption

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `NEXMAP_ENCRYPTION_KEY` | — | none, **required** | Key ring for encrypting personal data, as JSON. See [Security](security.md#the-encryption-key) |

The server does not start without it.

### First administrator

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `NEXMAP_ADMIN_USER` | — | `admin` | Username of the administrator created on first start |
| `NEXMAP_ADMIN_PASSWORD` | — | generated | Password of that administrator |

These are only read when the database has no users yet, i.e. on the very first start. See
[Security](security.md#the-first-administrator).

### Sessions and passwords

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `NEXMAP_SESSION_TTL` | `--session-ttl` | `24h` | How long a web UI session lasts, as a Go duration (`8h`, `30m`, `168h`) |
| `NEXMAP_COOKIE_SECURE` | `--cookie-secure` | `true` | Mark session cookies `Secure`, so browsers send them over HTTPS only. Set to `false` only for local development over plain HTTP |
| `NEXMAP_PBKDF2_ITERATIONS` | `--pbkdf2-iterations` | `600000` | PBKDF2-SHA256 iterations for password hashes |

Each password hash records its own iteration count, so changing `NEXMAP_PBKDF2_ITERATIONS` applies to
passwords set from then on; existing passwords keep working.

### Attachments

| Variable | Flag | Default | Image | Meaning |
|---|---|---|---|---|
| `NEXMAP_UPLOAD_DIR` | `--upload-dir` | `./uploads` | `/data/uploads` | Directory for uploaded attachments |
| `NEXMAP_MAX_UPLOAD_BYTES` | `--max-upload-bytes` | `10485760` (10 MiB) | | Largest accepted attachment, in bytes |
| `NEXMAP_ALLOWED_UPLOAD_TYPES` | `--allowed-upload-types` | empty: all types | | Comma-separated presets and/or file extensions |

The presets are `images` (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`), `logs` (`.txt`, `.log`, `.json`,
`.xml`, `.csv`, `.md`, `.yaml`, `.yml`) and `documents` (`.pdf`). For example,
`NEXMAP_ALLOWED_UPLOAD_TYPES=images,logs,zip` allows those presets plus `.zip` files.

Besides the extension, the server checks the file content: a file named `.png` must really be an image,
and a text extension rejects content recognised as an archive, PDF or media file. This stops mislabelled
uploads but is not a malware scanner. Attachments are always served as downloads, never rendered or
executed by the browser.

### Logging

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `NEXMAP_LOGLEVEL` | `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `NEXMAP_LOGFILE` | `--log-file` | empty: stdout | Write the log to this file (appended, mode 0600) instead of stdout |

Logs are JSON, one entry per line.

### Development

| Variable | Flag | Default | Meaning |
|---|---|---|---|
| `NEXMAP_DEV_MODE` | — | `false` | Enables the component catalogue at `/dev/catalogue` and fills a newly created database with demo products and users. **Never enable it in production** |

## Value formats

- Boolean variables (`NEXMAP_COOKIE_SECURE`, `NEXMAP_DEV_MODE`) are true for `true` or `1`; any other
  value means false.
- An invalid duration or number (`NEXMAP_SESSION_TTL`, `NEXMAP_PBKDF2_ITERATIONS`,
  `NEXMAP_MAX_UPLOAD_BYTES`) is ignored and the default is used, without a warning. Check the values
  carefully.
- An unknown log level or database driver stops the server with an error.

## Health check

`nexmapd healthcheck` requests `/api/v1/health` from the server running with the same configuration and
exits with 0 if it answers. The container image uses it as its `HEALTHCHECK`; outside a container, the
endpoint can be polled directly.

## Minimal production setup

```sh
NEXMAP_ENCRYPTION_KEY='[{"id":"2026-01","key":"<base64 key>"}]'
NEXMAP_ADMIN_USER=jdoe
NEXMAP_ADMIN_PASSWORD='<long random password>'
# for PostgreSQL instead of SQLite:
NEXMAP_DB_DRIVER=postgres
NEXMAP_DB_DSN='postgres://nexmap:<password>@db:5432/nexmap?sslmode=require'
```

Behind a TLS-terminating proxy, the remaining defaults are suitable for production.
