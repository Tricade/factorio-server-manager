# Technical notes

## Architecture

- React single-page interface built with Webpack and served by the Go binary
- Go backend using Gorilla Mux, SQLite/GORM, WebSocket status events and direct Factorio process control
- Persistent manager state under `/opt/fsm-data`
- Persistent Factorio saves, mods and configuration under `/opt/factorio`
- Production image built from source for `linux/amd64`

## Current fork behavior

- Server status is event-driven; background status updates do not replace open form state.
- Stable, experimental/latest and exact Factorio releases can be selected while the game server is stopped.
- The exact installed game version and selected release target survive container recreation.
- A combined Factorio mount retains the program tree; the split Unraid layout reinstalls the exact recorded official version after recreation and therefore needs download access at that point.
- Base Factorio and Space Age can be selected independently of the release channel.
- Named profiles switch the single stopped runtime between independent saves, mods, configuration, selected save, game mode and exact Factorio version.
- A persistent per-manager UI setting can start the active profile with its latest save after all persistent manager and profile state has initialized.
- Profiles own optional fixed checkpoints with manual, timed, last-player-left and clean-shutdown triggers, finite or unlimited retention, authenticated downloads and non-destructive restore.
- The authenticated application has one shared profile-context provider. Runtime pages display that context and declare whether each control mutates the active profile or manager-wide state.
- The New Game wizard is rendered only when the active profile's save API returns an empty list; profiles with saves expose their library and checkpoints without a competing creation workflow.
- New worlds are generated atomically by the installed Factorio binary. The same validated settings drive the final save and the official PNG previews for Nauvis and the enabled Space Age planets.
- Current-world map snapshots use Factorio's chart API in a disposable save/mod workspace and persist authenticated, profile-scoped PNGs without changing the playable save or mod state.
- Snapshot surfaces are grouped by their exported planet/platform identity; a manager-wide setting can skip platform surfaces before chart traversal so no platform PNG or entity-detail dataset is produced.
- Manager-wide automatic, manual-only and disabled modes are enforced before any isolated Factorio process starts; disabling generation retains existing read-only snapshot files.
- Automatic scheduling can optionally fail closed unless the live server's built-in RCON player count confirms that nobody is online; this occupancy check never blocks a manual trigger.
- The overview combines ephemeral live connected-player state with bounded player-time metadata from the most recent isolated snapshot; it does not present stale snapshot membership as current online state.
- New snapshots may persist bounded player-force entity footprints beside each surface image for a lazy browser Canvas overlay; older image-only snapshots remain valid.
- Mod Portal search, releases and dependency resolution are restricted to the active Factorio major/minor line.
- Required dependencies are selected automatically; optional and recommended dependencies remain explicit choices.
- Save-mod import asks Factorio itself for the save's current mod state in an isolated workspace instead of trusting creation-time `level-init.dat` metadata; downloads and activation remain staged and rollback-safe.
- UI version and source revision are visible in the navigation footer; the HTML entry point is cache-revalidated and uses versioned asset URLs.
- Uploads, archive paths, credentials and logs have bounded or redacted handling appropriate for an authenticated internal administration interface.
- SIGTERM stops new HTTP work and requests a clean Factorio shutdown within the container's three-minute grace period.

## Deliberate trade-offs

- The interface currently uses English only; a future multilingual implementation should use a complete message catalogue instead of mixing languages in individual views.
- The frontend does not yet have an automated component test suite, so production builds and browser smoke tests remain part of release verification.
- Dart Sass still reports the upstream loader's legacy JavaScript API warning.
- The minimized frontend bundle is larger than Webpack's default recommendation but does not affect correctness.
- The manager should be exposed through a trusted internal network, VPN or a properly secured reverse proxy.
- Profiles intentionally duplicate their archived saves, mods and configuration below manager storage. This favors transparent rollback and simple recovery over deduplicated storage.

### Current-save map rendering

