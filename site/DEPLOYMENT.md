# Deploying wiekko.net on Coolify

Internal runbook: how the download site (landing page, CLI install scripts, CLI release archives) is
deployed, and how to set it up again. For how the site is built, see `CONTRIBUTING.md`, "Download site".

## Overview

The site is its own Coolify application, next to the Wiekko server and the documentation site, built from
the same Forgejo repository (`ssh://git@forge.my-universe.com:22222/daemotron/nexmap.git`, private) on
branch **`stable`**.

```
just release vX.Y.Z push ──▶ stable moves to the release commit ──webhook──▶ Coolify
                                                                              │
     docker build -f site/Dockerfile ◀────────────────────────────────────────┘
       ├─ dist: builds wiekko for linux/darwin/windows × amd64/arm64,
       │        packages the archives, SHA256SUMS, release.json
       └─ runtime: nginx (unprivileged), the site, this release in /opt/wiekko/releases/

     container start ─▶ 40-publish-releases.sh
       ├─ copies the release to /data/releases/<version>/ (volume), unless it is there already
       └─ points /data/releases/latest.json at it
```

`stable` only ever moves to final releases (see `CONTRIBUTING.md`, "Releasing"), so every deployment is
a release, and the newest one. The build refuses a `+dev` tree, so a deployment that accidentally tracks
`main` fails instead of publishing a development build.

Each image contains only its own release; **the volume is what keeps older releases downloadable** (CI
jobs pin versions, `wiekko update vX.Y.Z` installs one). Releases on the volume are never overwritten.

## What you need

- A Coolify v4 server.
- DNS records for `wiekko.net` and `www.wiekko.net` pointing at it.
- Access to the Forgejo repository as an administrator of it (deploy key, webhook).

No token: unlike the documentation site, the build needs no git tags.

## Setting up

### 1. Coolify application

1. Create an application from the private repository, in the same way as the Wiekko server's (SSH with a
   deploy key, registered read-only in Forgejo under **Settings → Deploy keys**).
2. Branch **`stable`**.
3. Build pack **Dockerfile**, base directory `/`, Dockerfile location `/site/Dockerfile`. The base directory
   has to be the repository root: the build compiles the CLI.
4. Exposed port `8080`, domains `https://wiekko.net,https://www.wiekko.net`. nginx redirects `www.` to the
   apex itself, so both just need to reach the container.
5. **Persistent storage:** a volume mounted at `/data` (Coolify's default). The releases live in
   `/data/releases/`. The image prepares `/data` for its user (uid 101), so a fresh named volume is
   writable; a bind mount must be writable by uid 101.
6. Health check: path `/releases/latest.json`, port `8080`.

### 2. Automatic deployment

In Forgejo, **Settings → Webhooks → Add webhook**, type Gitea (or Forgejo), with the URL and secret from
the application's webhook settings in Coolify, triggered by push events, branch filter **`stable`**.

### 3. First deployment and check

Deploy from the current `stable` (the newest release). In the logs of the running container:

```
publish-releases: published v1.0.0
publish-releases: latest is v1.0.0
```

Then:

```sh
curl -fsS https://wiekko.net/releases/latest.json     # {"version":"v1.0.0","api_level":1}
curl -fsSI https://www.wiekko.net/ | grep -i location # https://wiekko.net/
curl -fsSL https://wiekko.net/install.sh | sh          # installs the CLI
```

Releases from before the site existed are not on the volume. To publish one, see "Publishing a release
by hand".

## Day to day

- **`just release vX.Y.Z push`** moves `stable`; the webhook deploys the site, which publishes the
  release. Nothing else to do.
- A redeploy of the same commit changes nothing: the release is on the volume already, and stays as it
  is.
- **Changes to the landing page or the install scripts** reach wiekko.net with the next release. To
  publish them sooner, nothing short of a release helps: the site deploys `stable` only.

## Publishing a release by hand

For a release made from a `release/*` branch (which does not move `stable`), a release from before the
site existed, or one lost with the volume. Every release can be rebuilt from its tag:

```sh
git switch --detach vX.Y.Z
just dist                       # build/dist/vX.Y.Z/
git switch main
```

Copy `build/dist/vX.Y.Z/` to the Coolify server, then into the running container's volume (the
container's name is in Coolify's application settings). nginx only needs to read the files:

```sh
docker cp build/dist/vX.Y.Z <container>:/data/releases/vX.Y.Z
docker exec <container> ls /data/releases/vX.Y.Z
```

`latest.json` stays as it is, which is right for older releases. The rebuilt archives match the originals
only when built with the same toolchain; their `SHA256SUMS` is written with them, so they are consistent
either way.

## Moving to another server

1. Copy the volume's `releases/` directory to the new server (e.g. `docker cp` out of the old container
   and into the new one), or republish the releases you need by hand as above.
2. Set up the application as in "Setting up", with a new deploy key and webhook.
3. Point the DNS records at the new server, deploy and check as in step 3.

## Troubleshooting

| Symptom | Meaning |
|---|---|
| Build fails with `dist: 'vX.Y.Z+dev' is not a release version` | The application builds a branch other than `stable`. |
| Container exits with `publish-releases: /data/releases is not writable` | The volume is not writable by uid 101: use a named volume, or fix the bind mount's owner. |
| `latest.json` is 404 | Nothing was published yet; check the container log for `publish-releases`. |
| `www.wiekko.net` shows a certificate error | Coolify did not get a certificate for it: both domains must be on the application. |
