# CI Integration

`nexmap utest` records automated test results in the product's active testing campaign (FR-018, FR-409). It reads a test report from stdin or from files, maps each test to an automatic test case by its **test identifier**, and uploads the run in one request.

```sh
go test -json ./... | nexmap utest --env linux-amd64
go test -v ./...    | nexmap utest --identifier-style name   # format auto-detected
nexmap utest --format junit reports/*.xml
```

## What happens to a run

| Situation | Effect |
|---|---|
| Test passes / is skipped | Its campaign result is set to `passed` / `skipped`; a defect on the result is cleared |
| Test fails | Result set to `failed`. An open defect already on the result is reused; otherwise a defect *Automated test failed: \<identifier\>* is created with the run metadata and the test's output (last 64 KiB), linked to the test case |
| Status unchanged since the last run | Nothing is written, so repeated CI runs do not flood the result history |
| Subtests (`TestX/case`) without their own test case | Rolled up into `TestX`: any failure fails it, the failing subtests' output goes into the defect |
| Test case runs per environment | Needs `--env NAME` matching a runtime environment of the product; other environments' results are untouched |
| No automatic test case has the identifier, or the test case is not in the campaign | Not recorded, no defect; failures are still listed in the build log |
| **No active campaign** | The upload is accepted, nothing is written (no results, events or defects), and the CLI prints `no active campaign — results not recorded` |

The FR-504 loop works with automation: closing the defect moves the result to re-test, and the next green run marks it passed. A later failure files a new defect, since the old one is closed.

Each status change is recorded in the result history with the token's user, the report format (`go-test`, `junit`), and the revision and branch.

## Setting up

1. **Create a CI user** with the *tester* role in the product (tokens carry their user's full rights, so do not use a personal token), sign in as that user and create a personal access token under *Settings → Tokens*.
2. **Register the tests** as automatic test cases with a test identifier. `nexmap test sync` creates the missing ones from a report (developer role):
   ```sh
   go test -json ./... | nexmap test sync --dry-run   # review first
   go test -json ./... | nexmap test sync
   ```
   New test cases join campaigns created afterwards; an active campaign keeps the scope it was created with.
3. **Check the mapping** before wiring CI: `go test -json ./... | nexmap utest --dry-run` shows how each test maps to a test case and what would change.
4. **Install the CLI** in the job: a release binary, or `go install nexmap.net/cmd/nexmap@<version>` where your Go proxy can reach the module (the snippets below assume the latter).
5. **Configure the job** with `NEXMAP_SERVER`, `NEXMAP_PRODUCT` and the secret `NEXMAP_TOKEN` (or commit `.nexmap.json` from `nexmap init` and only set the token).

## Options

| Flag | Meaning |
|---|---|
| `--format auto\|go-json\|go-text\|junit` | Report format; `auto` sniffs the input (JSON events → `go-json`, XML → `junit`, else `go test -v` text) |
| `--identifier-style package\|name` | `package` (default): `pkg/path.TestName` for Go, `classname.name` for JUnit; `name`: the bare test name. Match whatever your test cases use |
| `--env NAME` | Runtime environment of this run (case-insensitive); unknown names are rejected |
| `--campaign NAME` | Record in this campaign instead of the active one (must be active) |
| `--revision`, `--branch` | Default from `GITHUB_SHA`/`GITHUB_REF_NAME`, `CI_COMMIT_SHA`/`CI_COMMIT_REF_NAME`, … or `git rev-parse` |
| `--dry-run` | Evaluate and print the mapping; write nothing |
| `--exit-code` | Exit 4 if any test (or a package build) failed |
| `--quiet` | Do not copy stdin to stdout |

When reading stdin, `utest` copies its input to stdout so the test log stays in the build output, then prints a summary. With `-o json` it prints only the JSON report.

**Exit status.** In `go test -json ./... | nexmap utest`, the shell reports the exit status of `nexmap`, not of `go test`. Either let `nexmap` fail the job with `--exit-code`, or use `set -o pipefail` (bash) so a failing `go test` still fails it. Without either, the job passes even when tests fail.

## GitHub Actions

```yaml
name: test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - name: Install nexmap
        run: go install nexmap.net/cmd/nexmap@latest
      - name: Test
        env:
          NEXMAP_SERVER: https://nexmap.example.com
          NEXMAP_PRODUCT: ${{ vars.NEXMAP_PRODUCT }}
          NEXMAP_TOKEN: ${{ secrets.NEXMAP_TOKEN }}
        run: go test -json ./... | nexmap utest --env linux-amd64 --exit-code
```

For a build matrix, pass the matrix value: `--env ${{ matrix.os }}`, with one runtime environment per OS in the product.

## GitLab CI

```yaml
test:
  image: golang:1.25
  variables:
    NEXMAP_SERVER: https://nexmap.example.com
    NEXMAP_PRODUCT: "<product id>"
    # NEXMAP_TOKEN: set as a masked CI/CD variable
  script:
    - go install nexmap.net/cmd/nexmap@latest
    - go test -json ./... | nexmap utest --env linux-amd64 --exit-code
```

## Forgejo / Gitea Actions

Forgejo Actions sets the same `GITHUB_*` variables, so revision and branch are picked up automatically.

```yaml
on: [push]
jobs:
  test:
    runs-on: docker
    container:
      image: golang:1.25
    steps:
      - uses: https://code.forgejo.org/actions/checkout@v4
      - run: go install nexmap.net/cmd/nexmap@latest
      - env:
          NEXMAP_SERVER: https://nexmap.example.com
          NEXMAP_PRODUCT: ${{ vars.NEXMAP_PRODUCT }}
          NEXMAP_TOKEN: ${{ secrets.NEXMAP_TOKEN }}
        run: go test -json ./... | nexmap utest --env linux-amd64 --exit-code
```

## Other ecosystems

Anything that writes JUnit XML works: pytest (`--junitxml=report.xml`), jest (`jest-junit`), Maven Surefire (`target/surefire-reports/*.xml`), gotestsum (`--junitfile`).

```sh
pytest --junitxml=report.xml; status=$?
nexmap utest --format junit report.xml
exit $status
```

With the default `package` style, pytest identifiers look like `tests.test_calc.test_add` and Surefire ones like `com.example.CalcTest.testAdd`; jest-junit's default (classname equal to name) yields the test title.

## API

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
