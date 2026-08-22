# Docker and Unraid

The production container compiles the React UI and Go backend from this repository and installs Factorio separately into persistent storage on first start. The image supports `linux/amd64`, matching the official Factorio headless archive.

## Use a published image

Copy `docker/.env.registry.example` to `docker/.env.registry`, replace `RCON_PASS`, and set `FSM_IMAGE` to a complete image reference:

```text
FSM_IMAGE=ghcr.io/tricade/factorio-server-manager:latest
FSM_HTTP_PORT=8080
FACTORIO_PORT=34197
FACTORIO_VERSION=latest
RCON_PASS=replace-with-a-long-random-value
```

Then start it from the repository root:

```sh
docker compose --env-file docker/.env.registry -f docker/docker-compose.registry.yaml pull
docker compose --env-file docker/.env.registry -f docker/docker-compose.registry.yaml up -d
```

The GHCR package is public and does not require a registry login for pulls. Prefer an immutable SemVer tag such as `:0.15.1` for reproducible deployments; `:latest` follows the newest published release.

The example exposes the manager on TCP port `8080` and Factorio on UDP port `34197`. It serves HTTP inside the container, so use a trusted TLS reverse proxy or a private network before exposing the UI to the internet.

## Persistent storage

The production entrypoint refuses to start when manager credentials or game data would be disposable.

Always persist `/opt/fsm-data`. It contains the manager database, accounts, configuration, profiles, checkpoints, map snapshots, mod packs, release state and the internal game-server autostart preference.

Persist Factorio with exactly one of these layouts:

- Combined: mount all of `/opt/factorio`.
- Split: mount `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` separately.

Do not mount `/opt/factorio` and its three subdirectories at the same time. With either layout, container recreation restores the exact installed Factorio binary recorded under `/opt/fsm-data`; changing `FACTORIO_VERSION` later does not silently update the game.

`FSM_REQUIRE_PERSISTENT_MOUNTS=false` disables the persistence guard only for an intentionally disposable local test.

## First start and login

`FACTORIO_VERSION` selects the initial installation and accepts `latest`, `stable` or an exact three-part release such as `2.1.14`. Later version and game-mode changes are made in the UI while Factorio is stopped.

A fresh manager-data volume receives a one-time administrator credential file at `/opt/fsm-data/initial-admin-password.txt`. Change the administrator password after signing in; the bootstrap file is then removed. A force pull keeps the existing password when `/opt/fsm-data` is mounted correctly.

## Build this repository

Build a local production image without publishing it:

```powershell
.\scripts\publish-image.ps1 -Image factorio-server-manager -Tag local
```

Publishing is explicit. Log in to the selected registry and add `-Push`:

```powershell
docker login ghcr.io -u Tricade
.\scripts\publish-image.ps1 `
  -Image ghcr.io/tricade/factorio-server-manager `
  -Tag latest,0.15.1 `
  -Push
```

`docker/Dockerfile.registry` is the self-contained production build used by CI. It does not download an old upstream manager release. The `docker-compose.simple.yaml` and Traefik-based `docker-compose.yaml` files instead consume a locally generated `build/factorio-server-manager-linux.zip` through `Dockerfile-local`.

## Unraid

The canonical Community Applications template is `templates/factorio-server-manager.xml`. Once approved, install it by searching for **Factorio Server Manager** in Unraid's Apps tab. For a manual installation before approval, download the [raw template](https://raw.githubusercontent.com/Tricade/factorio-server-manager/main/templates/factorio-server-manager.xml) to:

```text
/boot/config/plugins/dockerMan/templates-user/my-factorio-server-manager.xml
```

In Unraid, choose **Docker → Add Container → User templates → Factorio-Server-Manager**. The default host paths share one parent and use clear subdirectories:

```text
/mnt/user/appdata/factorio-server-manager/data
/mnt/user/appdata/factorio-server-manager/saves
/mnt/user/appdata/factorio-server-manager/mods
/mnt/user/appdata/factorio-server-manager/config
```

The corresponding container paths stay separate, and the template deliberately omits an overlapping `/opt/factorio` mount.

Game-server autostart is configured inside **Server settings**. To run multiple Factorio servers concurrently, duplicate the container and give every copy a unique container name, Web UI host port, Factorio UDP host port, RCON password and separate storage paths. Profiles inside one manager are mutually exclusive and do not run in parallel.

## Migrate a legacy installation

Before removing an old container or pruning volumes, inspect its mount sources:

```sh
docker inspect FactorioServerManager --format '{{range .Mounts}}{{println .Type "|" .Name "|" .Source "->" .Destination}}{{end}}'
```

Old anonymous volumes may contain the manager database. The following read-only check locates candidates without printing credentials or database contents:

```sh
for volume in $(docker volume ls -q); do
  volume_path=$(docker volume inspect "$volume" --format '{{.Mountpoint}}')
  if [ -f "$volume_path/sqlite.db" ]; then
    printf 'volume=%s ' "$volume"
    stat -c 'database=%n size=%s modified=%y' "$volume_path/sqlite.db"
  fi
done
```

Stop the old container before copying the selected data into permanent appdata. Keep the original volume until login, users, profiles and settings have been verified in the replacement container.
