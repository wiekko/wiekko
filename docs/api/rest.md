# REST API

The REST API under `/api/v1` is the interface for the `nexmap` CLI and third-party systems. The web UI uses
its own endpoints and does not go through it.

## Authentication

Requests authenticate with a personal access token, created in the web UI under `/user/tokens`:

```
Authorization: Bearer <token>
```

Tokens are accepted only under `/api/`; the web UI's pages ignore them.

### Restricted tokens

A token can be narrowed when it is created, and never grants more than its user holds at the time of the
request:

- **Scopes.** Each scope is granted as `read` or `write`; `write` includes `read`. Without scopes, a token
  gets `write` on every product scope, but never `account` or `admin`.

  | Scope | Covers |
  |---|---|
  | `structure` | components, capabilities, milestones, sprints |
  | `backlog` | requirements, tasks, constraints |
  | `defects` | defects |
  | `tests` | test cases, campaigns and their results, test runs |
  | `discussion` | comments and attachments |
  | `settings` | product settings, members, categories, statuses, pyramid levels, runtime environments |
  | `account` | `/user/me/*`: your profile, password, two-factor setup and tokens |
  | `admin` | `/admin/*`; without it, an administrator's token has no administrator rights |

  A request needs `read` on every resource named in its path, and `write` on the first one if it changes
  data: `POST /products/{p}/capabilities/{id}/requirements` needs `structure:write` and `backlog:read`. Side
  effects belong to the request's own scope, so uploading a test run needs only `tests:write` although it
  files defects. Comments and attachments also need `read` on the item they belong to. The audit trail
  needs `read` on every product scope. `GET /user/me` is open to every token. A missing scope is answered
  with `403` and the code `TOKEN_SCOPE`.
- **Role cap.** The token acts with the lower of its user's role and the cap (`developer`, `tester` or
  `viewer`).
- **Products.** The token reaches only the listed products, and `GET /products` lists only those. Such a
  token cannot create products.

`GET /api/v1/user/me` shows the restriction of the token in use as `token`.

### Tokens

`POST /api/v1/user/me/tokens` (`account:write`) creates a token:

```json
{
  "name": "CI",
  "expires_at": "2027-01-31T23:59:59Z",
  "scopes": ["tests:write"],
  "role_cap": "tester",
  "products": ["7d1c…"]
}
```

All fields but `name` are optional; leave out `products` for a token that reaches every product of its
user. The response contains the token once, as `raw_token`. A request made with a token can only create
tokens that are narrower than its own. If the instance limits the token lifetime, `expires_at` is required
and must lie within the limit. `GET /api/v1/user/me/tokens` lists the tokens with their restrictions, and
`DELETE /api/v1/user/me/tokens/{id}` revokes one.

## Identifiers

Wherever a path names a requirement, task, constraint or defect by ID, its identifier works as well:
`GET /api/v1/products/{p}/defects/DF-17` is the same as using the defect's ID. Identifiers are not case
sensitive. Responses carry both: `id` and `identifier`. Defect identifiers are also accepted for the
`defect_id` of a campaign result and for the `owner_id` of comments and attachments on a defect. The
defect prefix is the product's `defect_prefix`, which `PATCH /api/v1/products/{p}` changes; it must
differ from every category prefix of the product.

## Responses

Successful responses wrap their payload in `data`; errors carry a machine-readable `code` and a
human-readable `message`:

```json
{"data": { … }}
{"error": {"code": "…", "message": "…"}}
```

## Endpoints

### Team, workflow and campaign scope

- `POST /api/v1/products/{p}/invitations` (owner; `settings:write`) invites someone who has no account
  yet, with `{"email": …, "role": …}`, and answers with the invitation's `url`.
- `POST /api/v1/products/{p}/statuses/split` (owner; `settings:write`) gives defects a status set of
  their own. It cannot be undone.
- `POST /api/v1/products/{p}/campaigns/{id}/test-cases` with `{"test_case_ids": […]}` and
  `DELETE /api/v1/products/{p}/campaigns/{id}/test-cases/{testCaseId}` (owner; `tests:write`) change the
  scope of a draft campaign; an active campaign's scope is locked.

### Specification export

`GET /api/v1/products/{productId}/specification` (viewer role; tokens need `structure:read` and
`backlog:read`) returns the product's specification as Markdown (`text/markdown`), not wrapped in
`data`: the structure, constraints and requirements, and with `?tasks=true` the tasks. The document is
ordered by stable keys and contains no timestamps, so an unchanged product exports identically. Errors
are JSON as usual.

### Test runs

`POST /api/v1/products/{productId}/test-runs` (tester role) takes

```json
{
  "source": "go-test",
  "campaign_id": "",
  "environment": "linux-amd64",
  "revision": "4161949",
  "branch": "main",
  "dry_run": false,
  "results": [
    {"test_identifier": "pkg.TestA", "status": "passed", "duration_ms": 12},
    {"test_identifier": "pkg.TestB", "status": "failed", "output": "b_test.go:9: got 1, want 2"}
  ]
}
```

and answers with `recorded`, `campaign`, `environment`, a `summary` (counts per status, per outcome, `defects_created`, `defects_reused`, `total`) and per-test `results` with `outcome` (`updated`, `unchanged`, `unknown_test_case`, `not_in_campaign`, `environment_required`, `not_recorded`), `defect` (`created`, `reused`) and the defect's `defect_id` and `defect_identifier`. Uploads are limited to 32 MiB and 50,000 results; statuses are `passed`, `failed` and `skipped`.
