# Security

This page covers what an administrator has to set up and look after to run nexmap safely. Every setting
mentioned is described in the [configuration reference](configuration.md).

## TLS and the reverse proxy

`nexmapd` speaks plain HTTP. Run it behind a reverse proxy that terminates TLS (on Coolify, the built-in
Traefik does this), and do not expose its port to the network directly:

- Leave `NEXMAP_COOKIE_SECURE` at `true`. Browsers then send the session cookie over HTTPS only.
- The server records the `X-Forwarded-For` header, without checking it, as the client address of
  sessions and failed logins. Only a proxy in front of the server makes that address trustworthy, and the
  proxy should replace a header sent by the client rather than append to it.

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
with its key protects nothing.

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
- **Sessions** expire after `NEXMAP_SESSION_TTL` (24 hours by default).

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
