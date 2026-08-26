# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Fixed
- Fixed authenticated save-mod imports failing with Factorio's `Token mismatch` error by supplying the stored Mod Portal username and user key to the isolated headless `--sync-mods` workspace; the password is not retained, process arguments remain secret-free and failed inspection still preserves the active mods and game mode.

## [0.17.3] - 2026-08-26

### Fixed
- Fixed save-mod imports for worlds created on older Factorio releases and later upgraded or remodded by deriving the current exact mod set through Factorio's isolated `--sync-mods` operation instead of stale `level-init.dat` metadata; failed detection or preparation preserves the existing mods and game mode.

## [0.17.2] - 2026-08-26

### Added
- Added structured bug-report and feature-request forms plus a pull-request template, with CI checks for required diagnostic, compatibility, security and verification fields.

### Fixed
- Fixed importing mods from Space Age saves by recognizing installed built-in feature mods and staging the complete replacement before activation, so failed downloads preserve the previous mod set and game mode.

## [0.17.1] - 2026-08-25

### Added
- Added root-level repository guidance for automated contributors covering architecture, safety invariants, verification, deployment and guarded release practices.

### Changed
- Raised the default per-file upload limit from 20 MiB to 512 MiB and exposed `FSM_MAX_UPLOAD` in the Unraid template and Docker examples so existing Factorio saves can be uploaded without custom container arguments.
- Refreshed the pinned Node 24 builder image to its current trusted upstream digest for reproducible release builds.

### Fixed
- Fixed save-upload size enforcement to allow multipart framing overhead and return/log the configured limit when an upload is too large.
- Fixed deleting, importing and activating mod sets with the split Unraid layout by replacing entries inside `/opt/factorio/mods` instead of attempting to rename the persistent mount point.
- Fixed Factorio rich-text icon placeholders appearing verbatim in factory-map surface and space-platform labels.

## [0.17.0] - 2026-08-23

### Added
- Added a manager-wide map-snapshot option that excludes space platforms from the isolated export entirely, avoiding their chart, image and building-detail output when large saves contain many platforms.
- Added explicit automatic, manual-only and fully disabled factory-map modes, retaining the last completed images when generation is disabled.
- Added an optional automatic-scheduling guard that postpones a due map render while players are online without restricting manual generation.
- Added a repository and package-level AI transparency notice covering AI-assisted development and selected visual assets while making clear that the deployed manager contains no AI model or AI-provider data flow.

### Changed
- Grouped the factory-map surface selector into planets, space platforms and modded/other surfaces while retaining player-visible platform names and legacy-snapshot fallbacks.
- Small space-platform maps now render their precise building-footprint Canvas at a bounded higher internal resolution and load it at fit-to-view zoom instead of enlarging a one-pixel-per-tile overlay.

### Fixed
- Release automation now detects hidden drafts by release ID and can safely resume a fully verified staged release after an interrupted publication.

## [0.16.0] - 2026-08-23

### Highlights
- Prepared the maintained fork for a distinct `Factorio Server Control` public identity and a safer Community Applications deployment without renaming compatibility-sensitive storage or image identifiers.
- Added a player overview and optional detailed building geometry to factory-map snapshots while retaining the existing isolated, non-destructive snapshot model.

### Added
- Added the repository profile, canonical Docker template, project icon and automated structural checks required for an Unraid Community Applications submission.
- Added a release checklist and user-oriented GitHub notes template covering highlights, behavior changes, security, compatibility, migration, upgrade and rollback.
- Added checksummed, versioned release archives, container provenance/SBOM generation and license/document inclusion in distributed artifacts.
- Added live connected-player state, snapshot-derived playtime ranking and bounded per-surface entity-detail exports for factory maps.

