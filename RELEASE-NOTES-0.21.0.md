## Highlights

- Keep completed uploads and other server actions distinct from a failed follow-up refresh. The UI retains the current form/tab, offers Retry and prevents writes against stale mod data.
- Preserve recoverable account, mod-pack, release and map data when migration or rollback fails, instead of cleaning away the recovery copy.
- Make the codebase easier to maintain with focused modules, a complete manual development guide and broader automated failure-path checks.

## Added

- Human development, architecture, HTTP/WebSocket and operational recovery references, including all 90 route-catalog entries and the single-server/profile ownership model.
- Regression tests for settings validation, process control, release/start races, migration failures, recovery backups, long logs and UI error/refresh behavior.
- A CI-enforced 500-line ceiling for imperative production files, pinned Staticcheck and complete Linux short-suite race coverage. These support review rather than guarantee bug-free software.

## Changed

- Separate mixed handlers, profiles, runtime lifecycle, maps, checkpoints, release transactions and startup-settings/dependency logic into cohesive modules without removing application capabilities.
- Normalize four legacy Go helper result tuples to put `error` last. This is a source-level compatibility change for Go integrations, which is why this is a minor release; installed containers and HTTP clients do not need those code changes.
- Give cold browser-page creation a separate bounded test-fixture budget. Application checks keep their 30-second budget and automatic browser-test retries remain disabled.
- Refresh official Node 24, Go 1.26.10 and Ubuntu 24.04 image digests after checking their upstream manifests. Node remains 24.22.1 and Go remains 1.26.10; this does not change the manager's dependency versions or runtime architecture.

## Fixed

- Reject oversized release/game-mode requests and null, corrupt or incorrectly shaped settings/admin lists before changing runtime state.
- Reserve engine replacement against concurrent server startup, and retain/report original program files when rollback itself fails. A new replacement refuses unresolved recovery backups instead of absorbing and deleting them.
- Migrate historical LevelDB accounts as one transaction, retain the original database, validate retries and avoid rotating an already migrated password again.
- Stage old flat mod-pack imports, preserve their source/recovery archives and refuse ambiguous interrupted states. Failed initialization exits nonzero rather than looking like a clean shutdown.
- Return Windows process-control and file-lock errors instead of terminating the manager. Force stop safely rejects a child process that has not been published yet.
- Drain oversized log records and continue reading subsequent lines. Keep log delivery ordered without spawning a goroutine for each line.
- Recover the previous map snapshot after interrupted activation, retain rollback evidence and reject invalid coordinates before conversion.
- Render structured API failures as safe text rather than crashing notifications or pages. Preserve console input after a rejected send, ignore stale checkpoint responses, disable settings reset during a save and suppress late feedback after unmount.

## Security and privacy

- Bound the affected API/log inputs and strengthen validation, rollback and credential-safe error handling. This release does not claim a new CVE fix or replace the security baseline shipped in 0.20.2.
- Keep Go 1.26.10 and the existing dependency versions, with refreshed immutable official build/runtime images. No new external service, telemetry or AI runtime is introduced.
- Dependency scanning reports no npm advisories and no reachable Go symbol/imported-package findings at verification time. The module-only deprecated OpenPGP advisory is described under limitations.
- Screenshots and fixtures use synthetic data. Private saves, live accounts, local paths and credentials are not included in release assets.

## Compatibility and migration

- No data-format migration is required for current installations. Profiles, saves, mods, startup settings, selected save, exact Factorio version, game mode and manager-wide autostart remain unchanged.
- HTTP methods/payloads, environment names, image/executable names, ports, database/profile/backup schemas and both persistence layouts remain unchanged.
- Keep `/opt/fsm-data` plus either a combined `/opt/factorio` mount or all three split saves/mods/config mounts. Do not overlap the layouts.
- External Go source consumers must update assignments for `ReadModPackRequest`, `ModPortalList`, `ModPortalModDetails` and `FactorioLogin` to the error-last ordering documented in the development guide. Check returned initialization/file-lock errors too.
- Historical/ambiguous migration and recovery states now fail closed. Retain the reported files, stop the manager and follow `docs/OPERATIONS.md`; do not delete recovery data to bypass the error. Old flat mod-pack source tracking requires same-directory hard-link support.
- The interface remains English-only, without a menu redesign. The browser baseline remains Chrome 111+, Safari 16.4+ or Firefox 128+; the production image remains `linux/amd64`.

## Upgrade and rollback

1. Save and stop Factorio cleanly. Back up `/opt/fsm-data` and the complete selected Factorio data layout together.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.21.0` or the updated `latest` alias, and recreate the container with the same mappings and 180-second stop timeout.
3. Reload the browser once, confirm 0.21.0 in the navigation footer and check the active profile, saves, mods and exact game version. The persisted internal autostart preference still determines whether the game starts.

The immutable `0.20.2` image remains available for recovery using the pre-upgrade full-volume backup. It does not contain these reliability/recovery fixes. Do not assume that an application downgrade can undo filesystem or migration work; restore a verified backup rather than deleting retained recovery files. Portable profile exports supplement, but do not replace, a full offline backup.

## Known limitations

- This is not a guarantee of perfect software or 100% coverage. The quality review documents measured coverage and remaining verification boundaries.
- Govulncheck's module-only GO-2026-5932 concerns deprecated `golang.org/x/crypto/openpgp`, which the manager does not import. The module remains required for bcrypt; no upstream fix is available for that unused package.
- One playable Factorio process runs at a time. Profiles switch a stopped runtime; map views remain isolated snapshots rather than a graphical live stream.
- Real-engine checks use representative base Factorio and Space Age versions with small disposable worlds. Arbitrary mod combinations, all historical saves, production load, physical mobile devices and authenticated Mod Portal downloads are not exhaustively exercised.
- Dependency and operating-system security information changes over time; continue monitoring upstream advisories.

## Verification

- `factorio-server-manager-linux-0.21.0.zip`
- `factorio-server-manager-windows-0.21.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.21.0` (`linux/amd64`, with provenance and SBOM)
- 78 UI/helper tests, production Webpack build and 70 browser scenarios across Chromium, Firefox, WebKit and mobile viewports.
- Go 1.26.10 short suite on Windows/Linux, full Linux short-suite race checks, vet, Staticcheck and reachability-aware vulnerability scanning.
- 18 Python tests, Unraid release-metadata/source-layout checks and CodeQL analysis.
- Production-image build, mounted-directory transactions, persistence guards and real-engine combined/split creation, checkpoint, restart and recreation scenarios.
- The guarded publication workflow verifies archive contents, executable permissions, checksums and the immutable image before advancing `latest` and publishing the GitHub release.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
