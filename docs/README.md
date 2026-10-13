# Torii Gateway documentation

Torii connects your AI clients to local or hosted models through one OpenAI-compatible API.
An **upstream provider** is the service that runs a model; Torii routes requests to it.

## Start here

1. [Choose an installation method](installation/README.md): native binary, source build, Docker, or Podman.
2. [Getting Started](getting-started.md): create your administrator, connect a provider, and get your first response.
3. [Connect your AI client](guides/clients.md): set the server address, API key, and model.

## Use and manage Torii

| Task | Guide |
| --- | --- |
| Find settings and apply changes | [Console](guides/console.md) |
| Add local, hosted, or subscription models | [Providers](guides/providers.md) |
| Generate client keys and manage provider secrets | [Credentials](guides/credentials.md) |
| Let Torii choose a model | [Smart routing](guides/smart-routing.md) |
| Import and inspect model evaluations | [Benchmarks](guides/benchmarks.md) |
| Check health, logs, and remote access | [Operations](guides/operations.md) |
| Back up, update, or restore | [Backup and upgrade](guides/backup-and-upgrade.md) |
| Diagnose a problem | [Troubleshooting](guides/troubleshooting.md) |

## Look up supported behavior

[Features and limitations](reference/features.md) · [Provider support](reference/providers.md) ·
[Configuration](reference/configuration.md) · [Environment variables](reference/environment.md) ·
[CLI](reference/cli.md) · [HTTP API](reference/api.md)

[Planned features](reference/features.md#planned-features) describes future scope. Start
with the supported behavior above when configuring an installation today.

For implementation details, architecture, tests, and the project roadmap, see
[Development](development/README.md). The original implementation is the [alpha, v0.1.0](releases/v0.1.0.md);
the first public beta and first published release is [v0.2.0](releases/v0.2.0.md).
Release compatibility notes follow the [release template](releases/TEMPLATE.md).