### Changed
- Organized every default Unraid host path below `/mnt/user/appdata/factorio-server-manager/` with dedicated `data`, `saves`, `mods` and `config` subdirectories while preserving non-overlapping container mounts.
- Renamed the maintained fork's public product identity to `Factorio Server Control`, while retaining stable repository, image, path and environment identifiers and explicit attribution to the original Factorio Server Manager project.
- Container shutdown now reserves a three-minute grace period and asks a running Factorio process to save and stop cleanly before the manager exits.
- Release and container helpers now build current frontend assets themselves, attach accurate source/version metadata and reserve `latest` plus immutable SemVer tags for the GitHub release workflow.
- Profile-aware UI state now resets across profile changes, mutating controls fail safe during unknown/running/stopping states, and map/mod/settings/user workflows expose clearer loading, empty, error and validation feedback.
- Factory maps now provide keyboard-operable zoom/pan, a focus-safe lightbox and a lazy Canvas building overlay without preventing legacy snapshots from displaying their base image.
- Modals, tabs and mobile navigation now manage focus, Escape handling, keyboard interaction and scroll locking consistently.
- Public documentation now states the manager's local-data and outbound-service privacy boundary, and README/Community Applications screenshot references use the available 16:9 captures instead of the portrait legacy controls image.

### Fixed
- Fixed direct HTTP login in the Unraid and registry examples by making the session-cookie security mode explicit, while retaining secure cookies for the HTTPS/Traefik example.
- Fixed the local Docker build context and release ZIP asset selection so the current manager binary, license, documentation, favicons and nested UI images are included.
- Fixed malformed native configuration handling so invalid JSON is rejected without being overwritten and generated secrets are stored with restrictive permissions.
- Fixed post-activation Factorio update failures so binary validation or runtime-state persistence can restore the prior program tree instead of leaving version metadata and files out of sync.
- Fixed checkpoint player counting, RCON serialization and WebSocket shutdown races; save/checkpoint archives now validate and activate atomically with bounded mod ZIP processing.

### Security
- Empty example RCON credentials now generate a random persisted localhost-only password instead of encouraging a reusable placeholder.
- The production entrypoint refuses disposable manager/game-data layouts, validates exact restored Factorio versions and stages the program tree before making its executable available.
- GitHub Actions and Docker base images are pinned to reviewed immutable revisions; the release toolchain now requires patched Go 1.25.14 plus `govulncheck`, and release images publish provenance and an SBOM.
- Stable publication now starts only from an existing verified SemVer tag, refuses existing release/image targets or moved tags, verifies archives and the immutable image before creating a hidden draft, and exposes the GitHub release only as the final mutation.
- Detailed map exports enforce authenticated access plus symlink, path, geometry, line, file and entity-count bounds before data reaches the browser.
- Replaced state-changing GET routes, made mutations and RCON-console access administrator-only by default, added a read-only viewer role, bounded login attempts/sessions and applied strict request/error and browser security-header handling.

### Compatibility and migration
- Retained the technical `factorio-server-manager` repository/image/executable namespace and default Unraid appdata root, plus `/opt/fsm-data`, `/opt/factorio`, the upstream Go module path and existing `FSM_*` environment names, so stopped original-manager data can be copied into the documented persistent mappings without a branding-only rename or conversion.
- Clarified that split-mount container recreation downloads the exact recorded official Factorio version again, while a combined `/opt/factorio` mount retains the installed program tree.

## [0.15.1] - 2026-08-22

### Added
- Added neutral Ko-fi support links to the project README and the manager sidebar.

### Changed
- Reworked the project and Docker READMEs into a public-release overview with explicit upstream attribution, the complete modern-fork feature set, current Docker/Unraid guidance and no obsolete community links.
- Refreshed the public screenshots for the current profile-aware interface, Ko-fi link and fullscreen factory-map viewer, and normalized every screenshot to a real PNG file.
- Disabled automatic Docker build-record artifact uploads in CI while keeping job summaries, preventing long-lived `.dockerbuild` clutter after every container test or publish job.
- Published the maintained fork from a clean root snapshot so historical private deployment records are not part of the public repository history.

## [0.15.0] - 2026-08-22

### Added
- Added direct wheel/button zoom, drag-to-pan and a separate fullscreen lightbox to factory-map snapshots.

### Changed
- Factory-map snapshots now use player-visible space-platform names and fit every surface to persistent player-owned construction with a safe tile margin instead of framing all charted chunks.

## [0.14.0] - 2026-08-22
### Added
- Added persistent Start, Save & stop and Force stop controls to the active-profile context bar on every server page.
- Added an operational overview with selected-save metadata, storage counts, checkpoint state, release target, game mode and network endpoint.
- Added a central Startup & network section for each profile and a manager-wide autostart section beside the existing multiplayer configuration.
- Added authenticated, profile-scoped factory-map snapshots generated from isolated save copies, including separate Space Age surfaces, manual refresh and a configurable manager-wide schedule.

