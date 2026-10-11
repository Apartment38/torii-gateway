# Development

Torii runs as one Go process with embedded SQLite and a dependency-free console.
User-facing procedures are in the [documentation home](../README.md).

| Topic | Document |
| --- | --- |
| Build, console generation, tests, releases, contributor workflow | [Contributing](contributing.md) |
| Context, components, request flow, deployment, constraints | [Architecture](architecture.md) |
| Durable contracts and extension rules | [Design](design.md) |
| Dated design rationale and superseded decisions | [Decisions](decisions.md) |
| Upstream protocol research | [Provider notes](provider-notes.md) |
| Jev decisions, model selection, sessions, and dispatch end to end | [Smart routing](routing.md) |
| Benchmark identities, normalization, pacing, publication | [Intelligence](intelligence.md) |
| Encryption, schemas, authentication, usage persistence | [State and security](state-and-security.md) |
| Reproducible performance measurement | [Performance](performance.md) |
| Sole project roadmap/progress tracker | [Status](status.md) |
| Dated smart-routing evaluation and frozen artifacts | [Evaluation](evaluations/smart-routing/README.md) |
| Documentation conventions | [Writing documentation](documentation.md) |

Inspect code and tests before changing a contract. Update the relevant guide and
reference when public behavior changes, and status only when work state changes.
