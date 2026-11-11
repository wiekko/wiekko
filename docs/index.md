# nexmap documentation

## Administrators

Deploying, configuring and running a nexmap server.

- [Installation](admin/installation.md): container image, Coolify

## CLI users

Developers, CI pipelines and AI agents working with the `nexmap` command line client.

- [Setup](cli/setup.md): installing, logging in, connecting a repository, configuration, commands
- [Scripting](cli/scripting.md): output formats, exit codes, raw API calls
- [CI integration](cli/ci-integration.md): recording automated test results with `nexmap utest`
- [AI agents](cli/ai-agents.md): agent guide, MCP server, what agents may change

## API

- [REST API](api/rest.md): authentication, response format, endpoints

## Specification

What nexmap *shall* do. Normative, with requirement identifiers; it describes targets, not necessarily
the current behaviour.

- [Requirements & constraints](spec/requirements.md)
- [Design system](spec/design-system.md): colours, typography, logo, asset bundling
- [UI structure](spec/ui-structure.md): page tree, navigation, UI design decisions

## Decisions

Why things are the way they are: design decisions with their reasoning.

- [Web UI](decisions/web-ui.md)
- [CLI](decisions/cli.md)
- [Test ingestion](decisions/test-ingestion.md)

## Contributing

Building, testing and how these docs are organised: [CONTRIBUTING.md](../CONTRIBUTING.md).