### Changed
- Container autostart now restores the active profile's configured bind address, UDP port and selected save instead of unconditionally replacing the save choice with Load Latest.
- Restored exact installed-version, saved-target and built-in Space Age feature state on the Version & mode page, while keeping explanatory prose behind compact help controls.
- Game settings now expose category/value counts and readable labels without reintroducing empty configuration groups.
- Map generation leaves the playable save, active mod list, achievement state and running Factorio process untouched by loading an isolated copy with a temporary manager-owned exporter.

## [0.13.1] - 2026-08-21
### Added
- Added an active-profile flyout to the sidebar for direct profile switching and quick access to fresh-profile creation.
- Added compact, accessible help tooltips for the few field and panel descriptions that remain useful during configuration.

### Changed
- Reduced duplicated profile and server status across the overview and runtime pages, shortened interface copy and aligned native select controls with the Foundry Night theme.
- Server settings are visibly disabled while Factorio is running, with a clear stop-required notice instead of accepting edits that cannot be saved.
- Save deletion remains visible while the server is running but is disabled until Factorio has stopped.

### Fixed
- Filtered the manager's recurring localhost RCON probes from both stored and live game-log output without hiding remote RCON connections.
- Clearing the final save now also clears stale selected-save metadata from the active profile and context bar.

## [0.13.0] - 2026-08-21
### Added
- Added a persistent active-profile context to the sidebar and every runtime page, including the profile name, game mode, exact Factorio version and selected save.
- Added explicit profile-scoped and manager-wide labels so saves, checkpoints, mods, settings, users, mod packs and container autostart show which state they affect.

### Changed
- Rebuilt the complete manager interface around the responsive **Foundry Night** design system: industrial graphite surfaces, copper actions, consistent status colors, compact data panels and accessible focus states.
- Replaced the legacy sidebar with profile-aware desktop navigation, a mobile drawer and a fixed mobile action bar while preserving every existing route and manager function.
- Reworked the overview, profile library, save and checkpoint views, mod workflows, version and game-mode selection, settings, users, console and logs so profile-scoped and manager-wide state remain visually distinct.
- Navigation now opens every page at its beginning instead of retaining the scroll position of the previous view.
- Reorganized navigation into **Active profile**, **Manager** and **Reference** sections.
- The New Game wizard is now shown only when the active profile has no save. Existing profiles instead expose their save library, checkpoints and an explicit **New profile** action.
- Fresh-profile creation now leads through activation into the first-world setup, while copied profiles remain separate and stopped until explicitly activated.

### Fixed
- Server-setting changes now expose a persistent save/discard bar on desktop and mobile, so long forms can be committed without scrolling to their end and without losing edits to navigation or refresh confusion.
- The selected save, installed release channel, exact Factorio version, game mode and UI build revision remain visible without adding synthetic runtime metrics that the backend cannot verify.
- Removed the ambiguous always-visible world generator from profiles that already contain a playable save.
- The selected profile save is now preferred by the start form and marked in the save library.
- Fresh base-game profiles now store explicit disabled states for every Space Age feature mod, and profile activation reapplies the recorded built-in-mod state before Factorio can rewrite its defaults.

## [0.12.1] - 2026-08-20
### Added
- Added an internal, persistent game-server autostart control to the manager UI. Each manager instance can independently start the newest save from its active profile after its container starts.
- Added profile-specific fixed checkpoints with independent timed, last-player-left and clean-shutdown triggers, plus manual creation from the Saves page.
- Added configurable finite or unlimited checkpoint retention, authenticated downloads, deliberate deletion and non-destructive restore into a new normal save.

### Changed
- Factorio autostart is configured per manager instance rather than through the Unraid template; the preference lives with persistent manager data.
- Live checkpoints ask the running Factorio process to write a fresh save through RCON, wait for startup readiness and verify the completed ZIP before publishing it as an immutable checkpoint.

### Fixed
- Automatic checkpoint triggers coalesce within a short cooldown, and retention never removes the oldest archive until a newer replacement has been created and verified.
- Last-player checkpoints use confirmed connected-player counts instead of treating an individual disconnect log line as an empty server.

