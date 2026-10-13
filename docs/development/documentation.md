# Writing and maintaining documentation

## Structure and quality bar

Use tutorials for a first successful outcome, how-to guides for tasks, reference
for complete supported syntax/limits, and development explanation for internals.
Write for semi-technical users: explain terms, identify which machine/terminal
runs each command, list prerequisites, give numbered actions and expected results,
then link remedies and the next task. Use exact settled console labels.

The [Awesome Documentation collection](https://github.com/pengqun/awesome-documentation)
links The Good Docs Project tutorial, installation, how-to, reference, and
troubleshooting templates, plus documentation-system guidance. This docs layout
adapts those document types and outcome/prerequisite/steps/verification structure;
it uses original Torii-specific prose rather than copying template text. The current
GitLab bodies were unavailable, so the archived first-party
[tutorial template](https://github.com/thegooddocsproject/templates/blob/dev/tutorial/tutorial-template.md)
and [how-to guidance](https://github.com/thegooddocsproject/templates/blob/dev/how-to/about-how-to.md)
were inspected as structural references. Their overview, prerequisites, sequential
tasks/results, and next-step structure informs Getting Started and task guides.
Check current licensing/attribution before copying template text.

[arc42](https://arc42.org/) informs the compact architecture's goals, constraints,
context, blocks, runtime/deployment, cross-cutting concerns, decisions, and risks.
[Stripe's development hub](https://docs.stripe.com/development) supplies the bar
for clear entry points to tools, reference, testing, upgrades and security.
[Docker Getting Started](https://docs.docker.com/get-started/) supplies the bar for
choosing installation then following a focused tutorial. No docs-site framework
is needed for this Markdown migration.

## Rules

- Match code/contract tests rather than historical status claims. Mark dated
  research/evaluation as historical and retain its qualifications.
- Keep README short: description, complete quickstart, full-docs/development links,
  license. Put detailed instructions and reference in docs.
- Store architecture/design/research/progress under development. Only `status.md`
  owns active work, completed milestones, and backlog.
- Specify type, default, conditions, units, allowed values, and precedence in
  configuration reference. Distinguish code defaults from example settings.
- Separate full runnable configuration from labeled excerpts. Explain placeholders,
  consistent working directories, writable data paths, and one-time key generation.
- Keep all saved secrets out of examples. Show passwords through prompts and keys
  through clearly identified placeholders. Do not reveal existing installations.
- Preserve useful information before deleting old files; repair links in code
  comments/scripts/workflows and relocate artifacts with evidence documents.
- Keep release notes at `docs/releases/` with the template's required sections;
  `scripts/check-release.mjs` and the workflow depend on that contract.

## Refresh preservation record

| Previous README/document topic | Current home |
| --- | --- |
| Purpose/logo/license and brief quickstart | Root README |
| Requirements, source build, downloads/native launch | Installation overview/source/native |
| Compose setup, secrets/ownership/mounts/ports/SELinux, host Ollama | Container installation |
| First admin, named/default keys, credential replacement | Getting Started and Credentials |
| Console pages, themes, save/reload | Console guide |
| Multiple/provider profiles, aliases, remote login, auth commands | Providers guide and provider/CLI references |
| Benchmark sources/terms, keys, tables/refresh, aliases/weights | Benchmarks guide, Configuration/API, development Intelligence |
| Backup, baseline reset, upgrades, rollback | Backup and upgrade |
| Verification/curl/streaming/client base URL | Getting Started, Clients, API |
| Discovery/limits/timeouts/logs/shutdown/TLS | Operations, provider/API reference |
| Encryption/migrations/usage flush, translation/catalog/selection mechanics | State/security, Architecture, Design, Routing, Intelligence |
| Developer workflow and release process | Contributing |
| Task IDs/results, plan/checkpoint completion, genuine backlog | Status |
| Frozen comparisons, screenshots, raw records | Development evaluations |

The obsolete implementation plan's repeated coding/handoff/quota boilerplate and
old milestone TODO specifications were removed. Durable implemented contracts
were reconciled into Design; dated completed results and unique evaluation
specifications were preserved. Git history retains obsolete prose.

## Validation

Check local links and heading anchors, obsolete path references, standalone YAML
validation, CLI flags, and documented binary builds. Exercise first setup,
provider discovery, inference, SSE, authentication, and restart persistence with
controlled upstreams in disposable storage. Use available runtimes only; label
unverified Docker/macOS/Windows/live-service checks explicitly in status.
Run relevant console/release checks when those consumers change. Routine docs
edits do not require tests duplicating prose or unrelated code coverage.