The New Game previews remain map-generation previews. The Overview's factory map is a separate snapshot pipeline backed by the chart pixels already stored in a completed save:

- the selected save is used while stopped; while Factorio is running, a newer completed `_autosave` is preferred;
- the ZIP is verified and copied to an ephemeral workspace before Factorio reads it;
- only enabled mod packages plus their settings are copied or hard-linked into that workspace, and the embedded `fsm-map-exporter` control script is enabled only in the temporary `mod-list.json`;
- an isolated Factorio config redirects `write-data` and `script-output` into the workspace while the installed headless binary loads the copied save in benchmark mode;
- the exporter reads `LuaForce.get_chunk_chart` RGB565 data for every charted surface, preserves `LuaSurface.platform.name` where available and calculates tile-level view bounds from persistent player-force entities; the Go backend validates, crops and converts the selected chart pixels to PNG without executing a graphical client;
- a successful result atomically replaces `/opt/fsm-data/map-snapshots/<profile-id>`; a failed run leaves the previous snapshot available.

Nothing from the temporary workspace is copied back into the profile. The playable save and active mod list therefore retain their original bytes, the server is neither stopped nor restarted, and the manager-only exporter cannot change the profile's Steam-achievement eligibility. Snapshot images and their API routes are authenticated. Manager-wide state stores the generation mode, interval, optional empty-server guard and platform inclusion preference beside other manager data.

This is intentionally a periodic static view rather than a live remote-map protocol. It avoids installing a permanent companion mod, a graphical/X/OpenGL runtime and a Wube account in the manager. Factorio 2.0.61 is the minimum supported release because that version introduced `LuaForce.get_chunk_chart`. Chart pixels contain one RGB565 map-color pixel per tile, so the viewer can show terrain, buildings and routes rather than only generated-chunk occupancy, but it cannot recreate the graphical client's sprites or higher-detail entity rendering. The image remains limited to chart data available to the player force in the copied save; unexplored chunks and non-chart map state are not reconstructed by parsing `level.dat`.

### Save-mod import

Factorio creates `level-init.dat` with a world and does not keep that creation-time header synchronized with later game-version or mod changes. Save-mod import therefore validates and copies the selected archive into an ephemeral workspace, gives the installed Factorio executable an empty temporary mod directory plus private `write-data`, and runs the official `--sync-mods` inspection there. Headless synchronization requires Mod Portal authentication when community archives are missing, so the workspace receives a minimal `player-data.json` containing the stored service username and user key. The account password is never retained or passed to Factorio, credentials never enter process arguments, and the complete private workspace is removed after inspection. The playable save, active mods and profile configuration are never passed into that workspace.

The manager validates the bounded generated `mod-list.json`, distinguishes enabled built-in features from save-required community mods whose archives are not present in the empty workspace, and only then enters the existing staged download and activation transaction. Factorio performs detection; the manager retains authenticated Mod Portal downloads, exact-version validation and rollback. A permanently unavailable exact release or an authenticated archive whose `info.json` identifies a different mod is omitted from the staged set and returned to the interface as a structured skip reason. Authentication, rate-limit, network, server, malformed-archive, validation and activation failures still abort the transaction and leave the previous mod directory and game mode unchanged.

## Server profiles

Profiles retain a single active Factorio process rather than starting parallel game servers:

- each profile owns its selected save, exact installed Factorio release and rolling target, base Factorio/Space Age mode, enabled-mod manifest, downloaded mods and Factorio configuration;
- switching is permitted only while Factorio is fully stopped;
- the active profile is snapshotted first, the target is staged and validated, and activation is rolled back as one operation if any version, mod or file step fails;
- manager users, manager configuration, portal credentials, RCON credentials and mod packs remain global and are never duplicated into profiles;
- the first manager start with profile support migrates the complete existing, unprofiled setup into an initial `Current setup` profile automatically and idempotently; no save or mod setup has to be converted by hand;
- choosing a profile does not start it automatically; start remains a separate explicit action.