## [0.12.0] - 2026-08-20
### Added
- Added named server profiles for switching one stopped Factorio runtime between independent saves, downloaded mods, mod state, configuration, selected save, game mode and exact Factorio versions.
- Existing installations are migrated automatically and idempotently into an initial `Current setup` profile; no manual save or mod conversion is required.
- Profiles can be cloned from the active setup or created as fresh base-Factorio setups, then renamed, described, activated or deleted from the UI.

### Changed
- Profile archives and their manifest persist below manager data, while the active profile continues to use the normal saves, mods and config mounts.
- Container autostart is deferred until profile migration and persistent profile state are initialized.

### Fixed
- Profile activation stages and validates every target directory, snapshots the active setup first, rolls back data and game-version changes on failure, never renames Docker/Unraid mount points and never starts Factorio implicitly.
- Normal API operations cannot race a profile activation while active saves, mods and configuration are being replaced.

## [0.11.4] - 2026-08-20
### Added
- Added a complete New Game wizard with Factorio presets, a shared seed, optional map dimensions and starting conditions, plus per-planet resource, terrain and enemy controls.
- Map previews are rendered by the installed Factorio headless binary with the active game version, game mode and mods. Space Age exposes separate previews for Nauvis, Vulcanus, Gleba, Fulgora and Aquilo.
- Added a production-container smoke test covering preview generation, atomic save creation, startup, safe shutdown and cleanup in both base Factorio and Space Age modes.

### Changed
- Save creation now writes into a temporary directory on the saves filesystem and activates only a complete archive; newly generated worlds never start automatically.
- Preview and world generation are serialized with server startup, and live status events do not replace in-progress wizard state.
- Log pages open at the latest line and keep following new output only while the viewer remains at the bottom; scrolling up exposes an explicit jump-to-latest control without moving the user's position.

### Fixed
- Stopping Factorio before its RCON connection is ready no longer dereferences a missing console connection.
- A safe stop requested during Factorio startup is now queued until the process reports RCON readiness, preventing an early interrupt from leaving the game process stuck.
- Opening the log page before Factorio has created its first log file now shows an empty state instead of crashing the interface.
- Stale preview images retain the seed and resolution with which they were actually rendered while clearly requesting a refresh.

### Security
- React Router was upgraded to 7.18.2, clearing the dependency audit findings affecting the previous 6.30 release line.

## [0.11.3] - 2026-08-16
### Removed
- Removed the in-app game-help page and its navigation entry because bundled gameplay guidance cannot be guaranteed to match every Factorio version, expansion state, mod set and client configuration.
- Removed installed-mod shortcut discovery and its API; the manager no longer presents inferred or default key bindings as gameplay documentation.

## [0.11.2] - 2026-08-14
### Added
- The game-help page detects statically declared keyboard, alternative, controller and linked-game-control defaults from enabled installed mod archives.
- English locale labels from installed mods are used when available; dynamically generated or unassigned controls are reported as skipped rather than guessed.
- The sidebar includes a neutral `Factory radio` link to the configured Suno playlist.

### Changed
- The remaining German game-help content and navigation label are now consistently English.
- Hard-coded shortcuts and routing advice for individual mods were replaced by installed-mod discovery, keeping the bundled help neutral for public distribution.
- Public-facing repository metadata and the Unraid template now point to `Tricade/factorio-server-manager` and GHCR without deployment-specific domains or paths.

### Security
- Installed mod Lua is never executed during shortcut discovery. ZIP reads, source sizes and token counts are bounded, and computed Lua expressions are ignored.

## [0.11.1] - 2026-08-14
### Added
- Factorio can now be installed from `stable`, the current experimental release, or an explicitly pinned archive version.
- The sidebar shows the exact UI build version and source revision so stale browser assets are immediately visible.
- Mod Portal search and dependency selection are restricted to releases compatible with the active Factorio major/minor line.

### Changed
- The runtime page exposes the base Factorio/Space Age switch next to the version controls and derives every installed marker from the actual executable and persistent mod list.
- Frontend entry points are served without browser caching and carry a build-version query parameter.

