#!/usr/bin/env bash
# Isolated non-root Podman credential/benchmark smoke test. No live volumes used.
# Usage: bash scripts/test-credential-container.sh [IMAGE]
set -euo pipefail
image=${1:-localhost/torii-credential-validation:local}
task_id="torii-security-test-$(date +%s)-$$"
container="$task_id"
key_secret="$task_id-key"
wrong_secret="$task_id-wrong-key"
config_volume="$task_id-config"
state_volume="$task_id-state"
data_volume="$task_id-data"
cleanup() {
  podman rm -f "$container" "$task_id-wrong" >/dev/null 2>&1 || true
  podman volume rm "$config_volume" "$state_volume" "$data_volume" >/dev/null 2>&1 || true
  podman secret rm "$key_secret" "$wrong_secret" >/dev/null 2>&1 || true
}
trap cleanup EXIT
openssl rand -hex 32 | podman secret create "$key_secret" - >/dev/null
openssl rand -hex 32 | podman secret create "$wrong_secret" - >/dev/null
for volume in "$config_volume" "$state_volume" "$data_volume"; do podman volume create "$volume" >/dev/null; done
origin=""
launch() {
  podman run -d --name "$container" --secret "$key_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
    -e TORII_DB_KEY=invalid-env-ignored -e TORII_CONFIG_BOOTSTRAP=true -p 127.0.0.1::8080 \
    -v "$config_volume:/config:U" -v "$state_volume:/state:U" -v "$data_volume:/data" \
    "$image" serve --config /config/config.yaml >/dev/null
  origin="http://$(podman port "$container" 8080/tcp)"
  for attempt in {1..100}; do if curl -fsS "$origin/healthz" >/dev/null 2>&1; then return; fi; sleep .2; done
  printf '%s\n' 'Container did not become healthy.' >&2; exit 1
}
# Supply authorization and request bodies via a pipe, never curl command arguments.
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
[[ $(podman inspect --format '{{.Config.User}}' "$container") != root ]]
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == true ]]
[[ $(request GET /gateway/v1/config '' '' true) == 401 ]]
setup_code=$(podman logs "$container" 2>&1 | sed -n 's/.*expires in 30 minutes): \([a-f0-9]*\).*/\1/p')
[[ ${#setup_code} == 64 ]]
password='synthetic-admin-password-42'
basic="Basic $(printf 'test-admin:%s' "$password" | base64 -w0)"
setup_body=$(printf '%s' "$setup_code" | jq -Rs '{username:"test-admin",password:"synthetic-admin-password-42",setup_code:.}')
api_key=$(request POST /gateway/v1/setup '' "$setup_body" | jq -er .api_key)
[[ $api_key == torii_* ]]
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
[[ $(request GET /gateway/v1/config "Bearer $api_key" '' true) == 401 ]]
credential=$(request POST /gateway/v1/credentials "$basic" '{"service":"setup","purpose":"bearer_token","value":"synthetic-provider-secret-42"}' | jq -er .credential)
configuration=$(request GET /gateway/v1/config "$basic")
updated=$(printf '%s' "$configuration" | jq --arg id "$credential" '.providers[0].bearer_token={credential:$id}')
request PUT /gateway/v1/config "$basic" "$updated" >/dev/null
# Import a published legacy fixture without any external benchmark HTTP calls.
podman cp internal/intelligence/testdata/legacy-cache.json "$container:/state/benchmark-intelligence.json"
configuration=$(request GET /gateway/v1/config "$basic")
updated=$(printf '%s' "$configuration" | jq '.intelligence={enabled:true,sources:[{id:"smoke-cache",type:"benchlm",enabled:false}]}')
request PUT /gateway/v1/config "$basic" "$updated" >/dev/null
benchmark=$(request GET /gateway/v1/intelligence "Bearer $api_key")
[[ $(printf '%s' "$benchmark" | jq '.sources["smoke-cache"].observations | length') == 1 ]]
[[ $(printf '%s' "$benchmark" | jq -r '.sources["smoke-cache"].observations[0].external_model_id') == synthetic-benchmark-model-42 ]]
# Disabled sources remain inspectable but must not contribute routing signals.
[[ $(printf '%s' "$benchmark" | jq -r '.sources["smoke-cache"].enabled') == false ]]
[[ $(request GET /gateway/v1/intelligence/unmatched "Bearer $api_key" | jq 'length') == 0 ]]
[[ $(podman cp "$container:/state/benchmark-intelligence.json" - | tar -xOf -) == "$(<internal/intelligence/testdata/legacy-cache.json)" ]]
configuration=$(request GET /gateway/v1/config "$basic")
[[ $configuration != *synthetic-provider-secret-42* && $configuration != *"$api_key"* && $configuration != *"$password"* ]]
saved_yaml=$(podman cp "$container:/config/config.yaml" - | tar -xOf -)
app_logs=$(podman logs "$container" 2>&1)
for text in "$saved_yaml" "$app_logs"; do
  [[ $text != *synthetic-provider-secret-42* && $text != *"$api_key"* && $text != *"$password"* ]]
done
mountpoint=$(podman volume inspect --format '{{.Mountpoint}}' "$data_volume")
[[ $(podman unshare stat -c '%a' "$mountpoint/torii.db") == 600 ]]
for sensitive in "$password" "$api_key" 'synthetic-provider-secret-42'; do
  if podman unshare grep -aFq "$sensitive" "$mountpoint/torii.db" "$mountpoint/torii.db-wal"; then printf '%s\n' 'Plaintext found in database.' >&2; exit 1; fi
done
rotated=$(request POST /gateway/v1/admin/api-key "$basic" | jq -er .api_key)
[[ $(request GET /v1/models "Bearer $api_key" '' true) == 401 ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
podman rm -f "$container" >/dev/null
launch
[[ $(request GET /gateway/v1/setup | jq -r .setup_required) == false ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 200 ]]
benchmark=$(request GET /gateway/v1/intelligence "Bearer $rotated")
[[ $(printf '%s' "$benchmark" | jq '.sources["smoke-cache"].observations | length') == 1 ]]
# Prove SQLite is authoritative: a different legacy file must not be re-imported.
podman cp config.example.yaml "$container:/state/benchmark-intelligence.json"
podman rm -f "$container" >/dev/null
launch
benchmark=$(request GET /gateway/v1/intelligence "Bearer $rotated")
[[ $(printf '%s' "$benchmark" | jq -r '.sources["smoke-cache"].observations[0].version') == fixture-v1 ]]
metadata=$(request GET /gateway/v1/credentials "$basic")
[[ $metadata == *"$credential"* && $metadata != *synthetic-provider-secret-42* && $metadata != *"$rotated"* ]]
terminal_result=$(printf '%s\n' "$password" | podman exec -i "$container" /gateway admin api-key --config /config/config.yaml)
[[ $terminal_result == *'displayed only once'* ]]
terminal_key=$(printf '%s\n' "$terminal_result" | tail -n1)
[[ $terminal_key == torii_* && $terminal_key != "$rotated" ]]
[[ $(request GET /v1/models "Bearer $rotated" '' true) == 401 ]]
[[ $(request GET /v1/models "Bearer $terminal_key" '' true) == 200 ]]
podman rm -f "$container" >/dev/null
if wrong_output=$(podman run --rm --name "$task_id-wrong" --secret "$wrong_secret,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400" \
  -v "$config_volume:/config:U" -v "$state_volume:/state:U" -v "$data_volume:/data" "$image" serve --config /config/config.yaml 2>&1); then
  printf '%s\n' 'Wrong master key was accepted.' >&2; exit 1
fi
[[ $wrong_output == *'cannot decrypt credential database'* ]]
printf '%s\n' 'PASS: non-root startup, mounted key, admin setup, encrypted credentials, SQLite benchmark migration/restarts, cache authority, console/CLI rotation, and wrong-key rejection.'
