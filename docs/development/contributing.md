# Contributing

For running Torii, use [Installation](../installation/README.md). These are contributor procedures.

## Requirements

- Go 1.25.1 or newer
- Git
- Podman or Docker for container testing

SQLite is embedded (pure Go); no database server or external infrastructure is
required. Running the gateway requires a separately backed-up database master key.

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

The dependency-free build combines `internal/intelligence/presets.json`,
`internal/web/static/benchmarks.ts` and `app.ts` into the committed `app.js`.
Run `node scripts/build-console.mjs` after editing either TypeScript source or
the shared presets. Console tests use a mocked DOM and gateway;
benchmark tests exercise fragmented streaming updates without live services.

To validate non-root credentials, benchmark persistence and administrator setup in Podman:

    podman build --format docker -t localhost/torii-credential-validation:local .
    bash scripts/test-credential-container.sh

This uses disposable containers, volumes and mounted secrets, cleans them up,
and never touches an existing gateway deployment. It exercises setup, authenticated
configuration saves, key regeneration, restart persistence and wrong-key rejection.
It also verifies shipped benchmark controls, authenticated table access and
streaming refresh. The script requires the host SQLite CLI (`sqlite3`) and uses
only synthetic keys, usage and disabled benchmark fixtures (no live source
calls). SQLite remains authoritative across container recreation and restore.

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
when formatting or committed module metadata is not current. It also checks the
shipped console and release-metadata validator, builds the non-root container,
and exercises fresh setup, persistence, rotations and stopped-backup recovery
using disposable Podman volumes. Node 24 is development/CI-only. No frontend
packages or new runtime services are required.

## Manual releases

1. Choose an unused `vMAJOR.MINOR.PATCH` tag, optionally with a SemVer prerelease
   suffix. Image tags do not support `+build` metadata.
2. Copy `docs/releases/TEMPLATE.md` to `docs/releases/VERSION.md`, replacing
   VERSION with the exact tag, and complete the version heading and all four
   compatibility sections. Run `node scripts/check-release.mjs VERSION`.
3. Commit the notes and implementation; the maintainer creates and pushes the
   release tag. No local implementation command publishes a release.
4. Release automation reuses the complete CI gate. Gateway/helper binaries are
   cross-built for Linux amd64/arm64, macOS amd64/arm64 and Windows amd64. An
   amd64/arm64 OCI image archive is built separately without publication.
5. Only after all jobs succeed does publication upload the image to GHCR and
   create a GitHub Release containing binaries, `SHA256SUMS`, compatibility notes,
   and `CONTAINER_IMAGE.txt` with the multi-platform digest. Existing image tags
   are not overwritten; a re-release must use a new version.
6. Repository settings must allow Actions to create releases and packages.
   Grant the publishing job access if a GHCR package already exists. Set the
   package visibility to public for anonymous pulls; initial visibility depends
   on repository/package settings. No extra registry password is committed.

Images and binaries embed version, commit and build time. Inspect with
`gateway version` or `torii-auth version`. Source builds report `dev` unless
metadata is supplied through linker flags. See [backup and upgrades](../guides/backup-and-upgrade.md)
for backup, reset and rollback requirements.

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

## Workflow

1. Follow repository instructions and inspect task-relevant code/tests first.
   Read applicable sections of [Architecture](architecture.md), [Design](design.md),
   and [Status](status.md) when needed; do not routinely load historical records.
   Explicitly assigned work takes precedence over backlog order.
2. Track actual active/completed work only in status. Preserve milestone IDs,
   relevant evidence, and unresolved limitations; do not create a second task ledger.
3. Run checks appropriate to changed behavior. Full CI gates remain required for
   releases; unrelated documentation edits do not require new application tests.
4. Update affected guides/reference and technical explanations when public behavior,
   schema, security, or durable conventions change. Repair moved links in comments,
   scripts, and workflows as well as Markdown.
5. Leave work buildable and reviewable, with meaningful unresolved checks recorded.

Before provider work, read [Provider notes](provider-notes.md). Before editing the
console, read `scripts/build-console.mjs` and preserve generated `app.js` freshness.
See [Documentation conventions](documentation.md) for document types and validation.