### Fixed
- Container recreation restores the exact previously installed Factorio binary and selected release target instead of silently falling back to the current Stable channel.
- Version replacement refreshes only the executable metadata; it no longer recreates manager runtime state or changes stopped/running state, selected save, settings, Space Age features, credentials, mods or other persisted data.
- Official release metadata is read from Factorio's separate channel and version-archive endpoints, keeping the exact-version selector populated when their response formats differ.

## [0.11.0] - 2026-08-13
### Added
- Added an Unraid container template for the published image.
- Mod Portal installs now resolve exact-version required dependencies recursively and offer optional or recommended dependencies as unchecked choices before downloading.
- The runtime page now switches explicitly between base Factorio and Space Age, including the built-in Quality and Elevated Rails dependencies.
- The runtime page reports the actually installed Stable, Latest or custom Factorio release instead of highlighting Stable unconditionally.

### Changed
- Configuration environment variables are now uppercase and prefixed with FSM
- updated all dependencies - Thanks to @jannaahs and @knoxfighter
- removed CGO as dependency

### Fixed
- Factorio release installation now replaces files inside a mounted Factorio directory instead of trying to rename the Docker/Unraid mount point.
- Saves, mods and configuration are preserved during release replacement, with rollback on backup or activation errors.
- The production container now refuses to start without persistent manager data and either a complete Factorio mount or persistent saves/mods/config mounts instead of silently accepting disposable credentials or game data.
- The selected release channel is persisted with manager data so an Unraid recreation using split game-data mounts downloads the same channel again.

## [0.10.1] - 2021-03-09
### Fixed
- Single admin user can no longer be deleted (so there is always a user)
- fixed incompatibility to glibc 2.32 by linking dynamic on linux
- Moved from alpine to ubuntu docker image base, to prevent factorio not running correctly

## [0.10.0] - 2021-02-10
### Added
- Config files can be defined with absolute paths. - Thanks to @FoxAmes
- Support for >= 1.1.14 factorio saves - Thanks to @knoxfighter
- Setting in `info.json` to allow usage without ssl/tls - Thanks to @knoxfighter

### Changed
- Rework of the authentication, to have a bit more security. - Thanks to @knoxfighter
- Changed from leveldb to sqlite3 as backend database. - Thanks to @knoxfighter
- generate new random passwords, if no exist, or if they are "factorio". - Thanks to @knoxfighter
- Use "OpenFactorioServerManager" instead of "mroote" as go package name. - Thanks to @mroote
- Disable mods-page, while server is running - Thanks to @knoxfighter
- Renamed GO-package from `mroote` to `OpenFactorioServerManager` to match git repo - Thanks to @mroote

### Fixed
- old factorio versions depended by mods always shown as compatible - Thanks to @knoxfighter
- Crosscompilation with mingw-w64 on linux. (Broke with sqlite3) - Thanks to @knoxfighter
- Crash on async writing to websocket room array. - Thanks to @knoxfighter

## [0.9.0] - 2021-01-07
### Added
- Autostart factorio, when starting the server-manager - Thanks to @Psychomantis71

### Changed
- Complete rework of the UI - Thanks to @jannaahs
- Backend is refactored and improved - Thanks to @knoxfighter and @jannaahs
- Rework of the docker image, so it allows easy updating of factorio - Thanks to @ita-sammann

### Fixed
- Console page is now working correctly - Thanks to @jannaahs
- Mod Search fixed by new implementation, which does not rely on the search endpoint of the mod portal - Thanks to @jannaahs
- Listen on port 80, previously port 8080 was used. Can be changed with `--port <port>`
- Update version numbers in Docker containers

## [0.8.2] - 2020-01-08
Many bugfixes and a few small features in this release.
- Adds a flag for a custom glibc version, required on some distros such as CentOS
- bugfixes with file handling
- UI fixes and improvements
- CI bug fixes and build improvements
- and more bugfixes

Special thanks to @knoxfighter for all the contributions.

### Added
- Support for 0.17 server-adminlist.json
- Support for custom glibc location (RHEL/CENTOS)

### Changed
- Use bootstrap-fileinputs for savefile upload
- Login-Page uses bootstrap 4

### Fixed
- Login Page Design
- Sweetalert2 API changes
- allow_commands not misinterpreted as boolean anymore
- Fixed some filepaths on windows
- Fixed hardcoded Settings Path
- Fixed Upgrading, Removing Mods on Windows results in error

