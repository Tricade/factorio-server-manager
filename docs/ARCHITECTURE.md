# Architecture and state ownership

This document maps the current implementation for contributors. It focuses on the manager's runtime, data ownership and compatibility edges; it is not a deployment checklist. For the full test workflow see [CONTRIBUTING.md](../CONTRIBUTING.md). For exact HTTP/WebSocket route details see [API.md](API.md). Deployment layouts and operational recovery are maintained in [OPERATIONS.md](OPERATIONS.md) and [TECHNICAL-NOTES.md](../TECHNICAL-NOTES.md).

## Runtime model and startup sequence

One manager process owns one Factorio child process. Profiles are snapshots for switching a stopped runtime, not separate concurrent servers. Separate manager containers may run independently only when their network ports, manager state and Factorio data are separate.

`src/main.go` performs startup in this order:

1. `bootstrap.NewConfig` resolves defaults, environment and flags, then loads the required JSON configuration. The JSON config wins for fields it contains.
2. `factorio.ModStartUp` prepares mod-related state.
3. `factorio.NewFactorioServer` loads server configuration and process state.
4. `factorio.InitializeProfiles` validates or creates the profile manifest and migrates the pre-profile active setup into the initial profile when needed.
5. Manager-wide autostart and map-snapshot scheduler startup are initiated. Autostart is deliberately after profile initialization.
6. `api.SetupAuth` opens/migrates the SQLite account table and ensures an administrator exists.
7. `api.NewRouter` builds security headers, authenticated UI/API routes, profile-data middleware, explicit methods, admin guards, wrong-method fallbacks and the WebSocket route.
8. The HTTP server listens with header/idle timeouts. On interrupt/SIGTERM, HTTP shutdown gets up to 10 seconds and Factorio is asked to stop gracefully within the 165-second manager budget, inside the container's documented 180-second stop allowance.

The manager does not treat an HTTP request as a process supervisor for arbitrary commands. Startup is claimed synchronously before its goroutine starts so simultaneous start requests cannot overwrite the saved profile/bind request. Server state is read through snapshots guarded by the server state mutex.

## Data flow

```text
React route/view
  -> shared Axios client and resource module (ui/api/)
  -> authenticated Gorilla Mux route (src/api/)
  -> request validation + profile/lifecycle guards
  -> Factorio domain operation (src/factorio/)
  -> profile files / manager JSON / SQLite / one Factorio process
  -> JSON, text, binary response or authenticated WebSocket event
```

The UI's profile provider is presentation state. The backend manifest and locks remain authoritative. An unavailable profile response is an error state, not permission to reinterpret profile data as manager-wide data. The shared API client carries cookies, centralizes authentication-required handling and displays plain-text or JSON error messages.

The WebSocket server is in `src/api/websocket/`; the Factorio console registers a control handler from `src/factorio/server_console.go`. `server_status` is operational state for either role. `gamelog` may contain RCON output and is admin-only, and command control is re-authorized against the current account on each command. The browser restores subscriptions after reconnect but does not replay commands.

## State ownership and storage

The authoritative root for manager-owned JSON is the directory containing the configured `conf.json`. The Factorio install root and active `saves/`, `mods/`, and `config/` directories come from `--dir`/configuration and may be mounted storage in a container. These roots are intentionally separate. In production the stable manager mount is `/opt/fsm-data`; the supported Factorio persistence is either one combined `/opt/factorio` mount or all three split mounts (`saves`, `mods`, `config`), never a mix of layouts. The mount directories themselves are immovable during replacement.

| Scope | Current state/examples | Owner and compatibility notes |
| --- | --- | --- |
| Manager configuration and credentials | `conf.json`, `sqlite.db`, one-time admin credential, `factorio.auth` | Bootstrap configuration, SQLite users, session/RCON secrets and Factorio Portal auth are manager-wide. Config values can contain secrets; keep the file private. |
| Manager preferences/scheduler state | `autostart.json`, `map-snapshot-settings.json`, `mod-portal-preferences.json` | Stored next to `conf.json`; these settings apply to one manager instance. The CLI autostart flag is a first-start fallback once the UI preference is persisted. |
| Exact engine state | `runtime-state.json` plus legacy `release-channel` marker | Stores release target and exact installed version so recreation does not silently follow a rolling channel. The split mount's program tree is restored from the exact recorded version. |
| Profile library | `profiles/manifest.json`, `profiles/<id>/...` | Profile manifest currently uses schema 1. Profile files include independent `saves/`, `mods/`, `config/`, selected save, network startup settings, exact version/release target, game mode and enabled built-in feature mods. |
| Active Factorio profile | `--dir/saves`, `--dir/mods`, `--dir/config` | The active profile's data is materialized here for the one runtime. Switching snapshots current contents and stages target contents inside each filesystem. |
| Checkpoints | `checkpoints/<profile-id>/state.json` and immutable checkpoint archives/metadata | Profile-scoped; state/metadata schema is versioned. Restore creates a new playable save and retains the source checkpoint. |
| Map snapshots | `map-snapshots/<profile-id>/` and manager-wide settings JSON | Profile image/entity snapshot data is separate from manager-wide scheduler settings. Snapshot manifest/schema is versioned. |
| Mod packs | Configured `mod_pack_dir` | Reusable definitions are manager-wide; loading a pack affects the active profile only through an explicit stopped operation. |
| Account records | SQLite GORM `User` | `gorm.Model`, unique username, bcrypt-derived password hash, role and email. API responses remove the stored password value. |

