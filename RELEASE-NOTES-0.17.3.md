## Highlights

- Save-mod import now uses the installed Factorio engine to discover the save's current exact mod state instead of trusting creation-time metadata that becomes stale after upgrades or mod changes.
- Worlds created on Factorio 1.1 and later upgraded to Factorio 2.x or Space Age retain their current game mode and community-mod set during import.
- Detection runs against an isolated copy. A timeout, malformed result, missing portal release or failed download leaves the playable save, active mods and game mode unchanged.

## Added

- Added an ephemeral save-inspection workspace with a private write-data directory, an empty temporary mod directory and a bounded generated mod list.
- Added regression coverage for upgraded Space Age saves, older Factorio saves, missing community archives, disabled built-in features, malformed results, size limits and workspace cleanup.

## Changed

- The Mods page's existing **Import mods from save** action now asks Factorio's official `--sync-mods` operation for the current save state before entering the existing staged download and activation transaction.
- Required community mods remain exact-version downloads through the manager's configured Factorio Mod Portal credentials; the isolated Factorio process performs detection only.

## Fixed

- Fixed successful-looking imports that reset an upgraded Space Age save to base Factorio and imported no community mods because `level-init.dat` still described the world as originally created.
- Fixed current community mods being missed after a save had been upgraded, remodded or continued on a newer Factorio release.

## Security and privacy

- The inspection process receives only a validated temporary save copy, the installed read-only Factorio data tree and private temporary directories. It does not receive the active mod directory, playable save path, profile configuration or stored Factorio credentials.
- Generated mod metadata is size-bounded, path-validated and rejected when malformed, symlinked, non-regular or incomplete. Temporary data is removed after success or failure.
- No telemetry, AI service, additional credential store or new public endpoint is introduced.

## Compatibility and migration

- No database, profile, API or persistent-storage migration is required. Existing combined `/opt/factorio` mounts and documented split saves, mods and config mounts remain compatible.
- Existing base-game, Space Age and historical-save imports use the same UI and endpoint. The installed Factorio engine must be able to inspect the selected save with `--sync-mods`.
- This import does not change the installed Factorio release. Built-in feature modules come from the installed game, while community mods retain the exact versions recorded by the save.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.3` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. Stop the Factorio server, select an uploaded save on the Mods page and run **Import mods from save**. Verify the detected game mode and mod list before starting Factorio.

To roll back, stop the container cleanly and recreate it with the immutable `0.17.2` image. This release does not intentionally change persisted formats; restore the pre-upgrade backup if required.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- Community imports still require valid Factorio Mod Portal credentials and every exact recorded release to remain available. Failure preserves the previous active set.
- Save import detects the required game mode and mods but does not automatically install a different Factorio version. Upgrade or switch the installed game release separately when the selected save requires it.

## Verification

- `factorio-server-manager-linux-0.17.3.zip`
- `factorio-server-manager-windows-0.17.3.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.3` (`linux/amd64`)
- SemVer image labels, Git revision, provenance and SBOM match the 0.17.3 tag
- Save-import detection verified with an upgraded Space Age save containing 19 exact community mods and with a historical Factorio 1.1 save

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
