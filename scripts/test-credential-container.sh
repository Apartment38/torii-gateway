#!/usr/bin/env bash
# Disposable non-root Podman validation. Never uses deployment volumes.
# Usage: bash scripts/test-credential-container.sh [IMAGE]
set -euo pipefail
image=${1:-localhost/torii-credential-validation:local}
task_id="torii-security-test-$(date +%s)-$$"
container="$task_id"
key_secret="$task_id-key"
wrong_secret="$task_id-wrong-key"
config_volume="$task_id-config"
data_volume="$task_id-data"
backup_directory=$(mktemp -d)
cleanup() {
  podman rm -f "$container" "$task_id-wrong" >/dev/null 2>&1 || true
  podman volume rm "$config_volume" "$data_volume" >/dev/null 2>&1 || true
  podman secret rm "$key_secret" "$wrong_secret" >/dev/null 2>&1 || true
  # Only this invocation's validated temporary archive.
  if [[ -f "$backup_directory/data.tar" ]]; then podman unshare rm "$backup_directory/data.tar"; fi
  rmdir "$backup_directory"
}
trap cleanup EXIT
openssl rand -hex 32 | podman secret create "$key_secret" - >/dev/null
openssl rand -hex 32 | podman secret create "$wrong_secret" - >/dev/null
for volume in "$config_volume" "$data_volume"; do podman volume create "$volume" >/dev/null; done
origin=""
launch() {
  podman run -d --name "$container" --secret "$key_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
    -e TORII_DB_KEY=invalid-env-ignored -e TORII_API_KEY=ignored-legacy-key -e TORII_CONFIG_BOOTSTRAP=true -p 127.0.0.1::8080 \
    -v "$config_volume:/config:U" -v "$data_volume:/data" \
    "$image" serve --config /config/config.yaml >/dev/null
  origin="http://$(podman port "$container" 8080/tcp)"
  for attempt in {1..100}; do if curl -fsS "$origin/healthz" >/dev/null 2>&1; then return; fi; sleep .2; done
  printf '%s\n' 'Container did not become healthy.' >&2; exit 1
}
stop() { podman stop --time 30 "$container" >/dev/null; podman rm "$container" >/dev/null; }
# Never put credentials in curl arguments or enable shell tracing.
request() {
  local method=$1 path=$2 authorization=${3:-} body=${4:-} status_only=${5:-false}
  {
    printf 'url = "%s%s"\nrequest = "%s"\n' "$origin" "$path" "$method"
    if [[ -n $authorization ]]; then printf 'header = "Authorization: %s"\n' "$authorization"; fi
    if [[ -n $body ]]; then printf 'header = "Content-Type: application/json"\ndata = '; printf '%s' "$body" | jq -Rs .; fi
    if [[ $status_only == true ]]; then printf 'output = "/dev/null"\nwrite-out = "%%{http_code}"\n'; else printf 'fail\n'; fi
  } | curl --silent --show-error --config -
}
launch
mountpoint=$(podman volume inspect --format '{{.Mountpoint}}' "$data_volume")
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" 'PRAGMA user_version;') == 4 ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT count(*) FROM sqlite_master WHERE name='imports';") == 0 ]]
host_id=$(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT value FROM application_settings WHERE key='codex_host_id';")
[[ $host_id == urn:uuid:* ]]
[[ $(podman inspect --format '{{.Config.User}}' "$container") == nonroot:nonroot ]]
[[ $(podman inspect --format '{{range .Mounts}}{{.Destination}} {{end}}' "$container") != *'/state'* ]]
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == true ]]
[[ $(request GET /gateway/v1/config '' '' true) == 401 ]]
[[ $(request GET /v1/models 'Bearer ignored-legacy-key' '' true) == 401 ]]
setup_code=$(podman logs "$container" 2>&1 | sed -n 's/.*expires in 30 minutes): \([a-f0-9]*\).*/\1/p')
[[ ${#setup_code} == 64 ]]
password='synthetic-admin-password-42'
basic="Basic $(printf 'test-admin:%s' "$password" | base64 -w0)"
setup_body=$(printf '%s' "$setup_code" | jq -Rs '{username:"test-admin",password:"synthetic-admin-password-42",setup_code:.}')
api_key=$(request POST /gateway/v1/setup '' "$setup_body" | jq -er .api_key)
[[ $api_key == torii_* ]]
[[ $(request GET /gateway/v1/config "Bearer $api_key" '' true) == 401 ]]
credential=$(request POST /gateway/v1/credentials "$basic" '{"service":"setup","purpose":"bearer_token","value":"synthetic-provider-secret-42"}' | jq -er .credential)
configuration=$(request GET /gateway/v1/config "$basic")
updated=$(printf '%s' "$configuration" | jq --arg id "$credential" '.providers[0].bearer_token={credential:$id} | .intelligence={enabled:true,sources:[{id:"smoke-cache",type:"benchlm",enabled:false}]}')
request PUT /gateway/v1/config "$basic" "$updated" >/dev/null
# Persist an empty, disabled source through the normal refresh API (no network).
request POST /gateway/v1/intelligence/refresh "Bearer $api_key" >/dev/null
[[ $(request GET /gateway/v1/intelligence "Bearer $api_key" | jq -r '.sources["smoke-cache"].enabled') == false ]]
saved_yaml=$(podman cp "$container:/config/config.yaml" - | tar -xOf -)
app_logs=$(podman logs "$container" 2>&1)
for text in "$saved_yaml" "$app_logs"; do
  [[ $text != *synthetic-provider-secret-42* && $text != *"$api_key"* && $text != *"$password"* ]]
done
[[ $(podman unshare stat -c '%a' "$mountpoint/torii.db") == 600 ]]
for sensitive in "$password" "$api_key" 'synthetic-provider-secret-42'; do
  if podman unshare grep -aFq "$sensitive" "$mountpoint/torii.db" "$mountpoint/torii.db-wal"; then printf '%s\n' 'Plaintext found in database.' >&2; exit 1; fi
done
rotated=$(request POST /gateway/v1/admin/api-key "$basic" | jq -er .api_key)
[[ $(request GET /v1/models "Bearer $api_key" '' true) == 401 ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
stop
# Seed only this disposable stopped database to validate public counter storage.
podman unshare sqlite3 "$mountpoint/torii.db" "INSERT INTO usage_aggregates VALUES('setup/setup','7','70','35',0.25,'2026-10-02T12:00:00Z');"
podman unshare tar -C "$mountpoint" -cf "$backup_directory/data.tar" .
launch
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT value FROM application_settings WHERE key='codex_host_id';") == "$host_id" ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT requests FROM usage_aggregates WHERE model_id='setup/setup';") == 7 ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT count(*) FROM benchmark_sources WHERE source_id='smoke-cache';") == 1 ]]
metadata=$(request GET /gateway/v1/credentials "$basic")
[[ $metadata == *"$credential"* && $metadata != *synthetic-provider-secret-42* && $metadata != *"$rotated"* ]]
terminal_result=$(printf '%s\n' "$password" | podman exec -i "$container" /gateway admin api-key --config /config/config.yaml)
[[ $terminal_result == *'displayed only once'* ]]
terminal_key=$(printf '%s\n' "$terminal_result" | tail -n1)
[[ $terminal_key == torii_* && $terminal_key != "$rotated" ]]
[[ $(request GET /v1/models "Bearer $terminal_key" '' true) == 200 ]]
stop
if wrong_output=$(podman run --rm --name "$task_id-wrong" --secret "$wrong_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
  -v "$config_volume:/config:U" -v "$data_volume:/data" "$image" serve --config /config/config.yaml 2>&1); then
  printf '%s\n' 'Wrong master key was accepted.' >&2; exit 1
fi
[[ $wrong_output == *'cannot decrypt credential database'* ]]
# Restore the stopped snapshot. SQLite close has checkpointed its WAL.
podman unshare tar -C "$mountpoint" -xf "$backup_directory/data.tar"
launch
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
[[ $(request GET /v1/models "Bearer $terminal_key" '' true) == 401 ]]
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
stop
printf '%s\n' 'PASS: fresh baseline, non-root persistence, admin setup, console/CLI rotation, usage/identity/benchmark status persistence, stopped backup restore, and wrong-key rejection.'
