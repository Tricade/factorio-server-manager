## Highlights

- Factory-map generation can now be automatic, manual-only or fully disabled, with independent controls to wait for an empty server and omit space platforms from the export workload.
- Map surfaces are grouped into planets, named space platforms and other/modded surfaces, making large Space Age saves easier to navigate.
- Compact space platforms receive a bounded high-resolution Canvas layer built from exact entity footprints, so their layout remains recognizable despite Factorio's one-pixel-per-tile chart source.

## Added

- A manager-wide option to exclude space platforms before chart, image and entity-detail export.
- Automatic, manual-only and disabled map-generation modes. The most recent completed snapshot remains available when automatic generation is stopped or the feature is disabled.
- An optional scheduler guard that postpones automatic generation while players are online without blocking an administrator's manual render.

## Changed

- The surface selector groups planets, space platforms and other/modded surfaces and uses player-visible platform names when the save exposes them.
- Small platform maps render their categorized building-footprint Canvas at a bounded higher internal resolution and make it available from fit-to-view zoom.
- Public documentation, Unraid metadata and release screenshots describe the resource controls and the distinction between Factorio chart pixels and manager-rendered building footprints.

## Fixed

- Release automation can locate hidden drafts by release ID and safely resume a fully verified staged image and draft after an interrupted publication.

## Security and privacy

- No new public endpoint, credential flow or telemetry was added. Snapshot images and entity details remain authenticated and are generated from an isolated save copy.
- Omitting platforms removes their chart, image and building-detail data from the temporary export rather than merely hiding them in the browser.
- The repository now includes a prominent AI-transparency notice covering AI-assisted development and selected visual assets. The deployed manager contains no AI model and sends no runtime data to an AI provider.

## Compatibility and migration

- No migration is required. Existing installations default to automatic hourly generation with space platforms included unless their manager-wide settings are changed.
- Existing completed snapshots remain viewable. Generate a fresh snapshot to receive player-visible platform names, grouped metadata and the high-resolution compact-platform detail layer.
- Repository, image, executable, API, environment-variable and persistent-storage identifiers are unchanged, including `/opt/fsm-data`, `/opt/factorio` and the default Unraid appdata root.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and `/opt/factorio` or all three documented split game-data mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.0` and recreate the manager container with the existing persistent mappings and 180-second stop timeout.
3. Open **Server settings → Factory map generation**, review the generation mode, interval, online-player guard and platform preference, then create a new snapshot when convenient.
4. Verify login, active profile, selected save/version, map selector and a clean game-server start/stop before removing the backup.

To roll back, stop the container cleanly and recreate it with the previous immutable SemVer image. Restore the pre-upgrade manager/game-data backup if an older application does not accept newer persisted state; backward database/profile compatibility is not guaranteed.

## Known limitations

- Factory maps are periodic snapshots, not a live graphical client. Factorio exposes one chart-color pixel per tile; the detailed platform layer contains categorized prototype footprints, not in-game sprites.
- A newly generated snapshot is required before platform display names and detailed entity geometry become available. Very large entity sets remain bounded deliberately.
- The production image is `linux/amd64`, matching the official Factorio headless archive.

## Verification

- `factorio-server-manager-linux-0.17.0.zip`
- `factorio-server-manager-windows-0.17.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.0` (`linux/amd64`)
- SemVer image label, Git revision, provenance and SBOM match the 0.17.0 tag

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
