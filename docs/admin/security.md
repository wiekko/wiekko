# Security

This page covers what an administrator has to set up and look after to run nexmap safely. Every setting
mentioned is described in the [configuration reference](configuration.md).

## TLS and the reverse proxy

`nexmapd` speaks plain HTTP. Run it behind a reverse proxy that terminates TLS (on Coolify, the built-in
Traefik does this), and do not expose its port to the network directly:

- Leave `NEXMAP_COOKIE_SECURE` at `true`. Browsers then send the session cookie over HTTPS only.
- The server records the `X-Forwarded-For` header, without checking it, as the client address of
  sessions and failed logins. Only a proxy in front of the server makes that address trustworthy, and the
  proxy should replace a header sent by the client rather than append to it. The login limits below
  count by the last address in the header, the one the proxy added.

## The encryption key

Personal data (email addresses and display names) is encrypted in the database with AES-256-GCM.
Usernames are treated as pseudonyms and are not encrypted. The key comes from `NEXMAP_ENCRYPTION_KEY`,
a JSON list of keys with an ID and a base64-encoded 32-byte key each:

```json
[{"id": "2026-01", "key": "<32 random bytes, base64-encoded>"}]
```

To generate one:

```sh
printf '[{"id":"%s","key":"%s"}]\n' "$(date +%Y-%m)" "$(openssl rand -base64 32)"
```

Treat the key like a database password: keep it in your platform's secret store, not in a repository.

**Back up the key separately from the database.** Without it, the personal data in a database backup
cannot be decrypted, and nexmap has no way to recover it. Conversely, a database backup stored together
with its key protects nothing. See [Backup and restore](backup.md).

### Rotating the key

The first key in the list encrypts new data; the others are kept only to decrypt data written with them.
To switch to a new key, put it first and keep every older key in the list:

```json
[{"id": "2027-01", "key": "<new key>"}, {"id": "2026-01", "key": "<old key>"}]
```

Never remove an old key: data encrypted with it would become unreadable.

Rotation is incomplete today, so rotate only when you have a reason to, such as a leaked key:

- Existing data is not re-encrypted. It stays encrypted with the old key until the record is next changed,
  so a leaked old key still exposes it.
- Email addresses also get a lookup hash derived from the first key, which is how the database prevents
  two accounts from sharing an address. After a rotation, that check only compares addresses hashed with
  the same key, so a duplicate of an address stored under the old key is not caught.

## The first administrator

On the very first start, when the database has no users, `nexmapd` creates an administrator from
`NEXMAP_ADMIN_USER` and `NEXMAP_ADMIN_PASSWORD`. If the username is unset it is `admin`; if the password is
unset, a random one is generated and **written to the log**.

Set both variables for the first start. If you rely on the generated password, read it from the log, sign
in, change the password right away, and keep in mind that the log still contains it. The administrator's
email address starts out as `admin@localhost`; change it on the profile page.

Once users exist, both variables are ignored and can be removed.

## Accounts and sign-in

- **Two-factor authentication:** users can set up TOTP on their profile page. *Enforce 2FA* in the
  instance settings (`/admin/settings`) requires it for everyone. An administrator can reset a user's
  2FA from the user's detail page, after which that user can sign in with the password alone until they
  set it up again.
- **Passwords** are stored as salted PBKDF2-SHA256 hashes (`NEXMAP_PBKDF2_ITERATIONS`). The instance
  settings can require uppercase and lowercase letters, digits and special characters in new passwords.
- **Email is not implemented yet**: there are no password reset or invitation emails. Administrators set
  new passwords for users from the user's detail page.
- **Sessions** expire after `NEXMAP_SESSION_TTL` (24 hours by default). Changing a password ends the
  user's other sessions, keeping only the one the change was made from; an administrator setting a new
  password ends all of that user's sessions. Personal access tokens stay valid either way; revoke them
  separately if they may have leaked. Locking an account blocks its sessions and tokens at once.
- **Failed logins are limited** per client address: after 5 wrong passwords for the same username, or
  20 for any usernames, within 15 minutes, that address has to wait until the 15 minutes are over. The
  web form says so and the API answers `429 TOO_MANY_ATTEMPTS` with a `Retry-After` header. A correct
  password clears the count for that username. There is no limit per account across addresses, so nobody
  can lock another user out by guessing their password. The counters live in memory and reset when the
  server restarts.

## New accounts

People get an account in one of two ways:

- **Invitation:** a product owner invites someone to their product with a role (on the product's Team
  page), or an administrator invites someone to the installation (`/admin/invitations`). nexmap shows a
  link (built from [`NEXMAP_PUBLIC_URL`](configuration.md#network) if set), which the inviter passes on. Whoever holds the link can create the account, so treat it as a
  secret: it works once, expires after 7 days, and is never written to the log. Accounts created from an
  invitation skip moderation.
- **Self-registration**, if *self-registration* is enabled in the instance settings: anyone who can reach
  the server can create an account on `/register`. With *moderation* enabled, such accounts wait for an
  administrator's approval under `/admin/users` before they can sign in. A self-registered account
  belongs to no product until a product owner adds it.

Only product owners (and administrators) can invite to a product, and only administrators can invite to
the installation without a product.

## Access tokens

Personal access tokens, which the CLI, CI jobs and AI agents use, carry every right of the user who
created them; they cannot be restricted to reading or to one product yet. For automation, create a
dedicated user with the lowest product role that does the job (usually *tester* for CI), and revoke
tokens that are no longer used under *API Tokens* (`/user/tokens`).

## Attachments

By default, every file type can be uploaded. To restrict this, set `NEXMAP_ALLOWED_UPLOAD_TYPES`; the
server then also checks that the content matches the extension. Attachments are always served as
downloads and never rendered by the browser, but nexmap does not scan them for malware.

## Development mode

`NEXMAP_DEV_MODE` fills a new database with demo users whose password is publicly known (it is in the
source code and the log). Never enable it on a server that others can reach.
