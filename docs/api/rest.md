# REST API

The REST API under `/api/v1` is the interface for the `wiekko` CLI and third-party systems. The web UI uses
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
  | `documents` | documents and their revisions |
  | `defects` | defects |
  | `tests` | test cases, campaigns and their results, test runs |
  | `discussion` | comments and attachments |
  | `settings` | product settings, members, categories, statuses, pyramid levels, runtime environments |
  | `account` | `/user/me/*`: your profile, password, two-factor setup and tokens |
  | `admin` | `/admin/*`; without it, an administrator's token has no administrator rights |

  A request needs `read` on every resource named in its path, and `write` on the first one if it changes
  data: `POST /products/{p}/capabilities/{id}/requirements` needs `structure:write` and `backlog:read`. Side
  effects belong to the request's own scope, so uploading a test run needs only `tests:write` although it
  files defects. Linking a document to a requirement or constraint needs `documents:write` and `backlog:read`, to a
  test case `documents:write` and `tests:read`. Comments and attachments also need `read` on the item they belong to. The audit trail
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
user. A request made with a web session has to add `"password"`, the user's current password, to grant
the `account` or `admin` scope; so does `POST /api/v1/user/me/delete`. The response contains the token once, as `raw_token`. A request made with a token can only create
tokens that are narrower than its own. If the instance limits the token lifetime, `expires_at` is required
and must lie within the limit. `GET /api/v1/user/me/tokens` lists the tokens with their restrictions, and
`DELETE /api/v1/user/me/tokens/{id}` revokes one.

## Identifiers

Wherever a path names a requirement, task, constraint, document or defect by ID, its identifier works as well:
`GET /api/v1/products/{p}/defects/DF-17` is the same as using the defect's ID. Identifiers are not case
sensitive. Responses carry both: `id` and `identifier`. Defect identifiers are also accepted for the
`defect_id` of a campaign result and for the `owner_id` of comments and attachments on a defect. The
defect prefix is the product's `defect_prefix`, the document prefix (`DOC-3`) its `document_prefix`;
`PATCH /api/v1/products/{p}` changes either. The two must differ from each other and from every category
prefix of the product.

## Responses

Successful responses wrap their payload in `data`; errors carry a machine-readable `code` and a
human-readable `message`:

```json
{"data": { … }}
{"error": {"code": "…", "message": "…"}}
```

## Concurrent edits

A `PATCH` of a requirement, task, defect, constraint, component, capability, milestone, sprint,
document, test case, campaign or product accepts `if_updated_at`: the `updated_at` of the version the
change is based on. If the item has been changed since, nothing is written and the answer is
`409 CONFLICT`; read it again and reapply the change. Without `if_updated_at` the change applies to
whatever is current.

## Links and assignments

Creating a link or an assignment that already exists succeeds and changes nothing, so a request can
safely be repeated. This holds for every `POST` that links two items or assigns a user, e.g.
`…/defects/{id}/links`, `…/test-cases/{id}/requirements`, `…/test-cases/{id}/environments` and
`…/assignees`. A test case's link to a requirement carries a `link_type`, `requirement` (the default)
or `definition_of_done`: linking the pair again with a `link_type` changes it, without one keeps it.

## Endpoints

### Team, workflow and campaign scope

