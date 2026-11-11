# REST API

The REST API under `/api/v1` is the interface for the `nexmap` CLI and third-party systems. The web UI uses
its own endpoints and does not go through it.

## Authentication

Requests authenticate with a personal access token, created in the web UI under `/user/tokens`:

```
Authorization: Bearer <token>
```

A token carries the full rights of the user who created it. For automation, create a dedicated user with
the role the job needs rather than using a personal token.

## Responses

Successful responses wrap their payload in `data`; errors carry a machine-readable `code` and a
human-readable `message`:

```json
{"data": { … }}
{"error": {"code": "…", "message": "…"}}
```

## Endpoints

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

and answers with `recorded`, `campaign`, `environment`, a `summary` (counts per status, per outcome, `defects_created`, `defects_reused`, `total`) and per-test `results` with `outcome` (`updated`, `unchanged`, `unknown_test_case`, `not_in_campaign`, `environment_required`, `not_recorded`) and `defect` (`created`, `reused`). Uploads are limited to 32 MiB and 50,000 results; statuses are `passed`, `failed` and `skipped`.
