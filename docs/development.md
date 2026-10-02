# Development Guide

## Requirements

- Go
- Git
- Podman or Docker for container testing

SQLite is embedded (pure Go); no database server or external infrastructure is
required. Running the gateway requires a separately backed-up database master key.

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

Run browser-console tests and verify the generated script (Node.js 22.13+):

    node --test internal/web/app_test.mjs
    node scripts/build-console.mjs --check

To validate non-root credentials, benchmark persistence and administrator setup in Podman:

    podman build --format docker -t localhost/torii-credential-validation:local .
    bash scripts/test-credential-container.sh

This uses disposable containers, volumes and mounted secrets, cleans them up,
and never touches an existing gateway deployment. It exercises setup, authenticated
configuration saves, key regeneration, restart persistence and wrong-key rejection.
The script also requires the host SQLite CLI (`sqlite3`). It imports legacy usage,
Codex identity and benchmark fixtures with benchmark sources disabled (no live
calls), then recreates without `/state` and verifies SQLite remains authoritative.

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

Provider credentials are entered through the authenticated console and encrypted
in SQLite; YAML keeps only `{credential: ID}` references. Legacy inline/env/file
credentials are imported once. Administrator credentials and the inference key
are database-managed, not configured in YAML or environment variables.
See [credential storage](credential-storage.md) for key management and migration.

## Running Locally

Example:

Generate a master key once and back it up separately:

    umask 077
    mkdir -p secrets
    openssl rand -hex 32 > secrets/torii_db_key

Set `state.database_path: ./state/torii.db` in your local YAML, then run:

    export TORII_DB_KEY_FILE="$PWD/secrets/torii_db_key"
    go run ./cmd/gateway serve --config ./config.yaml

Choose an administrator username/password when prompted and save the newly
generated inference API key immediately; it is displayed only once. Reuse the
same master key on every restart. Never regenerate it for an existing database.

## Container Build

    podman build -t gateway:dev .

The production image targets `linux/amd64`, runs as the non-root
`nonroot:nonroot` user, stores durable state under `/data`, and includes a
healthcheck that invokes `gateway healthcheck` against `/healthz`.

Run:

    podman run --rm \
      -p 8080:8080 \
      -v ./config.yaml:/config/config.yaml:ro \
      --secret torii_db_key,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400 \
      -v torii_gateway_database:/data \
      gateway:dev

Create the native Podman secret first with `podman secret create torii_db_key
secrets/torii_db_key`. The example's read-only YAML disables console saves; use a
writable directory mount at `/config` when testing saves. Existing installations
must temporarily mount their original state directory during migration; see
[runtime state](runtime-state.md).

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
