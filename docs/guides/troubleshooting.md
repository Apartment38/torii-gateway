# Troubleshooting

Start with the symptom below. Keep your request ID when consulting logs, and
never post provider keys, passwords, master keys, or unused setup codes.

| Symptom | Check and remedy | Verify |
| --- | --- | --- |
| Build fails | Use Go 1.25.1+; check module-download connectivity | `go version`, then repeat the documented build |
| Startup reports missing/invalid master key | Use the original 64-hex key; inspect file path/read permissions. Explicit key-file errors do not fall back to env | Restart with the same key, then check health |
| Wrong key/corrupt encrypted records | Restore the matching key and coherent backup; do not generate a replacement for this database | Login and provider discovery succeed |
| Unsupported schema 1–3 | Archive the old installation and follow the explicit baseline reset | Fresh setup succeeds; old backup remains intact |
| Setup code expired | Restart for a new code or run `admin create` locally | Setup creates one admin and displays a key |
| Console asks for login again | Sessions persist across reloads/restarts, but expire or are revoked by sign-out/account changes; check cookie policy and trusted proxy settings | Sign in again; existing drafts are retained during reauthentication |
| Password reset mail never arrives | Verify saved recovery email and HTTPS/SMTP settings; check spam and generic delivery errors in logs | Request again after the one-minute cooldown |
| 401 on inference | Use a current gateway key, not provider/master key; check deleted/rotated keys | Authenticated `/v1/models` succeeds |
| 401 on config/credentials | These need an administrator session or Basic login, not an inference key | Sign in to console or use Basic for management |
| No models/provider connection fails | Check upstream service, URL, credential, and installed models | Provider connection test returns actual models |
| Host Ollama unavailable from container | Use host gateway alias; Ollama must bind to a reachable interface and firewall must allow it | Host `/api/tags` and Torii connection test succeed |
| Console save fails with permission error | SQLite owns settings; mount the whole writable database directory with correct non-root ownership, including WAL/SHM access | Save and re-read active config |
| Console save conflicts | Another tab or API client changed the configuration revision; preserve your draft, refresh configuration, and reconcile changes | Save against the latest revision |
| Listener/database change has no effect | Save the listener in the console and restart; check `--listen`/`TORII_LISTEN` overrides. Select database path via `--database`/`TORII_DB_PATH`, using the matching key | New listener health and original data are present |
| Smart returns no eligible model | Check verified endpoint/tools/vision/context support, paid toggle, quality floors, quota | Send a simple request, then inspect selection headers |
| Smart tool continuation returns 503 | Pin expired, reset, or model became ineligible; preserve session identity and start a new conversation | New conversation succeeds without replaying old tool IDs |
| Unsupported Responses parameter | Send full stateless context; remove `conversation`/`previous_response_id` | Stateless request succeeds |
| Unsupported provider endpoint/format | Check provider support; Anthropic rejects structured output; Codex/Anthropic do not support embeddings | Use supported endpoint/model |
| Streaming arrives all at once | Disable reverse-proxy buffering and use client streaming/no-buffer mode | SSE events arrive incrementally |
| Benchmark refresh rejected | Save enabled intelligence/sources, ensure stored default key exists, and check CLI URL | Refresh returns a summary |
| Benchmark source fails/429 | Inspect source card; check credential, published schema, deadline, and rate-limit waits | Previous cache remains; retry later succeeds |
| Benchmark score missing | Check dates, aliases, ambiguity, and declared scales; approximate matches are display-only | Inspect unmatched/observations API |
| Downloaded helper endpoint unavailable | Rebuild an older container image; use helper matching host OS/architecture | Helper download works and pairing completes |

For complete procedures, see [Installation](../installation/README.md),
[Credentials](credentials.md), [Providers](providers.md), and
[Backup and upgrade](backup-and-upgrade.md). For a lost administrator password,
use [email recovery](credentials.md#reset-a-forgotten-password) when configured.
There is no local CLI reset command; do not edit encrypted system records.