The manifest and inactive archives are stored in `/opt/fsm-data/profiles`. During activation, replacements are staged inside each active saves/mods/config filesystem and only their contents are moved; Docker or Unraid mount points themselves are never renamed. Normal API requests hold a shared data lock so they cannot read or mutate those directories halfway through a switch. If the recorded exact Factorio version differs, its official headless archive is installed before the data activation and restored on rollback if a later step fails.

This design provides Vanilla, Space Age and large-modpack switching without extra ports, duplicate manager processes or several Factorio servers competing for memory and CPU. Parallel runtimes remain intentionally out of scope.

The one-runtime boundary is per manager process, not per Docker host. Independent containers can run concurrently when every container has unique published TCP/UDP ports and completely separate manager and game-data mounts. The package-global server state, WebSocket status, RCON connection, profile locks and release operations make multiple Factorio child processes inside one manager unsupported.

### Profile-aware UI state

`ProfileProvider` loads the profile manifest once for the authenticated layout and exposes the active profile to all routed pages. Profile creation, activation, version changes, mode changes, save operations and mod refreshes explicitly refresh that shared state. The provider is presentation state only: backend locks and profile manifests remain authoritative, and an unavailable profile response is shown as an error context instead of silently treating data as global.

The sidebar and page context bar distinguish two scopes:

- **Active profile:** process controls, saves, checkpoints, downloaded mods, Factorio server settings, game settings, logs, console, executable version and Space Age mode.
- **Manager-wide:** profile library, manager users, reusable mod-pack definitions and manager-container autostart.

The save page uses the authoritative save list to choose between first-world setup and normal management. An empty active profile exposes generation and upload. A non-empty profile exposes its save library, selected-save marker, fixed checkpoints and an explicit route to create another profile. This keeps world generation out of established profiles without removing upload or checkpoint functionality.

Base-game profiles write explicit `false` entries for `elevated-rails`, `quality` and `space-age`. Factorio otherwise treats omitted built-in mods as defaults and can enable them while generating a world. Profile activation reapplies the complete recorded built-in-mod state after the profile directories are staged, preserving Factorio, Space Age and custom feature combinations independently of release changes.

### Fixed save checkpoints

Profiles own optional fixed checkpoints in addition to Factorio's rotating autosaves. These checkpoints are immutable archives with unique timestamped identities: an existing checkpoint is never reused or overwritten.

- configurable triggers are a running-time interval, the verified transition from at least one connected player to zero players, and a clean server stop; a manual action is always available;
- interval length and each event trigger can be enabled independently per profile;
- retention can keep every checkpoint or retain the newest configured number; with finite retention, the oldest checkpoint is removed only after the replacement archive has been created and verified successfully;
- live checkpoints use Factorio's own save operation through the ready RCON connection instead of copying a ZIP while Factorio may be writing it;
- a minimum cooldown coalesces overlapping automatic triggers, and persisted error state exposes failed archive writes instead of silently hiding a backup gap;
- restoring a checkpoint creates a new playable save from it, preserving the checkpoint itself unchanged.

Checkpoint state and archives are stored below `/opt/fsm-data/checkpoints/<profile-id>`, separate from active and inactive save directories. Profile switching therefore cannot move or overwrite them. Player-leave detection polls Factorio's connected-player count and reacts only to a confirmed positive-to-zero transition rather than a single disconnect log line.

## Release verification baseline

Before publishing a release, run:

```sh
npm ci
npm test
npm run build
cd src
go version # must report the patch version required by go.mod
go test ./... -short -count=1
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
cd ..
python3 scripts/validate-unraid-template.py
```

The production container test additionally verifies mounted release replacement, required persistence guards, the supported split Unraid mounts, live RCON checkpoint creation and non-destructive checkpoint restore. Validate the generated ZIP contents with `scripts/build-release.ps1` on Windows or `make gen_release` on Linux, and use the user-facing structure in `RELEASE.md` rather than publishing an autogenerated commit list as the release body.
