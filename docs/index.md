# Wiekko documentation

## Administrators

Deploying, configuring and running a Wiekko server.

- [Installation](admin/installation.md): container image, Coolify, first start
- [Configuration](admin/configuration.md): every environment variable and flag of `wiekkod`
- [Security](admin/security.md): TLS, the encryption key, the first administrator, accounts and tokens
- [Backup and restore](admin/backup.md): what to back up, SQLite and PostgreSQL, restoring, upgrades

## Users

Working with Wiekko in the browser.

- [Concepts](guide/concepts.md): products, structure, requirements, tasks, constraints, defects, tests, roles
- [Getting started](guide/getting-started.md): creating a product, settings, team, structure
- [Backlog](guide/backlog.md): requirements, tasks, constraints, defects, boards and sprints
- [Testing](guide/testing.md): test cases, campaigns, recording results

## CLI users

Developers, CI pipelines and AI agents working with the `wiekko` command line client.

- [Setup](cli/setup.md): installing, logging in, connecting a repository, configuration, commands
- [Scripting](cli/scripting.md): output formats, exit codes, raw API calls
- [CI integration](cli/ci-integration.md): recording automated test results with `wiekko utest`
- [AI agents](cli/ai-agents.md): agent guide, MCP server, what agents may change

## API

- [REST API](api/rest.md): authentication, response format, endpoints

## Contributing

The source repository holds the material for contributors that this documentation leaves out:

- the specification: what Wiekko *shall* do, with requirement identifiers, kept in Wiekko itself
  (`CONTRIBUTING.md` explains access). It describes targets, not necessarily the current behaviour;
- `docs/decisions/`, the design decisions and their reasoning;
- `CONTRIBUTING.md`: building, testing, and how this documentation is organised and published.
