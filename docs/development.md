# Development Guide

## Requirements

- Go
- Git
- Podman or Docker for container testing

No database or external infrastructure is required for v1 development.

## Clone

    git clone <repository>
    cd <repository>

## Build

Build every command and package in the repository:

    go build ./...

To build the gateway binary:

    go build -o ./bin/gateway ./cmd/gateway

## Test

Run all package tests (this also compiles packages that do not yet have test
files):

    go test ./...

Run with race detection:

    go test -race ./...

Run a specific package:

    go test ./internal/provider/...

Run the configuration loader tests while working on configuration:

    go test ./internal/config

## Format

    gofmt -w .

Before committing, verify:

    gofmt -w .
    go test ./...
    go build ./...

## Configuration

The default development configuration should be stored outside source
control or copied from an example:

    cp config.example.yaml config.yaml

Do not commit credentials.

Secrets should be supplied through:

- environment variables
- files

## Running Locally

Example:

    export GATEWAY_API_KEY=development-only-key

    go run ./cmd/gateway serve --config ./config.yaml

Update this command if the CLI differs.

## Container Build

    podman build -t gateway:dev .

Run:

    podman run --rm \
      -p 8080:8080 \
      -v ./config.yaml:/config/config.yaml:ro \
      -v ./state:/state \
      gateway:dev

## Health Check

    curl http://localhost:8080/healthz

## Basic API Test

    curl http://localhost:8080/v1/models \
      -H "Authorization: Bearer $GATEWAY_API_KEY"

## Repository Documentation

Before architectural changes, read:

- `AGENTS.md`
- `TASKS.md`
- `docs/implementation-plan.md`
- `docs/architecture.md`
- `docs/decisions.md`

Before provider work, also read:

- `docs/provider-notes.md`

## Adding a Provider

General process:

1. Verify/document upstream behavior in `provider-notes.md`.
2. Determine whether the provider can use the transparent OpenAI transport.
3. Implement only provider-specific behavior that is necessary.
4. Add model discovery.
5. Add authentication handling.
6. Add provider contract tests.
7. Add streaming tests.
8. Update architecture documentation if behavior changes.

Do not create a custom protocol adapter if the generic OpenAI-compatible
transport is sufficient.

## Adding a Classifier

1. Implement the `Classifier` interface.
2. Return the fixed internal `Classification` structure.
3. Do not return physical model names.
4. Do not recursively invoke the `smart` model.
5. Add malformed-output and failure tests.
6. Document configuration.

## Dependencies

Prefer the standard library.

Before introducing a dependency, determine:

- what problem it solves
- why the standard library is insufficient
- whether an existing project dependency already solves it
- runtime/binary-size implications

## Task Workflow

Before beginning:

1. Read `TASKS.md`.
2. Confirm the assigned task.
3. Inspect related implementation and tests.

Before finishing:

1. run relevant tests
2. run `go test ./...`
3. run `go build ./...`
4. update `TASKS.md`
5. update relevant documentation
6. leave the repository in a resumable state

## Troubleshooting

Add commonly encountered development problems here as they are discovered.
