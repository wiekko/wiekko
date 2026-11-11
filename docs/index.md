# nexmap documentation

## Administrators

Deploying, configuring and running a nexmap server.

- [Installation](admin/installation.md): container image, Coolify, first start
- [Configuration](admin/configuration.md): every environment variable and flag of `nexmapd`
- [Security](admin/security.md): TLS, the encryption key, the first administrator, accounts and tokens

## Users

Working with nexmap in the browser.

- [Concepts](guide/concepts.md): products, structure, requirements, tasks, constraints, defects, tests, roles
- [Getting started](guide/getting-started.md): creating a product, settings, team, structure
- [Backlog](guide/backlog.md): requirements, tasks, constraints, defects, boards and sprints
- [Testing](guide/testing.md): test cases, campaigns, recording results

## CLI users

Developers, CI pipelines and AI agents working with the `nexmap` command line client.

- [Setup](cli/setup.md): installing, logging in, connecting a repository, configuration, commands
- [Scripting](cli/scripting.md): output formats, exit codes, raw API calls
- [CI integration](cli/ci-integration.md): recording automated test results with `nexmap utest`
- [AI agents](cli/ai-agents.md): agent guide, MCP server, what agents may change

## API

- [REST API](api/rest.md): authentication, response format, endpoints

## Contributing

nexmap is developed in its [repository](https://forge.my-universe.com/daemotron/nexmap), which also holds
the material for contributors that this documentation leaves out:

- the [specification](https://forge.my-universe.com/daemotron/nexmap/src/branch/main/docs/spec): what
  nexmap *shall* do, with requirement identifiers. It describes targets, not necessarily the current
  behaviour;
- the [design decisions](https://forge.my-universe.com/daemotron/nexmap/src/branch/main/docs/decisions)
  and their reasoning;
- [CONTRIBUTING.md](https://forge.my-universe.com/daemotron/nexmap/src/branch/main/CONTRIBUTING.md):
  building, testing, and how this documentation is organised and published.
