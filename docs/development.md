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

Run the provider/model ID parsing tests:

    go test ./internal/provider -run 'ParseModelID|ValidateID'

Run the configuration loader tests while working on configuration:

    go test ./internal/config

Run only the secret-reference tests while working on secret loading:

    go test ./internal/config -run SecretRef

Run the gateway authentication tests while working on client authentication:

    go test ./internal/api

## Format

    gofmt -w .

Before committing, verify:

    gofmt -w .
    go mod tidy
    go vet ./...
    go test ./...
    go test -race ./...
    go build ./...

The GitHub Actions CI workflow runs these checks independently on pushes to
`main` and on pull requests targeting `main`. It reads the required Go version
from `go.mod`, caches downloaded modules, and fails rather than rewriting files
when formatting or committed module metadata is not current.

## Configuration

The default development configuration should be stored outside source
control or copied from an example:

    cp config.example.yaml config.yaml

Do not commit credentials.

Secrets should be supplied through:

- environment variables
- files

Secret references use exactly one source:

    server:
      authentication:
        api_key:
          env: GATEWAY_API_KEY

or:

    server:
      authentication:
        api_key:
          file: /run/secrets/gateway-api-key

File-backed secret values are trimmed of surrounding whitespace when loaded.
Environment-backed values are used unchanged.

## Running Locally

Example:

    export GATEWAY_API_KEY=development-only-key

    go run ./cmd/gateway serve --config ./config.yaml

Update this command if the CLI differs.

## Container Build

    podman build -t gateway:dev .

The production image targets `linux/amd64`, runs as the non-root
`nonroot:nonroot` user, stores persistent state under `/state`, and includes a
healthcheck that invokes `gateway healthcheck` against `/healthz`.

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

Update documentation when a task changes architecture, public behavior,
configuration, developer workflow, significant design decisions, or another
durable project convention. Narrow task-specific test commands do not need to
be added to this guide unless they are genuinely useful as a reusable
development procedure.

## Adding a Provider

General process:

1. Verify/document upstream behavior in `provider-notes.md`.
2. Determine whether the provider can use the transparent OpenAI transport.
3. Implement only provider-specific behavior that is necessary.
4. Add model discovery.
5. Add authentication handling.
6. Add provider contract tests.
7. Add streaming tests.
8. Update the relevant provider or architecture documentation if public
   behavior, configuration, architecture, or a significant design decision
   changes.

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

1. Read `TASKS.md` and select the first unblocked TODO task unless another task
   is explicitly assigned.
2. Move the selected milestone to `IN PROGRESS`. Keep its tracking and commit
   distinct from later milestones so its result remains independently
   reviewable.
3. Confirm all listed dependencies are in `DONE`.
4. Inspect related implementation and tests.
5. For provider work, update dated protocol findings in
   `docs/provider-notes.md` before implementation.

Before finishing:

1. run relevant tests
2. run `go test ./...`
3. run `go build ./...`
4. move the completed milestone from TODO to `DONE` in `TASKS.md`, recording
   its result and any remaining issues
5. update documentation when the task changes architecture, public behavior,
   configuration, developer workflow, significant design decisions, or another
   durable project convention
6. leave the repository in a buildable, resumable state with the next task and
   its dependencies clear

Finishing those steps does not require stopping work. If time and context
remain, select the next unblocked milestone and repeat the workflow in the same
development session. Multiple sequential milestones are allowed; each should
retain its own task transition, acceptance checks, documentation updates, and
focused commit.

## Troubleshooting

Add commonly encountered development problems here as they are discovered.