The persisted `server-settings.json` and Factorio `config.ini` are in the active Factorio config area, not the manager database. The manager redacts server credentials and filesystem-location settings from viewer reads. Profile backup export scrubs server-authentication fields and maps canonical config filenames to the destination's configured paths.

### Database, schemas and compatibility

SQLite is currently the only active account database. `src/bootstrap/user.go` defines the GORM user model; `api.SetupAuth` calls `AutoMigrate` for that model. The legacy LevelDB migration exists only to convert historical user data into SQLite during config migration. It is not a general document-store abstraction and should not receive new Factorio profile state.

Most other persistent state is a file format rather than a database table. Existing version markers include:

- profile manifest schema `1` (`src/factorio/profiles.go` / `profiles_manifest.go`);
- checkpoint store and checkpoint metadata schema `1`;
- map-snapshot manifest and its exporter schema;
- the disposable mod-startup-settings evaluator manifest schema `1`;
- portable profile-backup `kind: factorio-server-control-profile-backup`, `format_version: 1` (independent of the installed manifest schema);
- manager runtime JSON with exact release target and installed version.

Loaders reject unsupported schema versions instead of silently interpreting them. Before changing a persisted structure, decide how old data is read, migrated, rolled back and backed up. Add tests for missing, valid-old, corrupt and unsupported data. Keep the on-disk profile manifest schema independent from the portable backup format; do not bump one as a substitute for a migration of the other. Opaque Factorio saves and `mod-settings.dat` are not anonymized by backups.

## Locks and lifecycle coordination

Locks protect invariants, not just individual files. Reuse the owning package's lock entry points; do not introduce a second process path or bypass these guards.

| Guard | Purpose and typical use |
| --- | --- |
| `profileDataGate` (`sync.RWMutex`) | Shared readers/mutators and server startup prevent a profile switch from swapping active data mid-operation. Profile activation/import and mount-safe directory transactions take the exclusive side. API middleware holds shared access unless that handler owns the profile operation's lock itself. |
| `profileMutex` | Serializes profile manifest read/modify/write and profile metadata refresh. Profile-management paths take it while reading or committing manifest state. |
| `serverLifecycleMutex` | Closes the check/use race among start/stop/reload and state-changing mod/config/release operations. |
| `worldGenerationLock` | Excludes concurrent previews/world creation, release installation and a server start that would use the same Factorio engine/runtime; startup-setting evaluation also reserves it. Release admission takes it while holding the lifecycle mutex and then releases that mutex before network IO. Contention is rejected (`TryLock`) rather than queued indefinitely. |
| `mapSnapshotOperationMutex` | Prevents two isolated snapshot exports from running at once and coordinates startup-settings engine evaluation. Snapshot export also takes a shared profile-data lock while reading the active profile. |
| `checkpointOperationMutex` | Serializes checkpoint creation, restore, retention and backup operations that touch checkpoint archives. Profile backup acquires profile-data exclusive, then checkpoint, then profile-manifest locks. |
| per-path file lock | Serializes operations that read/write the same save/mod file without blocking unrelated paths. |
| `stateMutex` | Protects live process state; callers use snapshot/accessor methods rather than reading mutable flags without synchronization. |

Important observed acquisition relationships: process start takes profile-data read, then lifecycle, then tries the world-generation lock and claims the process state; it releases the lifecycle mutex before launching asynchronous process work while retaining the data/world-generation guards through startup handoff/failure. Profile activation takes profile-data exclusive before the manifest mutex. Mod-startup-settings evaluation takes profile-data read, lifecycle, then tries world-generation and map-snapshot guards. Profile backup documents and tests the profile-data → checkpoint → profile order. These are call-site-specific relationships, not permission to invent a new global nesting order; inspect both directions before adding a multi-lock operation. Never hold state locks across a long process wait or network transfer.

## Failure handling and rollback invariants

