#!/usr/bin/env bash
# Disposable non-root Podman validation. Never uses deployment volumes.
# Usage: bash scripts/test-credential-container.sh [IMAGE]
set -euo pipefail
umask 077
image=${1:-localhost/torii-credential-validation:local}
task_id="torii-security-test-$(date +%s)-$$"
container="$task_id"
key_secret="$task_id-key"
wrong_secret="$task_id-wrong-key"
config_volume="$task_id-config"
data_volume="$task_id-data"
backup_directory=$(mktemp -d)
cleanup() {
  if [[ -n ${fixture_pid:-} ]]; then kill "$fixture_pid" 2>/dev/null || true; wait "$fixture_pid" 2>/dev/null || true; fi
  podman rm -f "$container" "$task_id-wrong" >/dev/null 2>&1 || true
  podman volume rm "$config_volume" "$data_volume" >/dev/null 2>&1 || true
  podman secret rm "$key_secret" "$wrong_secret" >/dev/null 2>&1 || true
  # Only this invocation's validated temporary archive.
  if [[ -f "$backup_directory/data.tar" ]]; then podman unshare rm "$backup_directory/data.tar"; fi
  rm -f "$backup_directory/http-headers" "$backup_directory/config-etag" "$backup_directory/fixture-port"
  rmdir "$backup_directory"
}
trap cleanup EXIT
trap 'printf "Disposable verification failed at line %s\n" "$LINENO" >&2' ERR
openssl rand -hex 32 | podman secret create "$key_secret" - >/dev/null
openssl rand -hex 32 | podman secret create "$wrong_secret" - >/dev/null
for volume in "$config_volume" "$data_volume"; do podman volume create "$volume" >/dev/null; done
origin=""
launch() {
  podman run -d --name "$container" --secret "$key_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
    -e TORII_DB_KEY=invalid-env-ignored -e TORII_API_KEY=ignored-legacy-key --add-host=host.containers.internal:host-gateway -p 127.0.0.1::8080 \
    -v "$config_volume:/config:U" -v "$data_volume:/data" \
    "$image" serve >/dev/null
  origin="http://$(podman port "$container" 8080/tcp)"
  for attempt in {1..100}; do if curl -fsS "$origin/healthz" >/dev/null 2>&1; then return; fi; sleep .2; done
  printf '%s\n' 'Container did not become healthy.' >&2; exit 1
}
stop() { podman stop --time 30 "$container" >/dev/null; podman rm "$container" >/dev/null; }
# Never put credentials in curl arguments or enable shell tracing.
request() {
  local method=$1 path=$2 authorization=${3:-} body=${4:-} status_only=${5:-false} accept=${6:-}
  {
    printf 'url = "%s%s"\nrequest = "%s"\n' "$origin" "$path" "$method"
    if [[ $authorization == COOKIE ]]; then
      printf 'header = "Cookie: %s"\nheader = "X-CSRF-Token: %s"\nheader = "Origin: %s"\n' "$session_cookie" "$session_csrf" "$origin"
    elif [[ -n $authorization ]]; then printf 'header = "Authorization: %s"\n' "$authorization"; fi
    if [[ ( $path == /gateway/v1/config || $path == /gateway/v1/models || $path == /gateway/v1/quotas ) && ( $method == PUT || $method == POST ) && -f "$backup_directory/config-etag" ]]; then
      printf 'header = '; printf 'If-Match: %s' "$(cat "$backup_directory/config-etag")" | jq -Rs .
    fi
    if [[ -n $accept ]]; then printf 'header = "Accept: %s"\n' "$accept"; fi
    if [[ -n $body ]]; then printf 'header = "Content-Type: application/json"\ndata = '; printf '%s' "$body" | jq -Rs .; fi
    if [[ $status_only == true ]]; then printf 'output = "/dev/null"\nwrite-out = "%%{http_code}"\n'; else printf 'fail\n'; fi
  } | curl --silent --show-error --dump-header "$backup_directory/http-headers" --config -
  if [[ $path == /gateway/v1/config && $method == GET ]]; then
    awk 'tolower($1) == "etag:" { sub(/\r$/, "", $2); print $2 }' "$backup_directory/http-headers" > "$backup_directory/config-etag"
  fi
}
python3 scripts/container-usage-fixture.py "$backup_directory/fixture-port" &
fixture_pid=$!
for attempt in {1..100}; do [[ -s "$backup_directory/fixture-port" ]] && break; sleep .05; done
fixture_port=$(cat "$backup_directory/fixture-port")
launch
mountpoint=$(podman volume inspect --format '{{.Mountpoint}}' "$data_volume")
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" 'PRAGMA user_version;') == 10 ]]
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
setup_body=$(printf '%s' "$setup_code" | jq -Rs '{username:"test-admin",email:"admin@example.test",password:"synthetic-admin-password-42",setup_code:.}')
api_key=$(request POST /gateway/v1/setup '' "$setup_body" | jq -er .api_key)
[[ $api_key == torii_* ]]
[[ $(request GET /gateway/v1/config "Bearer $api_key" '' true) == 401 ]]
[[ $(request GET /gateway/v1/admin/account "$basic" | jq -r .email) == admin@example.test ]]
client_result=$(request POST /gateway/v1/admin/api-keys "$basic" '{"name":"Smoke client"}')
client_key=$(printf '%s' "$client_result" | jq -er .api_key)
client_id=$(printf '%s' "$client_result" | jq -er .key.id)
[[ $(request GET /v1/models "Bearer $client_key" '' true) == 200 ]]
[[ $(request GET /gateway/v1/admin/api-keys "Bearer $client_key" '' true) == 401 ]]
key_metadata=$(request GET /gateway/v1/admin/api-keys "$basic")
default_id=$(printf '%s' "$key_metadata" | jq -er '.api_keys[] | select(.name=="Default gateway key") | .id')
[[ $key_metadata == *'Smoke client'* && $key_metadata != *"$client_key"* ]]
configuration=$(request GET /gateway/v1/config "$basic")
[[ $(printf '%s' "$configuration" | jq '.providers | length') == 0 ]]
[[ $(request GET /v1/models "Bearer $api_key" | jq -r '[.data[].id] == ["smart"]') == true ]]
request PUT /gateway/v1/config "$basic" "$configuration" >/dev/null
configuration=$(request GET /gateway/v1/config "$basic")
credential=$(request POST /gateway/v1/credentials "$basic" '{"service":"smoke","purpose":"bearer_token","value":"synthetic-provider-secret-42"}' | jq -er .credential)
# Add a static fixture explicitly; bootstrap must not invent a provider. No
# upstream request is made because model discovery is disabled for this fixture.
updated=$(printf '%s' "$configuration" | jq --arg id "$credential" --arg port "$fixture_port" '.providers=[{id:"smoke",type:"openai_compatible",base_url:("http://host.containers.internal:"+$port),treat_models_as_free:true,discover_models:false,models:["fixture"],bearer_token:{credential:$id}}] | .intelligence={enabled:true,sources:[{id:"smoke-cache",type:"benchlm",enabled:false,api_key:{value:"synthetic-benchmark-secret-42"}}]}')
request PUT /gateway/v1/config "$basic" "$updated" >/dev/null
source_reference=$(request GET /gateway/v1/config "$basic" | jq -er '.intelligence.sources[0].api_key.credential')
[[ -n $source_reference ]]
# Persist an empty, disabled source through the normal refresh API (no network).
request POST /gateway/v1/intelligence/refresh "Bearer $api_key" >/dev/null
[[ $(request GET /gateway/v1/intelligence "Bearer $api_key" | jq -r '.sources["smoke-cache"].enabled') == false ]]
[[ $(request GET /gateway/v1/intelligence/observations "$basic" | jq -r '.total') == 0 ]]
[[ $(request GET /gateway/v1/intelligence/observations '' '' true) == 401 ]]
refresh_events=$(request POST /gateway/v1/intelligence/refresh "$basic" '' false 'text/event-stream')
[[ $refresh_events == *'event: progress'* && $refresh_events == *'"phase":"complete"'* && $refresh_events == *'event: result'* ]]
[[ $refresh_events == *'"partial_failure":false'* ]]
console_html=$(request GET /)
[[ $console_html == *'id="benchmark-progress"'* && $console_html == *'id="benchmark-table"'* && $console_html != *'id="intelligence-config"'* ]]
console_script=$(request GET /web/app.js)
[[ $console_script == *'text/event-stream'* && $console_script == *'function renderBenchmarkSources'* ]]
saved_settings=$(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT document FROM gateway_settings;")
app_logs=$(podman logs "$container" 2>&1)
for text in "$saved_settings" "$app_logs"; do
  [[ $text != *synthetic-provider-secret-42* && $text != *synthetic-benchmark-secret-42* && $text != *"$api_key"* && $text != *"$password"* ]]
done
[[ $(podman unshare stat -c '%a' "$mountpoint/torii.db") == 600 ]]
for sensitive in "$password" "$api_key" "$client_key" 'synthetic-provider-secret-42' 'synthetic-benchmark-secret-42'; do
  if podman unshare grep -aFq "$sensitive" "$mountpoint/torii.db" "$mountpoint/torii.db-wal"; then printf '%s\n' 'Plaintext found in database.' >&2; exit 1; fi
done
# Cookie mutations require CSRF and origin; sessions survive container recreation.
session_result=$(request POST /gateway/v1/session '' '{"username":"test-admin","password":"synthetic-admin-password-42"}')
session_csrf=$(printf '%s' "$session_result" | jq -er .csrf_token)
session_cookie=$(awk 'tolower($1)=="set-cookie:" {print $2}' "$backup_directory/http-headers" | cut -d';' -f1)
[[ $session_cookie == torii_admin_session=* ]]
[[ $(request GET /gateway/v1/session COOKIE | jq -r .username) == test-admin ]]
configuration=$(request GET /gateway/v1/config COOKIE)
request PUT /gateway/v1/models COOKIE '{"id":"smoke/fixture","enabled":false}' >/dev/null
[[ $(podman exec "$container" /gateway models --database /data/torii.db) == smart ]]
[[ $(request POST /v1/chat/completions "Bearer $api_key" '{"model":"smoke/fixture","messages":[],"max_tokens":8}' true) == 503 ]]
configuration=$(request GET /gateway/v1/config COOKIE)
request PUT /gateway/v1/models COOKIE '{"id":"smoke/fixture","enabled":true}' >/dev/null
configuration=$(request GET /gateway/v1/config COOKIE)
budgets=$(jq -n --arg id "$client_id" '{keys:{($id):{daily:{requests:1,tokens:1000,estimated_cost:1}}}}')
request PUT /gateway/v1/quotas COOKIE "$budgets" >/dev/null
[[ $(request POST /v1/chat/completions "Bearer $client_key" '{"model":"smoke/fixture","messages":[{"role":"user","content":"synthetic-redaction-marker"}],"max_tokens":8}' | jq .usage.total_tokens) == 5 ]]
[[ $(request POST /v1/chat/completions "Bearer $client_key" '{"model":"smoke/fixture","messages":[],"max_tokens":8}' true) == 429 ]]
stream=$(request POST /v1/chat/completions "Bearer $api_key" '{"model":"smoke/fixture","messages":[],"max_tokens":8,"stream":true}')
[[ $stream == *'[DONE]'* && $stream == *'"total_tokens": 5'* ]]
[[ $(request GET /gateway/v1/usage COOKIE | jq '[.usage[] | select(.key_id != "*" and .model == "smoke/fixture")] | length > 0') == true ]]
[[ $(request POST /v1/embeddings "Bearer $api_key" '{"model":"smoke/fixture","input":"fixture"}' | jq .usage.total_tokens) == 3 ]]
[[ $(request GET /gateway/v1/usage COOKIE | jq --arg id "$default_id" --arg day "$(date -u +%F)" '[.usage[] | select(.key_id==$id and .model=="smoke/fixture" and .period==$day)] | .[0] | .input_tokens==6 and .output_tokens==2 and .estimated_tokens==0') == true ]]

for attempt in {1..100}; do
 activity=$(request GET /gateway/v1/activity COOKIE)
 [[ $activity == *'complete'* ]] && break
 sleep .05
done
[[ $activity == *'complete'* && $activity != *synthetic-redaction-marker* ]]
rotated=$(request POST /gateway/v1/admin/api-key "$basic" | jq -er .api_key)
[[ $(request GET /v1/models "Bearer $api_key" '' true) == 401 ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
[[ $(request GET /v1/models "Bearer $client_key" '' true) == 200 ]]
stop
# Seed only this disposable stopped database to validate public counter storage.
podman unshare sqlite3 "$mountpoint/torii.db" "INSERT OR REPLACE INTO usage_aggregates VALUES('smoke/fixture','7','70','35',0.25,'2026-10-02T12:00:00Z');"
podman unshare tar -C "$mountpoint" -cf "$backup_directory/data.tar" .
launch
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
[[ $(request GET /gateway/v1/session COOKIE | jq -r .username) == test-admin ]]
[[ $(request POST /v1/chat/completions "Bearer $client_key" '{"model":"smoke/fixture","messages":[],"max_tokens":8}' true) == 429 ]]
[[ $(request GET /v1/models "Bearer $client_key" '' true) == 200 ]]
request DELETE "/gateway/v1/admin/api-keys/$client_id" "$basic" >/dev/null
[[ $(request GET /v1/models "Bearer $client_key" '' true) == 401 ]]
[[ $(request GET /gateway/v1/admin/api-keys "$basic" | jq '[.api_keys[] | select(.name=="Smoke client")] | length') == 0 ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT value FROM application_settings WHERE key='codex_host_id';") == "$host_id" ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT requests FROM usage_aggregates WHERE model_id='smoke/fixture';") == 7 ]]
[[ $(podman unshare sqlite3 "$mountpoint/torii.db" "SELECT count(*) FROM benchmark_sources WHERE source_id='smoke-cache';") == 1 ]]
metadata=$(request GET /gateway/v1/credentials "$basic")
[[ $metadata == *"$credential"* && $metadata != *synthetic-provider-secret-42* && $metadata != *"$rotated"* ]]
terminal_result=$(printf '%s\n' "$password" | podman exec -i "$container" /gateway admin api-key --database /data/torii.db)
[[ $terminal_result == *'displayed only once'* ]]
terminal_key=$(printf '%s\n' "$terminal_result" | tail -n1)
[[ $terminal_key == torii_* && $terminal_key != "$rotated" ]]
[[ $(request GET /v1/models "Bearer $terminal_key" '' true) == 200 ]]
stop
if wrong_output=$(podman run --rm --name "$task_id-wrong" --secret "$wrong_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
  -v "$config_volume:/config:U" -v "$data_volume:/data" "$image" serve 2>&1); then
  printf '%s\n' 'Wrong master key was accepted.' >&2; exit 1
fi
[[ $wrong_output == *'cannot decrypt credential database'* ]]
# Restore the stopped snapshot. SQLite close has checkpointed its WAL.
podman unshare tar -C "$mountpoint" -xf "$backup_directory/data.tar"
launch
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
[[ $(request GET /v1/models "Bearer $terminal_key" '' true) == 401 ]]
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
request DELETE /gateway/v1/session COOKIE >/dev/null
[[ $(request GET /gateway/v1/session COOKIE '' true) == 401 ]]
stop
printf '%s\n' 'PASS: fresh schema 10, non-root persistence, admin setup, named client key creation/revocation, console/CLI rotation, benchmark console assets/authenticated table/streamed refresh, usage/identity/benchmark status persistence, stopped backup restore, persistent cookie sessions, model eligibility, JSON/SSE/embedding accounting, restart-proof quotas, redacted activity, and wrong-key rejection.'
