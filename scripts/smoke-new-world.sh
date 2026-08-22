#!/bin/sh

set -eu

base_url=${FSM_BASE_URL:-http://127.0.0.1}
credential_file=${FSM_CREDENTIAL_FILE:-/opt/fsm-data/initial-admin-password.txt}
test_save=${FSM_TEST_SAVE:-fsm-new-world-smoke.zip}
test_game_mode=${FSM_TEST_GAME_MODE:-}

work_dir=$(mktemp -d)
cookie_file="$work_dir/cookies.txt"
server_started=false
checkpoint_id=
restored_save=

cleanup() {
    if [ "$server_started" = true ]; then
        curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/server/stop" >/dev/null 2>&1 || true
        attempt=0
        while [ "$attempt" -lt 30 ]; do
            running=$(curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/server/status" 2>/dev/null | jq -r '.running // false') || running=true
            [ "$running" = false ] && break
            attempt=$((attempt + 1))
            sleep 1
        done
    fi
    if [ -n "$restored_save" ]; then
        curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/saves/rm/$restored_save" >/dev/null 2>&1 || true
    fi
    if [ -n "$checkpoint_id" ]; then
        curl --fail --silent --show-error --request DELETE --cookie "$cookie_file" "$base_url/api/checkpoints/$checkpoint_id" >/dev/null 2>&1 || true
    fi
    curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/saves/rm/$test_save" >/dev/null 2>&1 || true
    rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

username=$(sed -n 's/^username=//p' "$credential_file")
password=$(sed -n 's/^password=//p' "$credential_file")
if [ -z "$username" ] || [ -z "$password" ]; then
    echo "Smoke test credential is missing" >&2
    exit 1
fi

login_payload=$(jq -cn --arg username "$username" --arg password "$password" '{username:$username,password:$password}')
printf '%s' "$login_payload" | curl --fail --silent --show-error \
    --cookie-jar "$cookie_file" \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    "$base_url/api/login" >/dev/null

if [ -n "$test_game_mode" ]; then
    mode_payload=$(jq -cn --arg mode "$test_game_mode" '{mode:$mode}')
    printf '%s' "$mode_payload" | curl --fail --silent --show-error \
        --cookie "$cookie_file" \
        --header 'Content-Type: application/json' \
        --data-binary @- \
        "$base_url/api/server/game-mode" >/dev/null
fi

options=$(curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/saves/generation/options")
mode=$(printf '%s' "$options" | jq -r '.game_mode')
planets=$(printf '%s' "$options" | jq -r '[.planets[].name] | join(",")')
echo "mode=$mode"
echo "planets=$planets"

if [ -n "$test_game_mode" ] && [ "$mode" != "$test_game_mode" ]; then
    echo "Expected game mode $test_game_mode, got $mode" >&2
    exit 1
fi

if [ "$mode" = "space-age" ]; then
    [ "$planets" = "nauvis,vulcanus,gleba,fulgora,aquilo" ] || {
        echo "Unexpected Space Age planet list: $planets" >&2
        exit 1
    }
else
    [ "$planets" = "nauvis" ] || {
        echo "Unexpected base Factorio planet list: $planets" >&2
        exit 1
    }
fi

base_payload=$(jq -cn --arg name "$test_save" '{
    name: $name,
    preset: "rail-world",
    seed: 123456789,
    preview_size: 512,
    starting_area: 2,
    peaceful_mode: false,
    controls: {
        "iron-ore": {frequency: 0.5, size: 2, richness: 4},
        "water": {frequency: 0.5, size: 2}
    }
}')
if [ "$mode" = "space-age" ]; then
    base_payload=$(printf '%s' "$base_payload" | jq -c '.controls.scrap = {frequency: 2, size: 2, richness: 4}')
fi

old_ifs=$IFS
IFS=','
for planet in $planets; do
    payload=$(printf '%s' "$base_payload" | jq -c --arg planet "$planet" '.planet = $planet')
    printf '%s' "$payload" | curl --fail --silent --show-error \
        --cookie "$cookie_file" \
        --header 'Content-Type: application/json' \
        --data-binary @- \
        "$base_url/api/saves/generation/preview" >"$work_dir/$planet.png"
    signature=$(od -An -tx1 -N8 "$work_dir/$planet.png" | tr -d ' \n')
    [ "$signature" = "89504e470d0a1a0a" ] || {
        echo "Preview for $planet is not a PNG" >&2
        exit 1
    }
    bytes=$(wc -c <"$work_dir/$planet.png" | tr -d ' ')
    [ "$bytes" -gt 1024 ] || {
        echo "Preview for $planet is unexpectedly small" >&2
        exit 1
    }
    echo "preview[$planet]=$bytes bytes"
done
IFS=$old_ifs

printf '%s' "$base_payload" | curl --fail --silent --show-error \
    --cookie "$cookie_file" \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    "$base_url/api/saves/generation/create" >"$work_dir/create.json"

created_name=$(jq -r '.name' "$work_dir/create.json")
[ "$created_name" = "$test_save" ] || {
    echo "Unexpected generated save name: $created_name" >&2
    exit 1
}
echo "created=$created_name"

start_payload=$(jq -cn --arg save "$test_save" '{bindip:"0.0.0.0",port:34197,savefile:$save}')
printf '%s' "$start_payload" | curl --fail --silent --show-error \
    --cookie "$cookie_file" \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    "$base_url/api/server/start" >/dev/null
server_started=true

running=$(curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/server/status" | jq -r '.running')
[ "$running" = true ] || {
    echo "Generated save did not start" >&2
    exit 1
}
echo "start=ok"

checkpoint_state=$(curl --fail --silent --show-error --request POST --cookie "$cookie_file" "$base_url/api/checkpoints")
checkpoint_id=$(printf '%s' "$checkpoint_state" | jq -r '.checkpoints[0].id // empty')
checkpoint_trigger=$(printf '%s' "$checkpoint_state" | jq -r '.checkpoints[0].trigger // empty')
[ -n "$checkpoint_id" ] && [ "$checkpoint_trigger" = "manual" ] || {
    echo "Live checkpoint was not created" >&2
    exit 1
}
curl --fail --silent --show-error --cookie "$cookie_file" \
    "$base_url/api/checkpoints/$checkpoint_id/download" >"$work_dir/checkpoint.zip"
checkpoint_signature=$(od -An -tx1 -N4 "$work_dir/checkpoint.zip" | tr -d ' \n')
[ "$checkpoint_signature" = "504b0304" ] || {
    echo "Checkpoint is not a ZIP archive" >&2
    exit 1
}
echo "checkpoint=$checkpoint_id"

curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/server/stop" >/dev/null
attempt=0
while [ "$attempt" -lt 30 ]; do
    running=$(curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/server/status" | jq -r '.running')
    [ "$running" = false ] && break
    attempt=$((attempt + 1))
    sleep 1
done
[ "$running" = false ] || {
    echo "Generated save did not stop cleanly" >&2
    exit 1
}
server_started=false
echo "stop=ok"

restore_response=$(curl --fail --silent --show-error --request POST --cookie "$cookie_file" \
    "$base_url/api/checkpoints/$checkpoint_id/restore")
restored_save=$(printf '%s' "$restore_response" | jq -r '.name // empty')
[ -n "$restored_save" ] || {
    echo "Checkpoint restore did not return a save" >&2
    exit 1
}
checkpoint_count=$(curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/checkpoints" \
    | jq --arg id "$checkpoint_id" '[.checkpoints[] | select(.id == $id)] | length')
[ "$checkpoint_count" -eq 1 ] || {
    echo "Restore changed or removed the fixed checkpoint" >&2
    exit 1
}
echo "restore=$restored_save"

curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/saves/rm/$restored_save" >/dev/null
restored_save=
curl --fail --silent --show-error --request DELETE --cookie "$cookie_file" "$base_url/api/checkpoints/$checkpoint_id" >/dev/null
checkpoint_id=

curl --fail --silent --show-error --cookie "$cookie_file" "$base_url/api/saves/rm/$test_save" >/dev/null
echo "cleanup=ok"
