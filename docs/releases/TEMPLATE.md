# REPLACE_ME

Copy this file to docs/releases/vMAJOR.MINOR.PATCH.md and replace the first
heading with that exact version. Complete all sections before tagging.
The historical alpha is v0.1.0; the first public beta and first published release
is v0.2.0. All v0.x releases are marked as prereleases.

## Database compatibility

REPLACE_ME: supported schema range, current schema, changes, and preservation.
For the baseline release explicitly require the one-time reset from schemas 1–3.
Future releases must retain baseline 4 and document their migration path.

## Configuration changes

REPLACE_ME: removed/new/deprecated fields and operator actions, or "None".

## Upgrade prerequisites

REPLACE_ME: stop gracefully, archive data/configuration, back up the master key
separately, select a pinned artifact, and list any additional prerequisites.

## Rollback

REPLACE_ME: restore matching stopped database/configuration/key and the previous
application version; document data loss since the backup and any limitations.
