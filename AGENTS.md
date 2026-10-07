# Agent workflow

- Follow explicit user instructions. Treat implementation and tests as the
  source of truth for current behavior; update stale docs to support the code.
- Inspect task-relevant code and tests first. Follow applicable sections of
  [Contributing](docs/development/contributing.md),
  [Design](docs/development/design.md), and
  [Documentation conventions](docs/development/documentation.md).
  Read subsystem docs only when needed for the task.
- Search before opening files and read bounded sections. Do not routinely load
  the entire docs tree, historical status records, or evaluation artifacts.
  Keep summaries and tool output concise; avoid duplicate plans and trackers.
- Make changes on a dedicated task branch in a separate Git worktree. Reuse
  that worktree when resuming; default to the current checkout as the base.
  Preserve unrelated changes and do not switch another agent's checkout.
- Run task-appropriate baseline checks in the worktree before editing; record
  pre-existing failures separately from regressions.
- Commit at logical implementation breakpoints: coherent, buildable milestones
  with relevant validation, tests, and docs. Stage only task-owned changes and
  use descriptive commit messages; do not defer all commits until the end.
- Before handoff, run the contributor guide's checks applicable to the final
  change, including relevant console, release, configuration, or container checks.
  Reuse passing results unless subsequent changes invalidate them. For docs-only
  work, check links, affected examples, and `git diff --check`.
  Report unavailable checks honestly; do not claim they passed.
- Review documentation at the end of every run. Update only affected guides,
  reference, and development explanations; keep prose concise and link instead
  of duplicating content. If no documentation update is warranted, say so briefly.
- Use [Status](docs/development/status.md) as the sole progress tracker; update
  it only for meaningful progress or unresolved work, not routine run logs.
- End with a compact handoff: branch/worktree, commits, validation, and remaining
  issues. Retain the worktree for review. Push, merge, publish, or deploy only
  when authorized. Leave subagent orchestration to the harness.
