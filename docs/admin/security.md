# Security

This page covers what an administrator has to set up and look after to run nexmap safely. Every setting
mentioned is described in the [configuration reference](configuration.md).

## TLS and the reverse proxy

`nexmapd` speaks plain HTTP. Run it behind a reverse proxy that terminates TLS (on Coolify, the built-in
Traefik does this), and do not expose its port to the network directly:

- Leave `NEXMAP_COOKIE_SECURE` at `true`. Browsers then send the session cookie over HTTPS only, and
  the server sends `Strict-Transport-Security`, so browsers that have visited once refuse plain HTTP for
  a year.
- The client address of sessions, failed logins and the limits below comes from the `X-Forwarded-For`
  header, but only when the request arrives from a trusted proxy
  ([`NEXMAP_TRUSTED_PROXIES`](configuration.md#network), by default loopback and private networks).
  A request from any other address is taken to come from that address, whatever its header says. From a
  trusted proxy the header is read from the end, skipping trusted proxies, so entries a client wrote
  itself do not count, whether the proxy replaces the header or appends to it.
- If machines other than the proxy can reach `nexmapd` on a private network, set
  `NEXMAP_TRUSTED_PROXIES` to the proxy's address. Otherwise they can claim any client address and
  evade the limits.

- The server refuses form posts and API calls that a browser sends on behalf of another site. Modern
  browsers state the origin of a request themselves; for older ones the server compares the `Origin`
  header with the `Host` header, so the proxy has to pass the original `Host` on (Traefik and Caddy do;
  nginx needs `proxy_set_header Host $host`). Setting
  [`NEXMAP_PUBLIC_URL`](configuration.md#network) makes that comparison independent of the proxy.

### Response headers

Every response carries a `Content-Security-Policy` that only allows scripts, styles, images, fonts and
form targets from the server itself and forbids framing, plus `X-Content-Type-Options`,
`X-Frame-Options`, `Referrer-Policy` and `Permissions-Policy`. Images in Markdown that point to another
server are therefore not loaded. The policy still allows inline scripts, which the pages use, so it
limits what injected markup could do (load code from or send data to another server) rather than ruling
it out. If your proxy adds headers of its own,
make sure it does not replace these with weaker ones.

### Request limits

Request bodies are capped at 1 MiB, attachment uploads at `NEXMAP_MAX_UPLOAD_BYTES` plus 1 MiB for the
form around the file, and test run uploads at 32 MiB; larger requests get `413`. A request has to be
received within 5 minutes and answered within 10.

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

Each encrypted value, including a user's two-factor secret, is bound to the record and field it belongs
to. Someone who can write to the database but does not have the key cannot move one user's value into
another user's record: it no longer decrypts there. Values written by a version before this binding
existed are still read and are bound the next time the record is saved.

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
- **Usernames and email addresses** are unique regardless of case: `Admin` cannot be registered next to
  `admin`, and signing in accepts either spelling.
- **Passwords** are stored as salted PBKDF2-SHA256 hashes (`NEXMAP_PBKDF2_ITERATIONS`). The instance
  settings can require uppercase and lowercase letters, digits and special characters in new passwords,
  and a minimum length from 8 to 128 characters.
- **The password is asked for again** before two things a session alone must not be enough for: deleting
  your own account, and creating an access token with the `account` or `admin` scope.
- **The last administrator cannot be locked or deleted**, by anyone including themselves, and a product
  always keeps at least one owner. There is no setting to make a user an administrator yet; the first
  administrator is the only one unless you change `is_admin` in the database.
- **Email is not implemented yet**: there are no password reset or invitation emails. Administrators set
  new passwords for users from the user's detail page.
- **Sessions** expire after `NEXMAP_SESSION_TTL` (24 hours by default). The database stores only a hash
  of each session token, so a database backup contains nothing that works as a session cookie. Changing a password ends the
  user's other sessions, keeping only the one the change was made from; an administrator setting a new
  password ends all of that user's sessions. Personal access tokens stay valid either way; revoke them
  separately if they may have leaked. Locking an account blocks its sessions and tokens at once.
- **Failed logins are limited** per client address: after 5 wrong passwords for the same username, or
  20 for any usernames, within 15 minutes, that address has to wait until the 15 minutes are over. The
  web form says so and the API answers `429 TOO_MANY_ATTEMPTS` with a `Retry-After` header. A correct
  password clears the count for that username. There is no limit per account across addresses, so nobody
  can lock another user out by guessing their password. The counters live in memory and reset when the
  server restarts.
- **Wrong two-factor codes are limited** per user: after 10 wrong codes (from the authenticator or
  recovery codes) within an hour, no code is accepted for that user until the hour is over, at sign-in
  and when changing the two-factor settings alike. This takes the user's password or session, so it
  cannot be used to lock out a stranger; a user who is blocked this way should change their password.
  The server logs `second factor blocked after repeated wrong codes` with the user ID.

### Records of administrative actions

Changes to a product's team (adding and removing members, changing roles, transferring ownership) are
recorded in the product's audit trail. Administrator actions (approving, locking, unlocking and deleting
users, setting passwords, resetting 2FA, changing instance settings) are written to the server log as
`administrator action` with the administrator's and the affected user's ID and the client address. Keep
the log if you need to trace them later.

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

Creating accounts is limited to 20 attempts per client address and hour, for self-registration and
invitations together; after that the form and the API (`429 TOO_MANY_ATTEMPTS`) ask to wait.

Only product owners (and administrators) can invite to a product, and only administrators can invite to
the installation without a product.

## Access tokens

Personal access tokens are what the CLI, CI jobs and AI agents use. They are accepted only by the REST
API, never by the web UI's pages. Users can narrow a token to some products, to read or write access per
area, and to a lower role (see [Restricted tokens](../api/rest.md#restricted-tokens)); a token never grants
more than its user holds at the time of the request.

Two rights are never granted by default: managing the user's own account and tokens (`account`), and
administration (`admin`). Granting either from the web UI or a session needs the user's password. An administrator's token without the `admin` scope has no administrator rights,
so a leaked CI token cannot reach the instance settings. A token can create further tokens only with the
`account` scope, and only narrower than itself.

Under *Instance Settings*, **Maximum Token Lifetime** forces every new token to expire within the given
number of days (0, the default, allows tokens without expiry). The token list shows when each token was
last used; revoke those that are no longer needed under *API Tokens* (`/user/tokens`).

## Attachments

By default, every file type can be uploaded. To restrict this, set `NEXMAP_ALLOWED_UPLOAD_TYPES`; the
server then also checks that the content matches the extension. Attachments are always served as
downloads and never rendered by the browser, but nexmap does not scan them for malware.

## Development mode

`NEXMAP_DEV_MODE` fills a new database with demo users whose password is publicly known (it is in the
source code and the log). Never enable it on a server that others can reach.
