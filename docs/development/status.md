# Project status

Updated 2026-10-13. This is the sole project progress and backlog tracker.

## Completed private first-release correction

[PR #41](https://github.com/Apartment38/torii-gateway/pull/41) removed the standalone
authentication helper, remote pairing/import/download API and helper build
artifacts. Device-code console login and native gateway OAuth remain. The first
release notes now cover fresh native/container installation; subsequent versions
still require compatibility sections. Git history is retained.

The old private candidate's release, tag, five OCI image versions and workflow
run/artifacts were withdrawn. GitHub rejects deleting a package's last tagged
version, so a verified helper-free staging image temporarily preserved the package
and its access settings. Staging was deleted after replacement verification.
Obsolete task-owned local binaries, archives and the old validation image were
removed; existing deployments were not changed.

The recreated `v0.2.0` tag points to `c79441877fef393accb9530fa4e57885897db47c`.
The [replacement workflow](https://github.com/Apartment38/torii-gateway/actions/runs/38433314922)
passed on its first attempt. The [release](https://github.com/Apartment38/torii-gateway/releases/tag/v0.2.0)
contains exactly five gateway binaries, `SHA256SUMS` and `CONTAINER_IMAGE.txt`.
Repository and GHCR package remain private. The image index is
`sha256:bf17082d3e855cafaa6b3608d33849fc873ca86d7b236eca7c2ffad5e5e5b7c0`.

Validation: baseline and final Go/console checks, race tests, vet/build, unchanged
module metadata, all five cross-builds, login regressions, native fresh-install
startup/health, disposable non-root container persistence/recovery, release
validation, documentation links and diff checks pass. All seven published asset
hashes and release checksums match. Both OCI platforms have the corrected commit,
version and non-root user and contain no helper binaries or directory. Verified
replacement files are retained in `/tmp/torii-v0.2.0-corrected`. Native macOS,
Windows and arm64 execution remain unverified. The original clean main checkout
was fast-forwarded; the task worktree is retained for review.

## Superseded private v0.2.0 candidate (TASK-041)

[PR #40](https://github.com/Apartment38/torii-gateway/pull/40) is merged.
The original candidate tag pointed to `628032d9ab256de7a3adb4dec20849b77e049657`.
The withdrawn workflow run `38353272961` passed CI, all five binary targets and the multi-platform container build.
[v0.2.0](https://github.com/Apartment38/torii-gateway/releases/tag/v0.2.0)
originally contained ten binaries, `SHA256SUMS` and
`CONTAINER_IMAGE.txt`. Repository visibility remains private; an unauthenticated
registry pull returns HTTP 401. No deployment or visibility change was performed.

Downloaded build archives match their artifact digests. All twelve published
asset SHA-256 digests match local files, the release checksum file verifies,
native gateway/helper metadata matches the tag commit, and both OCI platforms
have the expected version/commit labels and non-root users. Obsolete downloaded artifacts were removed;
sanitized verification evidence remains in `/tmp/torii-v0.2.0-published`.
The withdrawn image index digest was
`sha256:1fcdf1b8318c06f902652fbe1b0a41e6381afa4ad13aa23706fca986afd2d368`.
Native macOS/Windows and arm64 execution remain unverified.

The first publish attempt failed while writing the arm64 manifest with
`permission_denied: read_package`. Its setup log explicitly granted
`Contents: write` and `Packages: write`; the operator confirmed existing
repository Admin access to the package. The failed-job rerun succeeded using
the same commit, artifacts, actor, Skopeo version and granted permissions.
No workflow permission edit, PAT substitution or tag movement was applied.
The original registry denial's root cause remains unconfirmed; successful
recovery does not establish a first-publication or transient-auth explanation.
Workflow-level package grants would not fix an already-granted publish-job scope.

## v0.2.0 release preparation

Prepared on `chore/prepare-v0.2.0` in `/tmp/torii-release-0.2.0`, based on
current `origin/main` (`faa6066`). Removed committed Python bytecode, excluded
generated Python caches and release artifacts from Git/container contexts,
corrected stale README testing claims and completed the beta feature summary in
[release notes](../releases/v0.2.0.md). Two deleted remote branches were pruned;
the original `main` checkout remains clean. No application behavior changed.

Validation: full Go tests and race tests, vet/build, formatting, unchanged module metadata,
console/auth and release-validator tests, generated console freshness, example
configuration validation, affected documentation links and diff checks pass.
All ten gateway/helper binaries cross-build for the five release targets;
checksums and native version/commit/build-time metadata verify. Validation
binaries are retained in `/tmp/torii-release-artifacts-0.2.0` and describe
implementation commit `7cf189d`; they must be rebuilt from the eventual tag.

The production Dockerfile builds with Go 1.25.1. The amd64 non-root candidate
image embeds baseline implementation commit `faa6066` (runtime code is unchanged),
reports `v0.2.0`, schema 10/baseline 4, and passes the disposable credential,
persistence, accounting/quota, session, backup/restore and wrong-key suite.
Host Go checks use Go 1.27.2 with a writable temporary cache and local test-server
access outside the sandbox. The initial race gate exposed a pre-existing reset
test that checked a six-second cooldown after expensive password verification;
the corrected test checks consecutive attempts before hashing and passes both
normally and with race detection. The complete race-gate rerun passes.

Release publication remains deferred (TASK-041). The maintainer must select the
reviewed commit and push its unused `v0.2.0` tag; release automation reruns the
full CI gate and produces the final multi-platform image and tagged assets.
No tag, push, release, registry publication or deployment was performed. Native
macOS/Windows execution, arm64 container execution and live provider/account
checks remain unverified; cross-builds and controlled fixtures do not establish
those results.

## Smart benchmark mapping and official model cards

Implemented on `feat/smart-benchmark-mapping` in
`/workspace/work/smart-mapping`. Benchmark Intelligence offers **Smart Mapping**
with a configured decision-model selector, cost warning, Start and cancellation.
Family search shortlists cached benchmark names before bounded System One calls;
validated high-confidence choices become source-specific identities through the
revision-checked configuration save. Manual mappings, ambiguous results, stale
models and provider credential boundaries retain their existing safeguards.
Models links identified publisher model/system cards and retains its mobile layout.
Affected benchmark/console guides and API reference are updated.

Validation: Go tests and race tests across all packages, build, vet, module tidy,
console/auth tests, release-validator tests, generated-script freshness, formatting
and diff checks pass. Go validation uses the downloaded Go 1.25.1 toolchain with
`-buildvcs=false` because VCS discovery selects the outer managed workspace;
module files are unchanged. Chromium fixtures exercise the complete Smart Mapping
save and model-card links at 1440×1000 and 390×844 without page errors or dialog
overflow. No live paid decision requests were made.

## Console design language consistency

Implemented on `codex/console-design-consistency` in
`/workspace/console-design-consistency`. Smart Routing groups its paid/free policy
switches with Router Name and Save/Remove. Add/Create actions share blue primary
buttons with SVG plus icons; Edit uses a pencil, and Remove/Delete uses red
buttons with minus/trash icons. Dense actions retain accessible names and tooltips
when icon-only. Authored headings, labels, navigation and buttons use Title Case;
page/section headings are larger. Operator names and protocol identifiers retain
their spelling. Design/contributor conventions and affected console guides are
updated; generated assets remain synchronized.

Validation: console/auth tests (157 cases), generated-script freshness, release
validator tests, affected documentation links and diff checks pass. Chromium
fixture checks cover nine console pages at 1440×1000 and 390×844 in light/dark
with no script errors, horizontal overflow, or unnamed icon actions; benchmark
Edit states preserve icons and accessible names. Full Go tests, race tests, vet and build pass. The initial full Go run hit an
unrelated intelligence request-pacing timing assertion during concurrent
compilation; the isolated test and full-suite rerun passed. No deployment or live
provider calls are part of this change.

## Administrator account and email password recovery

Implemented on `feat/admin-password-reset` in `/workspace/torii-admin-reset`.
Setup and CLI creation accept an optional recovery email. Server → Admin User
supports username/email/password changes with current-password reauthentication.
Login offers email recovery with a 30-minute, single-use link, transactional reset
consumption, session revocation, generic asynchronous request responses, bounded
workers/cooldowns, and verified TLS SMTP. Schema 10 preserves existing accounts
and adds only hashed reset tokens. Deployment requires public HTTPS/SMTP runtime
configuration; see the environment reference.

Validation: full Go tests, full race suite, vet, build, unchanged module metadata,
formatting, generated console freshness, 155 browser-script cases, and local docs
links pass. State/gateway tests and race checks were repeated after the final
migration/runtime changes; web tests and Chromium were repeated after the reset
page fix. Chromium covers actual setup/email/admin edits/relogin and recovery
requests, plus reset form submission against a mocked confirmation response at
1440×1000 and 390×844 without page errors or horizontal overflow. Go tests cover
actual transactional confirmation, expiry/replay/concurrency, legacy schema-9
upgrade, persistence, TLS SMTP delivery, and plaintext-downgrade rejection.

The non-root Docker image builds with a temporary proxy-CA secret mount, and a
disposable Docker smoke test verifies schema 10, email/account changes, CSRF,
reauthentication, revocation/relogin/restart persistence, generic reset requests,
and public reset-page headers. The full Podman credential suite was unavailable
because Podman and the host SQLite CLI are absent; its schema/setup assertions
were updated and shell syntax checked. Provisioned Go 1.25.1/caches use
`-buildvcs=false` because child builds cannot obtain VCS status in this sandbox.
No existing deployment or live-mail service was changed. Worktree retained for
review; deployment requires the documented HTTPS/SMTP environment configuration.
Review: [PR #34](https://github.com/Apartment38/torii-gateway/pull/34).

## Decision-model passthrough and provider namespaces

Implemented on `feat/jev-provider-passthrough` in `/workspace/torii-jev`,
starting from current main `d55248f` before implementation (verified against
GitHub, then fetched full history). Decision catalogs now carry System One
capability evidence and appear in public/management model discovery. The client
model list includes a TypeSafe-compatible `models` array alongside OpenAI `data`.
`POST /v1/systemone` transparently forwards physical decision models, with
provider credentials, aliases, model controls and operator accounting. Virtual
routers/non-decision models are rejected; decision-only models stay out of chat
pools. Mixed-provider decision models can run the smart-router judge.

OpenRouter additionally discovers `output_modalities=decisions`; a read-only live
check detected 480 total models including 21 decision models, with both
`typesafe/jev-1.13` and `~typesafe/jev-latest`. Detection uses catalog evidence,
not model names. TypeSafe uses profile/default ID `typesafe`; legacy `jev`
profiles remain accepted and saved IDs are retained until explicitly edited.
All provider IDs are editable. Saves update router references, model controls
and pricing while preserving upstream judge namespaces and API-key references.
Subscription IDs retain their OAuth binding; the editor offers login under the
new ID before save. Provider/API/configuration/console guides are updated.

Validation: all-package Go tests and race tests, vet, build, gofmt, unchanged
module metadata after tidy, generated-script freshness and 148 console tests
pass. Go commands use `GOFLAGS=-buildvcs=false` because this executor's VCS
stamping fails despite valid repository history. Protocol and judge tests use
controlled fixtures; the live check only reads public catalogs, with no billable
decision requests. Container/release checks are outside this change's scope. Review: [PR #33](https://github.com/Apartment38/torii-gateway/pull/33).

## Console monitoring and routing fixes

Implemented on `fix/console-monitoring-routing-balances` in
`/workspace/torii-console-fixes`, starting from current main `de98767`.
Activity Monitoring edits MiB while retaining byte-based API storage, including
fractional sizes. Prompt and Jev inspectors indent JSON and display multiline
text, with raw fallback for truncated/non-JSON bodies. Smart routing loads the
catalog on direct entry and refreshes after provider changes, filters disabled
connections, and retains in-flight discovery across unrelated policy saves.
Provider Balances renders one card per provider, deduplicating shared credit,
spend and quota evidence while retaining distinct model-scoped windows.
The console guide describes the updated units and behavior.

Validation: 144 console tests, generated-script freshness, all-package Go tests,
vet, build, formatting and unchanged module metadata pass. Desktop (1440×1000)
and mobile (390×844) Chromium checks pass with a mocked gateway, no page errors
and no horizontal page overflow. Go tests use inherited `GOFLAGS=-buildvcs=false`
for the imported main snapshot's shallow history. Browser and Go HTTP-server
tests run outside the socket-restricted sandbox. All-package race tests pass.
No live gateway data changed. Review: PR #32.

## Benchmarks page controls

Implemented on `feat/benchmark-page-controls` in
`/workspace/work/benchmark-page-controls`, starting from current main `4326118`.
The exact snapshot was imported through GitHub reads because the Git proxy is
unavailable; main is a local shallow boundary. Configure Ingestion and Upload
Data open separate overlay cards. The upload card no longer contains the loaded
measurement editor. Edit data switches the current table to inline measurement
fields with per-row Save/Cancel, draft retention and revision conflict handling.
Display Fields transfers available/enabled columns by double-click, buttons or
keyboard, with separate summary and measurement selections. Configured-only
measurement reads follow current catalog associations without granting display
matches routing authority.

Validation: 137 console tests, generated script freshness, all-package tests,
vet, build and module checks pass. Go checks use `-buildvcs=false` because the
imported main snapshot has a shallow history boundary. Chromium verifies
separate overlays, field transfers, row Save/Cancel and persistence with a mocked
gateway, at 1440×1000 light and 390×844 dark sizes, without page errors or page
horizontal overflow. All-package race tests also pass. No live gateway data changed.

## Smart routing editor

Implemented on `feat/smart-router-editor` in `/workspace/work/smart-router-editor`,
starting from main `49fcb92`, rebased onto main `fddcc2f`. Exact main snapshots
were imported through GitHub reads because the configured Git proxy is unavailable;
main is a local shallow boundary. Create router now opens a default
policy draft from the page header, beside the router selector. Router Name, Save
and Remove router share a section. Provider transfers update the model pool;
router selection restores its complete policy. New drafts wait for Save, while
existing policy edits retain automatic saving. Failed saves preserve drafts.

Validation: 133 console tests, generated script freshness, relevant Go web/config
tests, and all-package build (`-buildvcs=false`) pass. Chromium verifies router
selection, reset, provider double-click/filtering and persistence against a mocked
gateway, plus 1440×1000 light and 390×844 dark layouts without page errors or
horizontal page overflow. No deployment or live gateway configuration changes.

## Hermes auxiliary session titles

Implemented on `fix/hermes-title-generation` in
`/mnt/data/projects/torii-gateway/.worktrees/hermes-title-generation`;
commit `b5eff30`. Activity records showed successful smart turns followed by
three title-generation 503s. JSON-schema requests had no eligible candidate:
local Ollama structured-output support remained unknown, and
`reasoning_effort: "none"` incorrectly required reasoning capability.
Native discovery now advertises structured output for verified local completion
models; disabled reasoning no longer adds that gate. Cloud aliases and incomplete
metadata remain unknown. Regression tests preserve enabled-reasoning gates.

Validation: targeted baseline and final tests, full Go tests and race checks,
build, vet, unchanged module metadata, console tests/assets, affected documentation
links and diff checks pass. The original title shape now succeeds as
JSON and SSE with valid titles and stream termination. The installed Hermes
`generate_title` function returns a title without failure callbacks. Health,
discovery and missing/invalid-key rejection also pass. A full interactive Hermes
session was not run.

The running instance uses a patched `localhost/torii-gateway:local` image; the
original is retained as `localhost/torii-gateway:before-hermes-title-fix`.
No deployment source, Compose, environment or saved configuration was edited;
379 readable deployment file hashes and its original Git status are unchanged.
Existing unreadable configuration files were untouched. Normal activity/usage
records from verification remain in the existing database volume. A future image
rebuild must use the fixed source branch to retain this repair.


## Models pricing status and pagination

Implemented on `feat/models-pricing-pagination` in
`/workspace/models-pricing-pagination`, based on main `5b92ea7` before edits.
Commit `1dd4cc6` adds pricing status dots with hover/focus/click tooltips,
manual/stale/failed/unknown pricing states, and successful pricing timestamps.
Failed reads retain last known prices; bundled estimates keep their snapshot date.
Both table ends offer 25/50/100/All rows, numbered pages with truncation, and
Previous/Next. Models refresh actions stay grouped; administrator-only pricing
refresh forces provider catalog reads while retaining manual overrides. CSV import
opens a modal dialog. Console and API guides describe the behavior and limits.

Validation: main baseline checks, 130 console tests, full Go tests and race checks,
build, vet, module tidy (unchanged), generated assets, release-validator tests,
formatting/diff and affected documentation file links pass. Chromium checks cover
500-model pagination, synchronized controls, dot tooltips, pricing refresh and
import dismissal at 1440×1000 and 390×844, including dark theme, without page errors.
Go 1.25.1 was installed in workspace scratch; local builds use `-buildvcs=false`
because sandboxed worktree VCS stamping fails. Container checks are unavailable
because Docker/Podman and the SQLite CLI are absent. Live-provider checks and
deployment remain unperformed; pricing refresh is tested with a controlled upstream.

## Server timezone and Activity monitoring

Implemented on `feat/server-monitoring` in `/workspace/work/server-monitoring`.
Server settings expose an IANA display timezone, Activity size/retention limits
(default 100 MiB and seven days), and independent prompt/Jev debug capture toggles
(default off). Shared date formatters cover Activity, Overview, API-key dates,
benchmark timestamps and quota/reset displays; accounting periods stay UTC.
Activity inspectors show original API JSON and final Jev request/response bodies,
escape content as text, label 64 KiB truncation, and clear on dismissal/navigation.
Debug details persist with the events but are excluded from general activity logs.
Limit changes prune immediately; live views refresh after cleanup. No schema or
module dependency changes are required. Affected guides and references are updated.

Validation: full Go tests and race checks, build, vet, module tidy (no changes),
126 console tests, generated assets, release-validator tests, formatting/diff and
affected documentation links pass. Disposable Chromium checks cover persisted
settings, both inspectors, raw prompt preservation, malformed Jev response capture,
Escape dismissal and mobile overlays with no browser errors. Container validation
is unavailable because Podman and the host SQLite CLI are absent. Live-provider
checks and deployment remain unperformed.

## Console monitoring navigation

Implemented on `chore/sidebar-sections` in `/workspace/torii-sidebar`. Sidebar
sections are Monitor, Manage, and Configure. Provider Balances is a dedicated
Monitor page for provider balances, subscription limits, and resets; Usage & quota
retains operator accounting and budget controls. The new page loads independently
of API-key metadata, supports refresh and explicit empty/error states, and clears
cached telemetry when the administrator session ends. The console guide is updated.

Validation: console tests, generated-script freshness, and diff checks pass.
Submitted as PR #27. No deployment was performed.

## Known provider API URL defaults

Implemented on `fix/provider-url-defaults` in
`/workspace/work/provider-url-defaults`. Selecting any of the 15 hosted provider
integrations fills its complete API URL, including provider-specific path prefixes.
One table replaces the incomplete tracking-only defaults and conditional Nous/Codex
assignments. Switching types updates the endpoint; reopening a saved provider or
receiving an unchanged type event preserves its customized URL. Custom compatible
services still need an operator URL, and Ollama retains its detection workflow.
The provider guide now describes automatic defaults.

Validation: baseline and final console checks pass, including all 122 console
tests, generated script freshness, `go test ./internal/web`, guide links and
`git diff --check`. Regression checks exercise add-provider selection, visible
URL fields and actual connection-test payloads for every hosted integration, both
with blank drafts and previous endpoint values, plus custom URL preservation.
Backend provider behavior and dependencies are unchanged. No live provider calls
or deployment were performed.


## Codex browser login

Implemented on `fix/codex-browser-login` in
`/workspace/work/codex-browser-login`. The console replaces helper downloads and
terminal commands with an OpenAI browser tab and one-time device code. Gateway
polling matches the current upstream response/PKCE exchange contract and retains
account/client/issuer context in encrypted SQLite. Popup blocking leaves a visible
login link; pending sessions are reused and browser polling covers the 15-minute
lifetime. Helper APIs remain optional for explicitly requesting legacy clients.
The guide and API/CLI/development documentation reflect the new default.

Validation: full Go tests/race checks, build, vet, module tidy (no module changes),
formatting/diff checks, all 120 console tests, generated assets, release-validator
tests and affected documentation links pass. Targeted race checks also pass after
final login-test cleanup. Tests cover local/remote browser authorization, pending
polls, encrypted credential context, administrator access, token redaction,
redirect rejection, cancellation, blocked tabs, delayed completion and errors.
Go checks used the workspace's Go 1.25.1 toolchain with VCS stamping disabled;
mock-server checks required local socket access. Container credential verification
is unavailable because Podman and the host SQLite CLI are absent. Live OpenAI
sign-in and deployment remain unverified; account/workspace device authorization
must be enabled. No push, merge or deployment was performed.

Follow-up on `fix/codex-login-copy` in `/workspace/work/codex-login-copy`: the
one-time code appears in a read-only text box with a **Copy** button on both the
provider card and editor. Copy confirmation survives unchanged login polling;
clipboard failures select the code for manual copying. The provider guide is
updated. All 120 console tests and generated asset/diff checks pass, including
clipboard success, failure and selection preservation. Backend validation above
remains applicable; this follow-up changes console assets and documentation only.

## Manual model pricing

Completed gateway-wide `model_pricing` settings and the Models console pricing
editor/CSV upload. Input/output rates use USD per million tokens; partial rates
preserve automatic pricing for the other field, blank rates restore automatic
metadata, and explicit zero means free. Rates survive restart/rediscovery and
apply to Jev, deterministic selection, named routers and accounting, including
classifier models. Explicit provider free-model policy still takes precedence.
CSV import validates up to 1 MiB / 1,000 rows before the console's existing
revision-protected save; unlisted models remain unchanged. Administrator-only
API tools provide CSV validation/template download and per-model updates.

Validation: targeted baseline, full Go tests/race checks, build, vet, module
metadata, formatting, all 116 console tests, generated assets and affected
reference/guide links pass. Tests cover durable rates, revision conflicts,
CSV atomic validation, automatic-rate restoration, named routing/accounting and
administrator-only access. No database migration or dependency was needed.
Container verification remains unavailable because Podman and the host SQLite
CLI are absent. No deployment was performed.

## Jev model decisions and custom benchmarks

Implemented on `feat/jev-model-selection`. Jev now chooses a physical model for
each new prompt using dynamic criteria containing configured model prices,
capabilities, quality floors, context and usable task-relevant benchmark evidence.
The gateway validates the choice; tool continuations keep their originating model.
Generic classifiers and configured failure fallback retain deterministic selection.
Both configured pools and unrestricted runtime candidate pools are limited to 250.
The console uses available/enabled columns with double-click and button transfers.

Administrator CSV uploads merge durable custom measurements with an 8 MiB,
10,000-row upload limit and a downloadable documented template. Loaded measurements
can be edited individually with source-revision conflict checks. Custom data survives
restart/configuration reload and external refresh; publisher refreshes may replace
manual edits to publisher sources. No database migration or new dependency is needed.

Validation: targeted baseline checks passed before editing. Full Go tests, race
tests, vet, build, module metadata, formatting, console behavior/generated assets,
and affected documentation links pass. Tests cover actual Jev destination dispatch,
configured scope, zero-weight benchmark evidence, repeated prompt decisions, tool
continuity, administrator-only mutations, CSV rejection, persistence and stale edits.
Worktree builds used `GOFLAGS=-buildvcs=false` because sandboxed VCS stamping failed.
The Podman container verification remains unavailable: Podman and the host SQLite
CLI are absent. No live Jev quality evaluation or deployment was performed.

## Codex subscription quota tracking

Implemented on `fix/codex-quota` in `/workspace/work/codex-quota`: shared OAuth
refresh/retry and conditional encrypted rotation, verified upstream endpoint
selection, optional account-verified app-server reads, multi-bucket/duration
normalization, stale cache/backoff, background polling and console manual refresh.
Validation passes: Go package tests, changed-subsystem race tests, vet, build,
module/format checks, console/generated assets, release-validator tests and
modified documentation links. The full race suite fails the unchanged benchmark
`TestSharedLaneSpacesRequestsAndSerializesAcrossAdapters`; the same failure was
reproduced on the original checkout. The production container build and five
helper target builds pass. A disposable non-root image with the final gateway
binary passes startup, management authentication, console assets and restart
persistence. Podman-specific full credential/container checks were not run. The original live HTTP 401 remains unverified:
no affected credential or authenticated service is available in this environment.

## Alpha (v0.1.0) implementation and rollout

The alpha (v0.1.0) implementation and authorized Podman rollout are complete on
`feat/gateway-v1` in `/tmp/torii-gateway-v1`. SQLite now owns settings and revisions;
legacy YAML is used only for explicit transactional import/validation. The console
automatically saves validated edits, preserves conflicting drafts, and uses persistent
administrator cookie sessions. Model controls retain provider-qualified eligibility,
join existing benchmark identities, and honor explicit free-provider pricing. Named
keys have durable JSON/SSE accounting and atomic daily/monthly request, token and
estimated-cost quotas. Metadata-only activity history/SSE and structured rotating
logs are bounded. Codex helper instructions match the selected platform filename
and correctly quote shell arguments.

Full Go tests/race tests, vet/build, all 74 console tests, generated assets,
release metadata, module/format checks and affected documentation links pass.
Gateway and helper binaries cross-build for all five supported release targets.
The exact final image passed disposable non-root persistence/recovery tests,
including sessions, model controls, fragmented JSON/SSE/embedding usage, quota
restart recovery, redaction and wrong-master-key rejection. Acceptance also fixed
SQLite write-lock acquisition, conservative incomplete/classifier usage estimates,
and local model prices when the free-provider option is off.

Source commit `2c821fb` produced live image `cc85d358be44`. Live discovery,
authentication, cookie saves/conflicts, JSON/SSE/Smart inference, named-key usage,
activity and quotas passed; sessions and quotas survived restart. An image
identity check caught Compose retaining a container under a changed tag; forced
recreation and repeated acceptance verified the final artifact. Temporary test
settings were restored, the test key revoked and the session logged out. The
original database, master key, provider credentials, model IDs and benchmark state
were preserved. Stopped backups and rollback images remain in the deployment;
the final correction backup is
`backups/before-v1-accounting-final-20261008T043837Z`, and the original pre-alpha
backup is `backups/before-v1-20261008T035917Z`. Task-owned changes are replicated
into both authorized Torii directories. The alpha (v0.1.0) commit history is
integrated into the project's `main` branch. The review worktree is retained; release publication
remains deferred.

Current user-visible support is in [Features](../reference/features.md), with
contracts in [Design](design.md). The original Version 1 implementation is the
alpha (v0.1.0); historical Version 2 plans do not define current support. The first public beta and first
published release is [v0.2.0](../releases/v0.2.0.md).

## Earlier integration work

These retained records describe earlier work and historical checkout paths.
Current release preparation is summarized above.

Local `main` has been rebased onto merged provider-tracking PR #20, preserving provider balances/usage, scoped quota windows, Nous API-key authentication, local Ollama detection and named smart routers. Integration review added regression coverage for shared quota headers retaining independently collected billing data and for accounting wrappers preserving account-level cache identity. Full Go/race suites and final focused accounting/API/gateway race checks, vet/build, 109 console tests, two release-validator tests, generated assets, module/format checks, affected documentation links and exact-image disposable container persistence/recovery checks pass. The integration worktree is retained at `/tmp/torii-rebase-integration` on `fix/rebase-provider-router-integration`. No live deployment changes are part of this repair.

Expanded provider tracking is complete on `feat/provider-balance-tracking`
in `/workspace/work/provider-balance-tracking`, based on merged PR #19 and
retained for the requested follow-up PR. OpenRouter, OpenCode Go/Zen, Standard
Compute, Z AI, Kimi Coding, MiniMax, DeepSeek, xAI and Anthropic collectors expose
available usage, quota/reset windows and native-currency balances. Nous account
credits and Codex public/legacy usage paths, balances and banked resets are fixed.
Model APIs pass the fields to harnesses; selectors show provider usage separately
from gateway estimates. Billing credentials are encrypted, isolated, rotated on
read and use compatible-provider CA trust. Pricing fallback is dated/attributed;
live rates and operator overrides win. Scoped gauges do not block unrelated
models, zero OpenRouter dollar balances do not block free models, and response
headers cannot indefinitely extend billing snapshot freshness.

Full Go tests/race tests, final focused provider/quota/API race checks, vet/build,
module/format checks, 106 console tests, two release checks, generated assets,
affected documentation links and Chromium at desktop/mobile widths pass. The
production image and five login-helper targets built with an ephemeral proxy CA
secret; the final cache-expiry correction was rebuilt with matching Go 1.25.1
production flags and layered into the same non-root runtime. Disposable checks
verify authenticated balance/price discovery, inference usage/reset headers and
accounting/balances after restart. The shipping Dockerfile remains unchanged.

Public Zen wallet and Standard Compute budget amounts require an explicit private
endpoint because no supported public balance read was verified; available usage
and reset metadata are supported. xAI organization billing requires management
credentials/team ID; Anthropic organization reports require admin credentials,
and its subscription usage requires OAuth. External harnesses must render the
additive model fields. Live billing accounts were not exercised. No merge or
deployment was performed for this extension.

The six extension commits are also applied to the requested
`feat/provider-cost-usage` branch in `/workspace/work/provider-cost-usage`,
retained for review. Its implementation matches the validated tracking branch;
existing routing documentation remains on that branch's original history.

Provider/router fixes are implemented on `fix/provider-connections-smart-routers` in `/tmp/torii-provider-router-fixes`, pending provider telemetry acceptance. The changes cover classifier counts, in-card connection/save feedback, Nous API-key authentication and pricing, host-aware Ollama port detection and duplicate model IDs, Codex quota/reset metadata, named smart router pools with independent policy/session controls, and separate paid/free gates. Full Go/race tests, vet/build, 102 console tests, generated assets, release checks and final-image disposable persistence/recovery checks pass. Live Ollama port 55242 discovery, JSON/SSE chat, model-scoped router chat, provider-scoped router embeddings, Nous discovery (422 priced models), Nous chat and invalid-key rejection pass. The original deployment directory, image and stopped database snapshot were restored; health, original providers and absence of test routers were verified.

The saved Codex direct-scope token returns HTTP 401 for account quota polling and successful inference supplies no quota headers. Window/reset behavior is verified with fixtures; live account quotas need a supported credential or telemetry channel. No Nous API-key balance/usage/quota endpoint contract could be verified. Quota/reset metadata is exposed in the model catalog for harness consumption; no specific harness picker or attached browser was available for client display/visual acceptance. The operator has been asked how to resolve the telemetry limitations. The operator took responsibility for publishing and withdrew the PR request; the fixes were merged into local `main` before this rebase.


The premium console redesign is complete on `feat/premium-console` in
`/tmp/torii-premium-console`, retained for review. Separate builders and independent
running-browser critics iterated against Routerly screenshots and workflow docs.
Both final reviewers prefer Torii on matched console tasks; catalog density and
scoped accounting remain ties. The operational overview, restrained shell,
provider choice/editing, searchable catalogs, request investigation, scoped usage,
compact benchmark evidence and responsive navigation preserve existing controls.
Real disposable UI checks verify setup, key lifecycle, provider onboarding,
clipboard values, cancellation, deep links, themes and keyboard focus. All 99
console tests, full Go/race tests, vet/build, generated assets, affected links and
disposable non-root container validation pass. No blocking UI issue remains.
The [visual review page](evaluations/console-design/index.html) preserves captures,
critiques, comparison limits and [validation](evaluations/console-design/validation.json).
These subjective comparisons do not establish whole-product superiority. No push,
merge, publication or deployment was performed.

Alpha (v0.1.0) implementation and authorized rollout are complete. Release
publication and platform-native checks remain below. The Smart-first catalog repair and
configuration revision protection are deployed and validated.
Hermes remains unchanged as requested; its model-label formatting and custom
endpoint Fast controls remain outside scope. Its saved-endpoint re-test also
requires re-entering the actual key rather than a credential preview.

## Pending release checks and future work

The [README feature table](../../README.md#features) lists future scope. The
roadmap below also retains the planned client plugins:

| Feature | Planned scope and current boundary |
| --- | --- |
| MCP Gateway | Aggregate Model Context Protocol servers; current model tool-call transport does not host an MCP gateway |
| Skill Gateway | Centralized version-controlled skill registry; no skill storage/versioning/serving in the current gateway |
| TUI | Terminal interface with CLI/web-console feature parity; current interfaces are CLI, HTTP API and browser console |
| Multiple users | Per-user usage, quotas and allowed providers/models/tools/skills; current single-admin installation has named client-key accounting/budgets, without per-user permissions |
| Authentik Integration | Authentik web-console login; current administrator session/Basic authentication has no OIDC integration |
| Hermes plugin | Dedicated client integration; Hermes can already connect through the generic OpenAI-compatible endpoint |
| Pi plugin | Dedicated client integration; no Torii-specific Pi plugin is shipped |

No delivery dates or plugin APIs are committed. See
[Features and limitations](../reference/features.md) for current support.

- Before publishing a release, rerun the CI gate and disposable container
  persistence/recovery check, complete compatibility notes, verify all artifacts,
  and run opt-in live checks for the provider/account combinations being claimed.
  Code and local test completion is distinct from publication (TASK-041).
- Earlier documentation research could not fetch release availability or current
  GitLab template bodies. Archived first-party tutorial/how-to materials were inspected
  for structure. Installation keeps source builds as a complete path and refers
  to actual release assets rather than claiming a release exists.
- Docker, macOS, and Windows native/service workflows need validation on their
  respective hosts; documented build targets are not platform test results.
- Future scope, currently unimplemented: Authentik/OIDC, automated master-key rotation, stateful Responses,
  automatic updater, and MCP/tool/skill gateways.
- Performance measurements beyond the transparent dispatch microbenchmark require
  a reproducible harness; no production answer-quality, savings, actual billing,
  or general comparative routing superiority is established by the retained evaluation.

## Completed milestones without separate old task records

### Smart-first client model catalog — 2026-10-07

Hermes defaults to showing the first 50 models per provider. Adding seven Codex
models moved the trailing virtual `smart` entry beyond that initial list. Torii
now emits `smart` first from `/v1/models`, followed by the unchanged, sorted
physical models. A regression catalog with 60 physical models verifies that
Smart remains on the first client page without changing or omitting other IDs.

The user confirmed Hermes must remain unchanged. Read-only checks reproduced
its stripping of provider prefixes from labels and rejection of Fast overrides
through custom proxy endpoints. Those behaviors were not changed; client/API
documentation explains visibility, **Refresh Models**, and these boundaries.

API baseline, final full Go tests/race tests, vet/build, formatting, generated
console freshness, unchanged module metadata, affected documentation links and
`git diff --check` passed. Built the standard image at source commit `2af3bc2`
and deployed it through the existing Podman Compose project and volume after a
protected stopped database/configuration backup; the previous image is retained.
Live requests returned all 57 models with Smart first and exactly the same
physical IDs, metadata and order. Provider configuration matched before/after;
valid-key access, wrong-key rejection and container health passed. Hermes's
unchanged installed discovery/parser also received Smart first among 57 entries;
its code, configuration and model cache were not modified. Users must choose
**Refresh Models** to replace their cached catalog. Task-owned source files were
replicated into both authorized Torii checkouts; the task worktree is retained.

### Configuration save conflict protection — 2026-10-07

Reproduced an outdated console's full-document save restoring static `ziz/setup`
settings while adding Codex. A fresh console preserved native Ollama settings;
Codex itself did not rewrite the other provider. Logs correlate a later save
with the reversion, but historical request bodies are unavailable, so that
specific stale-tab incident remains inferred.

Configuration reads now supply an opaque `ETag`. Both PUT and POST saves require
its exact `If-Match` value: missing revisions return 428 and stale revisions 412,
before credential storage or handler construction. Serialized saves advance a
revision even for unchanged documents. Meaningful external YAML edits and
restarts invalidate old revisions. The console retains conflicted drafts and
requires reload/reconciliation; repository configuration-writing scripts and
API/provider/design documentation follow the new required-header contract.

Baseline checks passed. Final full Go tests/race tests, vet/build, module/format
checks, all 70 console tests, generated script freshness, script syntax, affected
documentation links and diff checks passed. The standard image at source commit
`b9147e3` passed disposable non-root credentials, persistence, rotation, benchmark
console/API and stopped-backup recovery checks, then replaced the live Compose
container with the original volume intact. The previous image and a protected
stopped configuration/database archive are retained for rollback; the master key
was preserved separately in the unchanged deployment environment.

Restored native `ziz` Ollama with a five-minute inference timeout while preserving
Codex, Jev and routing configuration. Live checks confirmed 57 catalog entries
(49 Ollama, seven Codex, `smart`), missing/stale revision rejection, valid saves,
wrong-key rejection, restart persistence and pre-restart revision rejection.
Small Ollama JSON/SSE and smart inference requests returned content. Hermes's
unmodified installed discovery/parser found all 57 entries using the supplied
key; the desktop form itself was not operated. No Hermes code/configuration or
upstream credentials were changed. Task-owned source files were replicated to
both authorized checkouts; the task branch/worktree is retained without merging.

### Configurable Ollama inference deadline — 2026-10-07

Ollama inference now defaults to a five-minute whole-request deadline instead of
being unbounded. Positive `request_timeout` overrides are accepted in YAML and
the provider card; clearing the console field restores the default. Model
discovery retains its independent ten-second deadline, and client cancellation
still propagates upstream. Updated the example, configuration reference and
provider/client/design explanations. Shorter client/proxy deadlines remain
outside Torii's control; no Hermes code or configuration was changed.

Relevant baseline checks, final full Go tests/vet/build, module/format checks,
full race checks plus affected-package races after discovery isolation, 67 console
tests/generated-script freshness, example YAML, documentation links and diff
whitespace passed. An isolated gateway completed a simulated 32-second cold load;
short and longer live timeout overrides behaved correctly. Regression tests
also cover stream cancellation and independence from discovery. Rebuilt the
Podman image, passed its disposable non-root persistence/recovery suite, saved
`5m` on the existing deployment's Ollama provider, and verified 50-model discovery,
smart chat, the deployed console control and wrong-key rejection. Both project
copies contain the change; previous deployment image/configuration are retained.

### Podman/Hermes connection troubleshooting — 2026-10-07

Replaced a deployment's static `setup` model with native Ollama discovery; the
public catalog now returns 49 upstream models plus `smart`. Jev remains a
classifier-only provider. Added a console warning when connection-test discovery
differs from the static catalog that saving will expose, with a regression test
and provider/client guidance. Replicated project changes to the deployment and
source checkouts and rebuilt/recreated the existing Compose service with its
database intact; retained the previous image and configuration for rollback.

Baseline API/provider/configuration/gateway tests passed. Final Go formatting,
module metadata, vet, build, full tests and race tests passed, as did console
tests/generated-script freshness, affected documentation links, diff whitespace,
and the fresh non-root image's disposable credential/persistence/recovery suite.
Live repeated discovery, persisted-key authentication after recreation, explicit
chat, smart JSON/SSE chat, and wrong-key rejection passed. A cold larger model
exceeded a 30-second client deadline; warmed requests completed under one second.
An isolated fresh Hermes conversation using the already-installed runtime also
passed. Live checks establish connectivity, not general model quality or latency.

The installed Hermes runtime resolves the saved key correctly. Its re-test
validator rejects blank and literal credential-preview submissions; a proposed
saved-key lookup was tested in memory with endpoint/identity isolation and
wrong-key rejection, but no Hermes files were changed. A schema-constrained
request can still fail smart selection because native Ollama discovery does not
verify structured-output support; the existing capability gates remain intact.

TASK-021/022: endpoint-bound Codex credentials, device login, PKCE/remote helper,
refresh and translated Chat/Responses/SSE. TASK-025–027/029–033: connected,
embedding/typed classification, smart endpoint, session affinity/tool safety,
and bounded pre-stream retry. TASK-035/036: SQLite aggregate usage, cost/quota/
latency integration. TASK-037–040: strict schema/CLI, compatibility/security suites,
non-root container and performance harness. These were locally implemented/tested;
release-environment checks remain separate above.

Previously called Version 2 tasks are implemented: TASK-013 OpenRouter discovery,
transparent inference/metadata/quota and separate credit key; TASK-015 Gemini
compatibility; TASK-016 xAI compatibility; TASK-018 Anthropic Messages Chat/basic
Responses/streaming translation; TASK-023 Nous subscription login/refresh/catalog/
quota. Typed Jev and Jev-compatible adapters are implemented. Their limitations
are in the provider reference, not a deferred-adapter backlog.

### Smart-routing evaluation — 2026-10-07

The final scoped critic acceptance resolved the earlier quota checkpoint's pending
usage decoding/fixture/test issues. Retained evidence records 21 final gateway
cases, matched pinned-reference constraint checks, default setup/customization,
and two measured live Jev decisions with synthetic inference output. No general
production-quality/economic claim is made. See [Evaluation](evaluations/smart-routing/README.md).
Temporary continuation/HQ/account-quota instructions are retired.

### Documentation refresh — 2026-10-07

Reconciled current feature/authentication/persistence claims; simplified README;
added complete installation/tutorial/guides/reference; moved technical explanations
and evaluation evidence under development; consolidated the old task ledger,
implementation-plan status, and checkpoints here. Preserved release tooling paths
and migrated artifact consumers. Validation evidence is recorded below.

Validation completed for this refresh:

- Built gateway and helper from source; both version commands succeeded.
- Validated the full example configuration and two standalone documentation
  configs; parsed all 13 YAML blocks, with fragments kept as excerpts.
- Local Markdown file/anchor checks, stale-path search, and `git diff --check`
  passed. All 17 frozen artifacts, including the anonymous review, are preserved
  byte-for-byte at their new location; relocated Python consumers parse.
- Native disposable-fixture smoke passed fresh setup, administrator/bearer
  boundaries, provider test/save/discovery, JSON inference, exact SSE, named
  client keys, restart persistence, and revocation. No real provider calls.
- Existing local `localhost/torii-gateway:benchmark-files` image passed the
  disposable non-root credential container suite: schema 5, setup/rotation,
  named keys, benchmark console/auth/streamed refresh, usage/identity persistence,
  coherent stopped backup/restore, and wrong-key rejection.
- The same existing image passed the documented rootless Podman Compose
  file-secret/ownership/bootstrap/first-admin flow and persisted key after
  down/up in a unique temporary project. Deployment containers/config/data
  were not used. This validates the installation procedure against that image;
  it is not a fresh production image build or registry-backed release test.
- `go test ./internal/state` and release-metadata Node tests passed. The only Go
  source edit repairs the schema-error documentation link; application behavior
  is unchanged. Docker, macOS, Windows, and live upstream checks were not run.


## Historical completed task records

The following dated records preserve task IDs, rationale, tests, results, and
limitations from the original ledger. Their historical wording is not a current
feature specification or active TODO list. Follow current guides for deployment.

### Benchmark file ingestion and console organization — 2026-10-07

Completed native revision-pinned Parquet downloads for LiveBench/Arena, with
bounded temporary storage, complete-shard publication, credential-safe redirects,
and explicit auto/rows/parquet transport compatibility. Added optional encrypted
Hugging Face token controls and display-only configured-model matching that
preserves variants and follows the current provider catalog. Made the toolbar
sticky and top-aligned; moved database path to Server, model failure policy to
Routing, and default API-key rotation to Credentials. Added container listen
address guidance and updated configuration/examples and source research notes.

Validation: full Go tests and race suite, focused final race checks, vet/build,
console generation and behavior tests, release checks, and headless layout checks
across both themes and desktop/narrow screens passed. The combined anonymous live
refresh published 60,372 LiveBench and 10,923 Arena observations using six HTTP
requests and zero /rows calls. The final Go 1.25.1 production image built and ran
as nonroot with its healthcheck; all Parquet fixture tests also passed inside it.
LiveBench's public dataset remains dated 2025-04-07; expired evidence is still
excluded from routing. Generic/custom HF sources retain rows by default.

### TASK-014 — Verify the normal OpenAI API profile

Completed the origin-locked normal OpenAI API profile over the existing
transparent compatible transport. The profile requires an external bearer-key
reference, rejects alternate origins and generic header/model-rewrite options,
supports optional project and organization headers, and remains isolated from
the persistent Codex credential store. Official API reference URLs and the
2026-09-30 verification date are recorded in `docs/development/provider-notes.md`.

### TASK-034 — Implement quota and allowance state

Goal:
Represent known and unknown quota consistently and cache provider snapshots.

Completed:
- added normalized known/unknown quota, remaining fraction, and exhaustion state
- added local allowance injection, TTL expiry, invalidation, and finite-value validation
- coalesced concurrent refreshes per model and preserved caller cancellation
- added cache, unknown-state, exhaustion, local allowance, and race coverage

Decisions:
- unknown quota is never interpreted as exhausted or unlimited
- collection failures have bounded exponential backoff; Codex preserves last-known-good evidence

### TASK-028 — Implement deterministic model selection

Goal:
Choose the cheapest eligible physical model using explainable deterministic policy.

Completed:
- added hard endpoint, feature, context-headroom, availability, quota, and free-only filters
- added configurable complexity, tool, and reasoning quality thresholds with
  unknown-quality fallback only when no known-quality model qualifies
- added upper-bound input/output cost estimation and zero monetary cost for local models
- added quota, location, EWMA latency, context headroom, and lexical tie-breaking
- returned per-model debug reasons and added table and order-independence fuzz tests

Decisions:
- unknown endpoint or required-feature support is not treated as verified support
- unknown price is more expensive than known price and is excluded in free-only mode
- unknown latency is neutral and therefore does not beat measured latency solely
  because it happens to be represented as zero

### TASK-024 — Implement request requirements and classifier contracts

Goal:
Define objective request requirements and one validated classifier contract.

Completed:
- added the fixed alpha (v0.1.0) task, complexity, requirement, preference, confidence,
  failure-policy, request, classifier, and runner contracts
- extracted endpoint, tools, images, structured output, reasoning, streaming,
  approximate input size, and output limits without modifying raw request bytes
- made objective requirements override semantic classifier output
- bounded classifier calls, prohibited `smart` recursion, and implemented the
  deterministic general/medium capability fallback
- added malformed schema, extraction, recursion, timeout, and fallback tests

Decisions:
- estimated input tokens use a deterministic conservative byte heuristic until a
  provider tokenizer exists and are not presented as billed usage
- classification describes work and requirements only; it never names a model

### TASK-020 — Add authentication administration CLI

Goal:
Provide validated, non-interactive administration of provider credentials.

Completed:
- added `gateway auth login`, `logout`, and `status` commands
- required the provider to exist in a strictly decoded configuration
- accepted access and optional refresh tokens only through environment variables
  or files and persisted endpoint-bound credentials
- reported only provider state, including expiry, without printing tokens
- added login/restart/status/logout, redaction, and invalid-configuration tests

Decisions:
- token values are never accepted as command-line arguments
- OAuth-specific interactive acquisition remains owned by each OAuth provider

### TASK-019 — Add secure persistent credential storage

Goal:
Persist provider-scoped OAuth credentials safely. The original file-based implementation
recorded below is historical and has been superseded by encrypted SQLite schema 4.

Completed:
- added a versioned provider-bound credential record and store interface
- added restrictive auth-directory and credential-file permissions
- added size-bounded, strict, corruption-safe reads with secret-safe errors
- added same-directory atomic replacement, file and directory syncing, deletion,
  restart persistence, and provider-isolation coverage

Decisions:
- every stored credential is bound to both a validated provider ID and endpoint
- unsafe permissions, symlinks, unknown fields, trailing data, and wrong provider
  records are rejected instead of repaired or partially loaded

### TASK-017 — Define the canonical translated request/response model

Goal:
Define the translation-only contract needed by the Codex transport without
changing transparent compatible traffic.

Completed:
- added canonical chat/Responses requests, messages, multimodal content, tools,
  tool calls/results, structured output, reasoning, output limits, and extensions
- added canonical responses, normalized usage and finish reasons, and streaming deltas
- added strict validation for lossy or ambiguous request states
- added round-trip and invalid-contract tests

Decisions:
- unknown translated fields must be retained as validated raw JSON extensions or
  rejected by an adapter; silent loss is forbidden
- transparent OpenAI-compatible providers continue to use raw request bytes and
  do not instantiate the canonical model

### TASK-012 — Add configurable generic OpenAI-compatible providers

Goal:
Add origin-isolated, configurable standard OpenAI-compatible upstreams.

Completed:
- added optional discovery or static catalogs, model aliases, and transparent forwarding
- added environment/file-backed bearer and custom-header secrets with unsafe-header rejection
- added independent connection, request, and discovery timeouts plus TLS CA/server-name configuration
- disabled redirects and isolated clients, credentials, TLS, and connection pools by provider
- added configuration, discovery, rewriting, passthrough, streaming, and credential-isolation tests

Decisions:
- custom header values are secret references rather than inline strings
- redirects are returned rather than followed so credentials never leave the configured origin
- generic providers use the existing byte-preserving compatible transport

### TASK-011 — Build the model catalog and capability metadata core

Goal:
Build deterministic, immutable model snapshots without inventing unknown
capabilities or making one unavailable provider hide every other provider.

Completed:
- added normalized model descriptors for endpoint support, capabilities,
  context, price, quality, locality, availability, and provenance
- represented capability knowledge explicitly as supported, unsupported, or
  unknown and embedded the bundled JSON catalog in the binary
- implemented deterministic live, bundled, and configuration override merging
- added startup discovery, sorted list/retrieval views, collision detection,
  partial-provider refresh, bundled fallback, and last-known-good caching
- made startup fail when no provider or bundled fallback is usable while
  allowing a partial catalog when at least one provider remains usable
- added merge, ordering, immutability, collision, stale-cache, fallback, API,
  and partial-provider tests

Decisions:
- the bundled catalog begins empty rather than guessing model capabilities;
  verified provider work will add records as primary-source research lands
- catalog snapshots contain value types and deep-copy optional numeric fields
  so callers cannot mutate catalog state
- public OpenAI model objects remain compatibility-focused while the richer
  normalized descriptors remain the internal source for later smart routing

### TASK-010 — Normalize public errors and request IDs

Goal:
Provide shared typed failures and safe request correlation across the public API.

Completed:
- added the shared OpenAI error envelope and typed internal failure taxonomy
- mapped authentication, request, routing, provider, upstream, capability,
  quota, classifier, and stream failures without matching error strings
- accepted safe client request IDs or generated cryptographically random IDs
- returned the gateway request ID on every response and propagated it to the
  selected upstream while preventing an upstream ID from replacing it
- added structured `slog` completion records containing request metadata only
- tested status/code mappings, propagation, generated IDs, typed wrapping, and
  log redaction invariants

Decisions:
- accept one request ID of at most 128 characters from a conservative ASCII
  alphabet; replace missing, duplicate, or unsafe values
- keep request IDs in a small context package so transports do not depend on
  the HTTP API package
- log method and URL path but never query values, headers, bodies, or credentials

### TASK-009 — Complete transparent OpenAI endpoint coverage

Goal:
Expose the complete transparent OpenAI-compatible endpoint set supported by
the Ollama MVP.

Completed:
- added transparent Completions, Embeddings, and stateless Responses handlers
  alongside Chat Completions
- preserved unknown request fields and the existing 32 MiB body limit across
  every inference endpoint
- relayed JSON and supported streaming responses through the shared transport
- rejected `previous_response_id` and `conversation` with an OpenAI-style
  `unsupported_parameter` error
- added transport, provider, handler, and end-to-end endpoint coverage

Decisions:
- use one provider forwarding contract for the compatible endpoint set so all
  endpoints retain identical byte-preservation and cancellation behavior
- treat the presence of a stateful Responses field as unsupported, including a
  null value, rather than silently changing request semantics

### Pre-alpha usage documentation

Prepared public-facing documentation for the currently implemented pre-alpha
feature set and a configuration example.

Completed:
- documented the project's purpose, implemented feature set, limitations,
  source installation, configuration schema, API usage, and operational notes
- added a safe example configuration using an environment-backed client API
  key and a local Ollama provider
- added a test that loads the committed example through the strict production
  configuration decoder to prevent it from becoming stale or invalid

### TASK-008 — SSE streaming relay

Goal:
Relay streaming Chat Completions with low overhead and prompt cancellation.

Completed:
- enabled `stream: true` on the transparent OpenAI-compatible transport
- advertised `text/event-stream` to upstreams for streaming requests
- relayed upstream stream bytes without parsing or rebuilding SSE events
- flushed each copied upstream chunk promptly instead of buffering the response
- retained request-context cancellation through the upstream request
- added transport and end-to-end prompt-flush coverage

Decisions:
- preserve SSE payloads byte-for-byte so unknown, tool-call, and reasoning
  deltas remain compatible
- terminate a failed stream rather than attempting a mid-stream provider switch

### OpenAI-compatible transport hardening

Hardened the shared OpenAI-compatible path by preserving escaped trailing
base-path slashes, bounding Ollama model discovery, validating model-catalog
arrays, rejecting top-level trailing commas, and stripping all applicable
hop-by-hop response headers, including `Proxy-Connection`.
### TASK-006 and TASK-007 — Transparent transport and Ollama provider

Goal:
Forward OpenAI-compatible requests without dropping unknown fields, and support
model discovery plus non-streaming Chat Completions through Ollama.

Completed:
- added a reusable OpenAI-compatible transport with byte-targeted top-level
  model rewriting, context cancellation, shared HTTP-client reuse, and exact
  preservation of every other request field
- added Ollama model discovery through its OpenAI-compatible model endpoint
- exposed discovered models under provider-qualified public IDs
- added OpenAI-compatible model-list, model-retrieval, and non-streaming Chat
  Completions handlers
- explicitly reject streaming requests until TASK-008
- added transport, provider, and end-to-end handler contract tests
- wired configuration loading, Ollama providers, bearer authentication, health
  checks, and the OpenAI handler into the `gateway serve` executable
- added a process-level test that builds and starts the gateway and requests its
  authenticated model catalog
- ensured graceful shutdown waits for active requests to finish, up to the
  configured 10-second shutdown deadline
- made routing-field extraction case-sensitive so unknown case variants cannot
  override the requested provider or model
- extract routing fields without copying unrelated large request values, return
  413 for bodies over 32 MiB, and distinguish client validation failures from
  upstream failures
- validate configured bearer keys at startup and bound idle HTTP connections
- preserve escaped provider base paths and remove response headers named by
  upstream `Connection` tokens
- preserve escaped trailing base-path slashes, strictly reject trailing commas,
  strip proxy connection headers, validate model catalogs, and bound model
  discovery calls without limiting inference
- validate complete, size-bounded model-catalog documents and require every
  catalog entry to be an object

Decisions:
- gateway client authorization is never forwarded to Ollama
- upstream status, body, and end-to-end response headers are relayed for Chat
  Completions while hop-by-hop headers are removed
- model discovery currently fails the aggregate request when a configured
  provider is unavailable; partial-catalog policy remains future work

---

### Continuous integration workflow

Added a minimal GitHub Actions workflow for pushes to `main` and pull requests
targeting `main`. CI verifies formatting and module tidiness, runs `go vet`, the
standard and race-enabled test suites, and builds all packages using the Go
version declared in `go.mod`.

---

### Documentation workflow clarification

Clarified that documentation updates are required for durable changes to
architecture, public behavior, configuration, developer workflow, and
significant design decisions. Narrow task-specific test commands no longer need
to be added to `docs/development/contributing.md` unless they are reusable development
procedures.

---

### TASK-005 — Provider/model ID parsing

Goal:
Parse public model IDs into validated provider namespaces and upstream model
IDs.

Completed:
- added provider ID validation shared by configuration and model ID parsing
- split public model IDs on the first slash only
- rejected missing separators and empty upstream model IDs
- added tests for simple and OpenRouter-style nested model IDs

Decisions:
- parsing returns no partial provider or model value when validation fails
- upstream model IDs are otherwise preserved verbatim, including nested slashes

---

### TASK-004 — Gateway bearer authentication

Goal:
Protect gateway API handlers with the configured bearer API key.

Completed:
- added the extensible client authenticator and identity contracts
- added static `Authorization: Bearer` authentication for the v1 API key
- compared fixed-size API-key digests in constant time
- added OpenAI-style JSON authentication errors and bearer challenges
- added focused middleware, credential rejection, and identity tests
- made the middleware fail closed for missing authenticators and identities

Decisions:
- all missing, malformed, duplicate, and invalid credentials return the same
  response to avoid exposing authentication details
- store and compare SHA-256 digests so comparisons remain fixed-length even
  when a presented credential has a different length

---

### TASK-003 — Secret references

Goal:
Load secrets referenced by environment variables or files without allowing
secret values in YAML.

Completed:
- added a reusable `SecretRef` with strict exactly-one-source validation
- added environment-variable resolution without altering the stored value
- added file resolution with surrounding whitespace trimming
- added optional server API-key secret configuration for strict YAML loading
- added focused validation, resolution, and configuration decoding tests

Decisions:
- environment values are preserved exactly; only file-backed values are
  whitespace-trimmed as required
- missing environment variables and unreadable files fail resolution without
  exposing secret values in errors

---

### TASK-002 — YAML configuration loader

Goal:
Load and validate the initial gateway configuration.

Completed:
- added strict single-document YAML decoding with unknown-field rejection
- added file-based configuration loading
- validated listener and state settings plus the minimal Ollama provider schema
- validated provider IDs, uniqueness, supported type, and HTTP(S) base URLs
- added focused loader and validation tests

Decisions:
- used `gopkg.in/yaml.v3`, because the Go standard library has no YAML decoder
- intentionally reject configuration for providers and subsystems that are not
  implemented yet

---

### TASK-001 — Bootstrap Go project

Goal:
Create the initial Go project and repository structure.

Requirements:
- Go module
- cmd/gateway/main.go
- internal package directories required for Milestone 1
- docs directory
- project builds
- no unnecessary dependencies

Acceptance:
- `go build ./...` succeeds
- `go test ./...` succeeds

Completed:
- initialized the Go module and gateway command
- added the Milestone 1 package skeleton for API, configuration, providers,
  and upstream transports
- kept the module dependency-free
- documented and verified the repository-wide build and test commands

---
