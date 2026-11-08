# nexmap

Nexmap Product Management Solution

## Binaries

| Binary    | Source          | Purpose                                                      |
|-----------|-----------------|--------------------------------------------------------------|
| `nexmapd` | `cmd/nexmapd`   | The server: web UI and REST API (`just build`, `just dev`)   |
| `nexmap`  | `cmd/nexmap`    | The command line client, talking to the server via REST only |

Quick start with the CLI (see `docs/40_cli_plan.md`):

```sh
nexmap login --server https://nexmap.example.com   # paste a token from /user/tokens
nexmap init                                         # writes .nexmap.json (no secrets)
nexmap whoami
nexmap api /products/{product}/requirements -o json
```

In CI, set `NEXMAP_SERVER`, `NEXMAP_PRODUCT` and `NEXMAP_TOKEN` instead of logging in.