- `POST /api/v1/products/{p}/invitations` (owner; `settings:write`) invites someone who has no account
  yet, with `{"email": …, "role": …}`, and answers with the invitation's `url`. With
  `"delivery": "email"` (only if the server [sends mail](../admin/configuration.md#mail)), Wiekko emails
  the link to that address instead, and the answer carries neither `url` nor `raw_token`. Invitation
  emails are limited per inviter and per address (`429 TOO_MANY_ATTEMPTS`); if the email cannot be sent,
  the answer is `502 MAIL_NOT_SENT` and no invitation is left behind. `"delivery": "link"` is the
  default.
- `POST /api/v1/products/{p}/statuses/split` (owner; `settings:write`) gives defects a status set of
  their own. It cannot be undone.
- `POST /api/v1/products/{p}/campaigns/{id}/test-cases` with `{"test_case_ids": […]}` and
  `DELETE /api/v1/products/{p}/campaigns/{id}/test-cases/{testCaseId}` (owner; `tests:write`) change the
  scope of a draft campaign; an active campaign's scope is locked.
- A campaign keeps its own copy of each test case in its scope. A draft campaign follows edits to the
  test case; from activation on, the copy is frozen. Campaign results carry the copy's `test_title`
  and the `environment_name` they ran in.
  - **Deleting a test case or an environment:**
    - Draft campaigns drop it.
    - Active campaigns keep the results that have a verdict and drop those still `pending` or `retest`.
    - Closed campaigns keep everything.
  - Kept results then have `test_case_id` `null`, or `environment_id` empty, but keep their title,
    environment name and history.
- `PATCH /api/v1/products/{p}/requirements/{id}` (owner) with `{"parent_id": …}` moves a requirement
  or task below another item, given by ID; `""` moves it to the top level. The checks are those of the
  web UI: the parent belongs to the same product, the move creates no cycle, and a requirement's parent
  is a requirement (tasks may hang below either). A refused move answers `400 REQUEST_FAILED`, an
  unknown parent `404 NOT_FOUND`.
- `DELETE /api/v1/products/{p}/requirements/{id}` deletes a requirement or task. With
  `?children=keep`, the default, the items below it move up one level, to its parent or to the top
  level; with `?children=delete`, they are deleted too.
- `PATCH …/requirements/{id}` and `PATCH …/defects/{id}` (owner) with `{"archived": true}` archive a
  closed item, `{"archived": false}` brings it back. A requirement or task is archived together with every
  item below it, which must all be closed. Moving an archived item to an open status unarchives it, its
  subtree and its archived ancestors. Items carry `archived` and `archived_at`.
- `GET …/requirements` and `GET …/defects` leave archived items out; `?archived=include` lists them
  too, `?archived=only` lists only them. Every other read, such as the linked items of a capability, a
  milestone or a test case, includes them.

### Test cases and constraints

A test case can verify a constraint, or part of it, e.g. that session cookies are HttpOnly. Passing it
is evidence for the constraint's compliance; compliance itself stays the owner's decision.

- `GET`, `POST` (with `{"constraint_id": …}`, an ID or an identifier such as `SC-30`) and
  `DELETE …/{conId}` on `/api/v1/products/{p}/test-cases/{id}/constraints` read and change the links
  (developer; tokens need `tests:write` and `backlog:read`). Linking a linked pair again changes
  nothing. The list holds the linked constraints.
- `GET /api/v1/products/{p}/constraints/{id}/test-cases` (`backlog:read` and `tests:read`) lists the
  linked test cases, then the test cases of the constraint's capabilities that are not linked to it.
  Each carries `latest_result` (its most recently executed result in any campaign, with `status`,
  `executed_at`, `campaign_id` and `environment_name`, or `null`) and `via`: empty for a linked test
  case, the capabilities (`id`, `name`) it comes through otherwise.

### Documents

Documents are the protocols, schemas and other texts that define the product. Wiekko holds them; a
repository gets copies through the [export](#specification-export).

- `GET /api/v1/products/{p}/documents` lists them, without bodies unless `?body=true`; filter with
  `state`, `format`, `component_id`, `capability_id`, `milestone_id` and `q` (title or identifier).
- `POST /api/v1/products/{p}/documents` (owner) creates one from `title`, `body`, `format` (`markdown`,
  the default, or `text`, `json`, `yaml`, `xml`, `proto`, `sql`, `graphql`), `state` (`draft`, the
  default, `current` or `superseded`), and optionally `component_id` or `capability_id` (not both),
  `milestone_id` and, for a superseded document, `superseded_by_id`. The body is stored byte for byte, up
  to 512 KiB of valid UTF-8 without NUL characters.
- `GET`, `PATCH` and `DELETE /api/v1/products/{p}/documents/{id}`. A `PATCH` that changes the title, body
  or format creates a new revision; `revision` is the number of the latest one.
- `GET …/documents/{id}/raw[?revision=n]` returns the body itself, always as a `text/plain` attachment
  named like `DOC-3.json`, so a browser never renders it.
- `GET …/documents/{id}/revisions` lists the revisions, `GET …/revisions/{n}` returns one with its body,
  `GET …/diff?from=&to=` compares two (by default the latest with the one before) as `unified` text and
  as `lines`, and `POST …/restore` with `{"revision": n}` (owner) makes an earlier content current
  again as a new revision.
- `GET`, `POST` (with `{"target_id": …}`, an ID or identifier) and `DELETE …/{targetId}` on
  `…/documents/{id}/requirements`, `…/constraints` and `…/test-cases` read and change the links (owner).
  `GET …/requirements/{id}/documents`, `…/constraints/{id}/documents` and `…/test-cases/{id}/documents`
  read them from the other side.

A single requirement, constraint or test case (`GET …/requirements/{id}` and so on) names its documents
in `documents`; for a token without `documents:read`, only their identifiers.

### Specification export

`GET /api/v1/products/{productId}/specification` (viewer role; tokens need `structure:read` and
`backlog:read`) returns the product's specification as Markdown (`text/markdown`), not wrapped in
`data`: the structure, constraints and requirements, and with `?tasks=true` the tasks. For a token with
`documents:read` it also lists the current documents, with `?drafts=true` the drafts too, each with the
file name its body is exported under; the bodies come from the documents endpoints. The document is
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

### Audit trail (administrators)

The audit trail of every product, including deleted ones, is open to administrators (`admin:read`,
purging `admin:write`). The filters `product_id`, `entity_type`, `user_id`, `action`, `from` and `to`
(dates as `YYYY-MM-DD` or RFC 3339; `to` includes the whole day) apply to the list and the export.

- `GET /api/v1/admin/audit[?limit=&offset=]` lists entries, newest first, as
  `{"entries": […], "total": n, "limit": …, "offset": …}`; `limit` defaults to 50, at most 500.
- `GET /api/v1/admin/audit/export?format=jsonl|csv` downloads the matching entries, oldest first, as
  an archive file.
- `GET /api/v1/admin/audit/deleted-products` lists deleted products whose history is still kept, with
  their name and number of entries.
- `DELETE /api/v1/admin/audit/products/{productId}` purges the history of a deleted product and
  answers with `{"purged": n}`. A product that still exists keeps its history (`409 PRODUCT_EXISTS`).
  The purge itself is recorded as one entry with the action `purge_audit`.
