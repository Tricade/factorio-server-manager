## Highlights

- Save-mod import can now download missing community mods during Factorio's isolated inspection without failing with `Token mismatch` when the manager is logged into the Mod Portal.
- The supplied Space Age save was exercised through the production container and normal browser workflow with Factorio 2.0.77, importing all 19 recorded community mods and the required built-in Space Age features.
- Authentication remains isolated: Factorio receives only the stored service username and user key in a private temporary workspace, while the password and process arguments remain secret-free.

## Added

- Added regression coverage for the temporary Factorio service credential, restrictive file permissions, credential-free process arguments, incomplete authentication and workspace cleanup.

## Changed

- The isolated `--sync-mods` workspace now receives a minimal `player-data.json` so the official Factorio process can authenticate downloads that are required to inspect a save accurately.
- Save-mod import now reports that Mod Portal authentication is required before starting Factorio when no complete stored service credential is available.

## Fixed

- Fixed authenticated save-mod imports failing with HTTP 403 `Token mismatch` as soon as Factorio needed to download a community mod into the empty inspection workspace.
- Failed inspection or a temporary Mod Portal error continues to preserve the active mod directory and game mode so the user can retry safely.

## Security and privacy

- The temporary `player-data.json` contains only the already stored Mod Portal service username and user key and uses mode `0600` where supported. It is removed with the complete private inspection workspace after success or failure.
- The account password is not retained or passed to Factorio, and credentials do not appear in process arguments. No additional credential store, telemetry service, AI service, public endpoint or outbound destination is introduced.

## Compatibility and migration

- No database, profile, API, environment-variable or persistent-storage migration is required. Existing combined `/opt/factorio` mounts and documented split saves, mods and config mounts remain compatible.
- Save-mod import requires a valid Factorio Mod Portal login and every exact community-mod release recorded by the save to remain available. The installed Factorio version must be able to inspect the selected save.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.4` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. Stop the Factorio server, confirm the Mod Portal login, select the save on the Mods page and run **Import mods from save**. Verify the detected game mode and mod list before starting Factorio.

To roll back, stop the container cleanly and recreate it with the immutable `0.17.3` image. This release does not intentionally change persisted formats; restore the pre-upgrade backup if required.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- Mod Portal availability and exact recorded releases remain external requirements. A temporary portal error can fail an import, but the previous active set is preserved and the operation can be retried.
- Save import detects required mods and game mode but does not automatically install another Factorio version.

## Verification

- `factorio-server-manager-linux-0.17.4.zip`
- `factorio-server-manager-windows-0.17.4.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.4` (`linux/amd64`)
- SemVer image labels, Git revision, provenance and SBOM match the 0.17.4 tag
- Real-user save import verified with Factorio 2.0.77, Space Age and 19 exact community mods after both a successful login and a rollback-safe transient portal failure

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