## [0.8.1] - 2019-03-01
### Fixed
- Fixed redirect, when not logged in
- Fixed login page completely white

## [0.8.0] - 2019-02-27
This release contains many bug fixes and features. Thanks to @knoxfighter @sean-callahan for the contributions!
- Fixes error in Factorio 0.17 saves
- Refactors and various bug fixes

## [0.7.5] - 2018-08-08
## Fixed
- fixes crash when mods have no basemodversion defined

## [0.7.4] - 2018-08-04
- Ability to auto download mods used in a save file courtesy @knoxfighter
- Fix bug in mod logging courtesy @c0nnex

## [0.7.3] - 2018-06-02
- Fixes fields in the settings dialog unable to be set to false. Courtesy @winadam.
- Various bugfixes in the mod settings page regarding version compatability. Courtesy @knoxfighter.

## [0.7.2] - 2018-05-02
### Fixed
- Fixes an error when searching in the mod portal.

## [0.7.1] - 2018-02-11
### Fixed
- Fixes an error in the configuration form where some fields were not editable.

## [0.7.0] - 2018-01-21
- Rewritten mods section now supporting installing mods directly from the Factorio mod portal and many other features courtesy @knoxfighter
- Various bug fixes

## [0.6.1] - 2017-12-22
- Adds the ability to specify the IP address for the Factorio game server to bind too.
- Updates the --rcon-password flag
- Small fixes

## [0.6.0] - 2017-01-25
This release adds a console feature using rcon to send commands and chat from the management interface.

## [0.5.2] - 2016-11-01
This release moves the server-settings.json config file. It will now save the file in the factorio/config directory.

## [0.5.1] - 2016-10-31
- Fixed bug where server-settings.json file is overwritten with default settings
- Started adding UI for editing the server-settings.json file

## [0.5.0] - 2016-10-11
- This release adds beta support for Windows users.
- Various updates for Factorio 0.14 are also included.

## [0.4.3] - 2016-09-15
This release has some small bug fixes in order to support Factorio server 0.14.
- Made the --latency-ms optional as it is removed in version 0.14
- Improved some error handling messages when starting the server.

## [0.4.2] - 2016-07-13
This release fixes a bug with Factorio 0.13 where the full path for save files must be specified when launching the server.

## [0.4.1] - 2016-05-15
This release fixes a bug where the UI reports an error when the Factorio Server was successfully started.

## [0.4.0] - 2016-05-15
### New features
- Abillity to create modpacks for easy distribution of mods
- Multiple users are now supported, create and delete users in the settings menu

### Features
- Allows control of the Factorio Server, starting and stopping the Factorio binary.
- Allows the management of save files, upload, download and delete saves.
- Manage installed mods, upload new ones, delete uneeded mods. Enable or disable individual mods.
- Allow viewing of the server logs and current configuration.
- Authentication for protecting against unauthorized users
- Available as a Docker container
- Abillity to create modpacks for easy distribution of mods
- Multiple users are now supported, create and delete users in the settings menu

## [0.3.1] - 2016-05-03
### Fixed
Fixes bug in #24 where Docker container cannot find conf.json file.

## [0.3.0] - 2016-05-01
### New features
- This release adds an authentication feature in Factorio Server Manager.
- Now able to be installed as a Docker container.
- Admin user credentials are configured in the conf.json file included in the release zip file.

### Features
- Allows control of the Factorio Server, starting and stopping the Factorio binary.
- Allows the management of save files, upload, download and delete saves.
- Manage installed mods, upload new ones, delete uneeded mods. Enable or disable individual mods.
- Allow viewing of the server logs and current configuration.
- Authentication for protecting against unauthorized users
- Available as a Docker container

## [0.2.0] - 2016-04-27
This release adds the ability to control the Factorio server. Allows stopping and starting of the server binary with advanced options.

### Features
- Allows control of the Factorio Server, starting and stopping the Factorio binary.
- Allows the management of save files, upload, download and delete saves.
- Manage installed mods, upload new ones, delete uneeded mods. Enable or disable individual mods.
- Allow viewing of the server logs and current configuration.

## [0.1.0] - 2016-04-25
First release of Factorio Server Manager

### Features
- Managing save files, create, download, delete saves
- Managing installed mods
- Factorio log tailing
- Factorio server configuration viewing
