## Highlights

- Existing saves up to 512 MiB can now be uploaded through the default Docker and Unraid configuration without adding custom container arguments.
- Mod imports, profile activation and mod-pack changes now work with the recommended split Unraid mounts because the manager replaces directory contents without moving the persistent mount point.
- Factory-map labels no longer expose Factorio rich-text icon markup in place of readable surface or space-platform names.

## Added

- Root-level contributor guidance documents the repository architecture, persistent-storage invariants, verification baseline and guarded release process for automated contributors.

## Changed

- The default per-file upload limit increased from 20 MiB to 512 MiB. `FSM_MAX_UPLOAD` is now documented in the Docker examples and exposed in the Unraid template; existing explicit overrides keep their configured value.
- The pinned Node 24 builder image was refreshed to its current trusted upstream digest for reproducible release builds.

## Fixed

- Save uploads account for multipart framing overhead while still enforcing the configured file-size limit, and rejected uploads now report and log that limit clearly.
- Mod deletion, save imports, mod-pack imports and profile activation replace entries inside the mounted mods directory instead of trying to rename the mount itself.
- Factory-map surface and space-platform labels remove supported Factorio inline-icon tags, retain visible text from formatting tags and fall back to the technical surface name when a label contains only an icon.

## Security and privacy

- No new outbound connection, telemetry or credential flow was added.
- Upload endpoints remain authenticated and bounded by the configured per-file limit. The higher default changes capacity, not authorization.

## Compatibility and migration

- No database, profile or persistent-storage migration is required. Combined `/opt/factorio` mounts and the documented split saves, mods and config mounts remain supported.
- Existing `FSM_MAX_UPLOAD` overrides are unchanged. Installations that used the former default will accept files up to 512 MiB after upgrade.
- Repository, image, executable, API and persistent-storage identifiers remain unchanged.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and `/opt/factorio` or all documented split persistent mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.1` and recreate the manager container with the existing mappings and stop timeout.
3. Verify login, an existing-save upload, the resulting mod list and a clean Factorio start and stop.

To roll back, stop the container cleanly and recreate it with the previous immutable `0.17.0` image. Restore the pre-upgrade backup if required; this release does not intentionally change persisted formats.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- Installing non-base mods from the Mod Portal still requires valid Factorio portal credentials.
- Browser labels render inline Factorio icons as plain text without the icon itself; icon-only labels use the technical surface name.

## Verification

- `factorio-server-manager-linux-0.17.1.zip`
- `factorio-server-manager-windows-0.17.1.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.1` (`linux/amd64`)
- SemVer image labels, Git revision, provenance and SBOM match the 0.17.1 tag

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
