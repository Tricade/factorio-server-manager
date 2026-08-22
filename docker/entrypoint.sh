#!/bin/sh

set -eu
umask 077

require_persistent_mounts() {
    if [ "${FSM_REQUIRE_PERSISTENT_MOUNTS:-false}" != "true" ]; then
        return
    fi

    if ! mountpoint -q /opt/fsm-data; then
        echo "ERROR: /opt/fsm-data is not a persistent bind mount or named volume." >&2
        echo "Refusing to start because a container update would lose manager data and generate new credentials." >&2
        echo "Map persistent host storage to /opt/fsm-data, then recreate the container." >&2
        exit 1
    fi

    if mountpoint -q /opt/factorio; then
        return
    fi

    for persistent_path in /opt/factorio/saves /opt/factorio/mods /opt/factorio/config; do
        if ! mountpoint -q "$persistent_path"; then
            echo "ERROR: Factorio game data is not fully persistent." >&2
            echo "Map either /opt/factorio as a whole or all of /opt/factorio/saves, /opt/factorio/mods and /opt/factorio/config." >&2
            exit 1
        fi
    done
}

init_config() {
    jq_filter='.
      | .sq_lite_database_file = "/opt/fsm-data/sqlite.db"
      | .log_file = "/opt/fsm-data/factorio-server-manager.log"
      | .factorio_credentials_file = "/opt/fsm-data/factorio.auth"
      | .mod_pack_dir = "/opt/fsm-data/mod_packs"'

    if [ -n "$RCON_PASS" ]; then
        echo "Using the configured Factorio RCON password"
        jq --arg rcon "$RCON_PASS" "$jq_filter | .rcon_pass = \$rcon" \
            /opt/fsm/conf.json >/opt/fsm-data/conf.json
    else
        jq "$jq_filter" /opt/fsm/conf.json >/opt/fsm-data/conf.json
    fi

    chmod 0600 /opt/fsm-data/conf.json
}

install_game() {
	runtime_state=/opt/fsm-data/runtime-state.json
	release_target=""
	download_target=""

	if [ -f "$runtime_state" ]; then
		if ! jq -e 'type == "object"' "$runtime_state" >/dev/null 2>&1; then
			echo "ERROR: persisted runtime-state.json is invalid; refusing to guess a Factorio version." >&2
			exit 1
		fi
		release_target=$(jq -r '.release_target // empty' "$runtime_state")
		download_target=$(jq -r '.installed_version // empty' "$runtime_state")
	fi

	if [ -z "$release_target" ] && [ -f /opt/fsm-data/release-channel ]; then
		IFS= read -r release_target </opt/fsm-data/release-channel || true
	fi
	if [ -z "$release_target" ]; then
		release_target=${FACTORIO_VERSION:-}
	fi

	if ! release_target=$(normalize_release_target "$release_target"); then
		echo "ERROR: no valid Factorio release target is persisted or configured." >&2
		echo "Set FACTORIO_VERSION to stable, latest or an exact version such as 2.1.14." >&2
		exit 1
	fi

	if [ -x /opt/factorio/bin/x64/factorio ]; then
		installed_version=$(read_installed_version)
		write_runtime_state "$release_target" "$installed_version"
		echo "Using the persisted Factorio installation ${installed_version} (${release_target})"
		return
	fi

	# Container recreation must restore the exact previous binary. The rolling
	# target is retained only as UI context and for an explicit future update.
	if ! download_target=$(normalize_exact_version "$download_target"); then
		download_target=$release_target
	fi
	echo "Installing Factorio ${download_target}; selected target is ${release_target}"
	curl --fail --show-error --location --retry 3 \
		"https://www.factorio.com/get-download/${download_target}/headless/linux64" \
		--output /tmp/factorio_release.tar.xz
	tar -xJf /tmp/factorio_release.tar.xz -C /opt
	rm /tmp/factorio_release.tar.xz
	installed_version=$(read_installed_version)
	write_runtime_state "$release_target" "$installed_version"
}

normalize_exact_version() {
	value=${1:-}
	if ! printf '%s' "$value" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(\.0)?$'; then
		return 1
	fi
	printf '%s\n' "$value" | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+)\.0$/\1/'
}

normalize_release_target() {
	value=${1:-}
	case "$value" in
		stable|latest) printf '%s\n' "$value" ;;
		*) normalize_exact_version "$value" ;;
	esac
}

read_installed_version() {
	version=$(/opt/factorio/bin/x64/factorio --version | sed -n 's/^Version: \([0-9][0-9.]*\).*/\1/p' | head -n 1)
	if ! normalize_exact_version "$version"; then
		echo "ERROR: unable to read the installed Factorio version." >&2
		exit 1
	fi
}

write_runtime_state() {
	target=$1
	installed=$2
	temporary=/opt/fsm-data/.runtime-state-new
	jq -n --arg target "$target" --arg installed "$installed" \
		'{release_target: $target, installed_version: $installed}' >"$temporary"
	chmod 0600 "$temporary"
	mv "$temporary" /opt/fsm-data/runtime-state.json
	printf '%s\n' "$target" >/opt/fsm-data/.release-channel-new
	chmod 0600 /opt/fsm-data/.release-channel-new
	mv /opt/fsm-data/.release-channel-new /opt/fsm-data/release-channel
}

mkdir -p /opt/fsm-data/mod_packs /opt/factorio/saves /opt/factorio/mods /opt/factorio/config
require_persistent_mounts

if [ "$#" -gt 0 ]; then
    exec "$@"
fi

if [ ! -f /opt/fsm-data/conf.json ]; then
    init_config
fi

install_game

cd /opt/fsm
exec ./factorio-server-manager \
    --conf /opt/fsm-data/conf.json \
    --dir /opt/factorio \
    --mod-pack-dir /opt/fsm-data/mod_packs \
    --port 80

