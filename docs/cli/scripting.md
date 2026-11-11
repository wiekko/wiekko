# Scripting with the CLI

The output formats and exit codes below are a stable contract for scripts and AI agents.

## Output formats

| Format | Use |
|---|---|
| `-o table` | Plain tables and records for people. The default |
| `-o md` | Compact markdown with the same headings every time; best for pasting into an AI agent's context |
| `-o json` | Machine-readable, with stable field names; use it for anything that parses the output |

The default is `table` whether or not stdout is a terminal, so a command's output never changes with the
environment it runs in: scripts must ask for `-o json` explicitly. To change your personal default, set
`NEXMAP_OUTPUT` or `output` in the user configuration file (see [Setup](setup.md#configuration)).

## Errors and exit codes

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Usage or configuration error (bad arguments, no server configured, ambiguous reference) |
| `2` | API error: server unreachable, authentication failed, or not permitted for your role |
| `3` | Not found: the item does not exist or is not visible to you |
| `4` | Tests failed (only `utest --exit-code`) |

Errors go to stderr as a single line, `nexmap: <message>`, followed by a hint for authentication and
role errors. With `-o json`, stderr carries `{"error": {"code": "…", "message": "…"}}` instead.

A role refusal (exit 2) is final: retrying will not help; the user needs a different role in the product.

## Long texts

Commands that take a description or comment body accept it from a file or stdin (`--body-file -`), which
avoids shell quoting problems with markdown:

```sh
nexmap comment add FR-12 --body-file - <<'EOF'
Decided to keep the old endpoint until **v2**.
EOF
```

## Raw API calls

For anything without a dedicated command, `nexmap api` sends an authenticated request to the
[REST API](../api/rest.md) and prints the response:

```sh
nexmap api /products/{product}/requirements -o json
nexmap api POST /products/{product}/requirements --data @new.json
generate-body | nexmap api POST /products/{product}/requirements --data @-
```

The method defaults to `GET`; `{product}` in the path is replaced with the configured product; `--data`
takes JSON text, `@file` or `@-` for stdin.

## Retries

Read requests (`GET`, `HEAD`, `OPTIONS`) are retried automatically after network errors and `429`,
`502`, `503` and `504` responses. Requests that change data are sent once, so a failed write is never
applied twice behind your back.
