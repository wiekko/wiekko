# Backup and restore

Wiekko does not make backups itself. A complete backup has three parts:

| Part | Where | Notes |
|---|---|---|
| Database | `WIEKKO_DB_DSN`: the SQLite file (container: `/data/wiekko.db`) or the PostgreSQL database | Everything except attachment contents |
| Attachments | `WIEKKO_UPLOAD_DIR` (container: `/data/uploads`) | One directory per product, one file per attachment |
| Encryption key | `WIEKKO_ENCRYPTION_KEY` | Needed to read personal data in the database |

**Keep the key apart from the database backups.** Without the key, the email addresses and display names
in a backup cannot be decrypted, and Wiekko has no way to recover them. A backup stored together with
its key, on the other hand, protects nothing. Store the key once, in a password manager or your
platform's secret store, and again whenever you [rotate it](security.md#rotating-the-key): a backup
needs every key that was in the list when it was taken.

Back up the database first, then the attachments. An attachment uploaded in between is then just an
unused file in the backup. The other order could leave a database that references files the backup
lacks.

## SQLite

The database runs in WAL mode, so the file on disk alone is not a consistent copy while the server
runs: recent changes may still sit in `wiekko.db-wal`. Do not copy the files with `cp` while the server
runs. Use SQLite's online backup instead, which produces a consistent single file without stopping the
server:

```sh
sqlite3 /path/to/wiekko.db ".backup '/backups/wiekko-$(date +%F).db'"
```

The container image has no `sqlite3`. Run it on the host, against the file in the volume mounted at
`/data` (for Docker volumes, typically `/var/lib/docker/volumes/<volume>/_data/wiekko.db`). The host
and the container must share the same file system; this does not work over a network share.

Alternatively, stop the server and copy `wiekko.db` together with any `wiekko.db-wal` and
`wiekko.db-shm` next to it.

Then the attachments:

```sh
tar -czf /backups/wiekko-uploads-$(date +%F).tar.gz -C /path/to/data uploads
```

## PostgreSQL

Use `pg_dump`, which takes a consistent snapshot while the server runs:

```sh
pg_dump --format=custom --file=/backups/wiekko-$(date +%F).dump "$WIEKKO_DB_DSN"
```

Back up the attachment directory as for SQLite.

## Restoring

1. Stop `wiekkod`.
2. Put the database back:
   - SQLite: replace `wiekko.db` with the backup file and delete any `wiekko.db-wal` and
     `wiekko.db-shm` left next to it, as they belong to the replaced database.
   - PostgreSQL: `pg_restore --clean --if-exists --no-owner --dbname="$WIEKKO_DB_DSN" wiekko.dump`
     drops the existing tables and restores those of the backup.
3. Replace the attachment directory with the one from the backup.
4. Make sure `WIEKKO_ENCRYPTION_KEY` contains the key (or keys) the backup was written with.
5. Start `wiekkod`.

A restored database is as it was at the time of the backup, sessions included: everyone signed in
then may still be signed in, and later password changes are undone. After restoring because of a
security incident, have users change their passwords, and revoke API tokens that may have leaked.

A backup from an older Wiekko version can be restored into a newer one: the missing migrations run at
startup. The reverse is refused: `wiekkod` does not start on a database migrated by a newer version.

## Upgrades

Migrations run automatically when a new version starts and cannot be undone. Back up before every
upgrade. An instance that deploys the `stable` branch automatically upgrades as soon as a release is
made, so schedule its backups accordingly, or deploy a release tag and upgrade by hand. To go back to
the previous version, restore that backup and start the previous version on it.

## Test the restore

Before relying on the backups, restore one into a separate instance (another directory or container,
with the same encryption key) and check that you can sign in, open a product and download an
attachment. Repeat this after changing how backups are taken.