- Bootstrap user migration commits one account transaction and retains its LevelDB source. An exact already migrated non-default account, or a rotated-default account proven against its private generated-credential record, can be retried without changing its password. Conflicting existing accounts are never silently replaced.
- Legacy mod packs use sibling staging and retained source/recovery files. Ambiguous interrupted state does not authorize overwriting an existing target. Preparation/initialization failures report a nonzero process exit; see OPERATIONS for recovery rather than bypassing the validation.
- Starting Factorio claims state before launching and releases reservations on every failure; failed startup resets the starting/stopping flags and broadcasts the resulting state.
- Profile activation snapshots the current profile, validates staged target data, installs the exact required binary when needed, activates contents inside mounted directories, validates built-in feature state, reloads configuration, and commits the manifest last. Any pre-commit error attempts to restore the old directory contents, runtime metadata and selected profile. If rollback itself fails, the error retains that distinction for operators.
- Upload, profile import, mod restore and mod-pack operations validate paths/ZIP structure before activation, stage beside the destination, use bounded inputs and preserve the prior state on failure. Persistent mount roots are never renamed.
- Save-mod import and map/startup-settings exporters use isolated disposable Factorio workspaces. Their generated save, mod state, temporary configuration and outputs are not copied into the playable profile.
- Snapshot/backup persistence uses temporary sibling files/directories and restrictive modes where supported. A response must not claim activation succeeded before the committed state is validated.
- Handler errors should be safe to expose. Child-process/mod output can contain secrets or arbitrary third-party text; avoid returning it from sensitive operations. Log enough context for diagnosis without credentials, raw private files or secrets.
- UI mutation completion and subsequent refresh are separate outcomes. Failed refresh leaves a warning/retry state, not a fabricated rollback or automatic replay. Error renderers accept safe strings, never arbitrary structured/private response objects.

Tests should use the injected functions and temp roots in package tests to force errors before and after the commit point. Every bug fix needs a regression that fails against the old behavior. Filesystem mount semantics and Windows replacement behavior need the appropriate Docker/Windows CI coverage; a normal unit test is not a substitute for those platforms.

## Source-domain map

| Source area | Responsibility |
| --- | --- |
| `src/main.go` | Config/bootstrap ordering, HTTP timeouts and coordinated shutdown. |
| `src/bootstrap/` | CLI/environment/config loading, generated manager secrets and user model/legacy migration. |
| `src/api/routes.go`, `routes_catalog.go`, `routes_mod_catalog.go` | Router/middleware/auth classification and focused declarative REST route catalogs. |
| `src/api/*_handler.go`, `handlers.go` | HTTP boundary validation, permissions, body/response formatting and delegation to domain packages. |
| `src/api/auth.go`, `websocket/` | Cookie session, user/admin policy, WebSocket hub and role-sensitive subscriptions/commands. |
| `src/factorio/server*.go` | One child process, status snapshots, lifecycle, settings, console, OS-specific stop behavior. |
| `src/factorio/profiles*.go` | Profile manifest, snapshots, transactional switching/management and active data layout. |
| `src/factorio/profile_backup*.go`, `backup_archive.go`, `mod_backup.go` | Portable backup validation, preview, export/import and rollback-safe restore. |
| `src/factorio/saves*.go`, `save*.go`, `world_generation*.go`, `checkpoints*.go` | Save paths/archive handling, isolated world generation and fixed checkpoints. |
| `src/factorio/mod*.go`, `mod_portal*.go`, `mod_pack_merge.go` | Installed mods, dependencies/Portal, packs, merge planning and startup settings. |
| `src/factorio/map_snapshot*.go`, `map_snapshot_exporter/` | Isolated chart export, validation, rendering and persistence; exporter state is manager-only and never returned to playable profile data. |
| `ui/App/`, `ui/api/`, `ui/tests/`, `ui/e2e/` | React views/context, shared client/resources, Node tests and synthetic browser tests. |
| `docker/`, `scripts/`, `templates/`, `.github/workflows/` | Production build, persistence entrypoint, validators, opt-in tools and CI/release automation. |

Generated frontend output under `app/` is served by the Go binary but is not the source of truth; edit `ui/` and rebuild. Do not edit generated release archives, local databases, credentials or build output as part of a source change.

## Supported external boundaries

Outbound manager traffic is limited to the documented official Factorio release, authentication and Mod Portal services. There is no product telemetry or AI service path. The production container is `linux/amd64`, matching the official headless runtime. The current map-snapshot implementation requires Factorio 2.0.61 or later because it depends on `LuaForce.get_chunk_chart`; exporter behavior still needs testing against the exact release line being changed. Tailwind's documented browser baseline for the UI is Chrome 111+, Safari 16.4+ or Firefox 128+. Arbitrary mods, Mod Portal authentication, real-world save sizes and every possible engine/storage cross-product are not all exercised by CI.

Release packaging and publication are separate, explicit tasks. Use [RELEASE.md](../RELEASE.md) only when a release is requested. A normal code change does not imply changing SemVer metadata, tags, container aliases or publishing artifacts.
