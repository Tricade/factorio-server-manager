## Highlights

- Backup preview and import now check file sizes before uploading, using the destination manager's actual configured limit.
- Oversized profile and mod backups show an actionable message instead of a misleading control-service connection error.
- Large profile collections can be split into individual exports or smaller groups. The existing container setting remains the way to raise the upload limit.

## Added

- An administrator-only, non-cacheable upload-limit endpoint supplies the effective request and backup-file limits to the UI.

## Changed

- Backup import shows the effective upload limit and selected file size. Export includes guidance for large collections and oversized individual profiles.
- Backup preview/import stays disabled when the limit cannot be verified; a retry control reloads it. The limit is checked again before each upload.
- README, Docker documentation, technical notes, Unraid change history and the synthetic-data backup screenshot are updated.

## Fixed

- Files that exceed the destination's limit no longer begin an upload that can end in a generic "backup could not be processed" or "control service is not reachable" message (#60, #63).
- Known-length oversized backup requests are rejected before their bodies are read or written to temporary upload storage. Streamed requests remain bounded by the existing request limit.
- Interrupted backup uploads report relevant connection, reverse-proxy and temporary-disk checks without changing error messages for unrelated API requests.

## Security and privacy

- Existing authentication, administrator checks, stopped-server requirements for backup operations, archive validation and transaction rollback are unchanged.
- The new read-only endpoint exposes only two size limits and does not expose configuration secrets. Oversize checks in the browser supplement, rather than replace, server-side limits.
- No new outbound services, telemetry or AI runtime are introduced. Screenshots and test archives use synthetic data; no real saves or credentials are shipped.
- Saves and custom mod settings can still contain private data. Keep backup ZIPs private; they are not anonymized exports.

## Compatibility and migration

- No migration is required. Database/profile schemas, portable backup format 1, existing profiles, game versions, selected saves, autostart and persistent-volume paths are unchanged.
- `FSM_MAX_UPLOAD` remains configurable in MiB through the container environment, with the unchanged 512 MiB default. This release adds no global UI setting and does not automatically increase the limit.
- In Unraid, edit **Maximum Upload Size** in the container's advanced settings and apply the change. For example, `FSM_MAX_UPLOAD=2048` allows a 2 GiB request, minus the small multipart reserve for the file itself. Recreate the container with the same persistent mappings to apply an environment change.
- Keep `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts. Never configure both Factorio layouts together.

## Upgrade and rollback

1. Save and stop Factorio cleanly. Back up `/opt/fsm-data` and the selected Factorio persistence layout before updating.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.19.1` (or the updated `latest` alias) and recreate the container with its existing mappings and 180-second stop timeout.
3. Confirm the UI footer reports 0.19.1. Check the active profile, exact Factorio version and effective backup-upload limit. The existing internal autostart preference still applies.

For rollback, stop the container cleanly and use the retained immutable `0.19.0` image with the pre-upgrade full-volume backup. Stored formats are unchanged in this patch, but 0.19.0 does not provide the new pre-upload checks. Keep the backup until the update is verified. Portable profile ZIPs supplement full offline backups; they do not contain all manager-wide data.

## Known limitations

- Raising `FSM_MAX_UPLOAD` does not remove the 16 GiB backup-request cap or the separate 16 GiB expanded outer-archive cap. The UI reserves 68 KiB of the request limit for multipart metadata.
- Export size is not capped by the destination's import limit. Above that limit, export profiles individually or in smaller groups. If a single profile is still too large, omit optional saves/checkpoints or raise the destination limit within the existing caps.
- A reverse proxy can enforce a lower limit or timeout. Both managers still need sufficient temporary disk space; export can temporarily require approximately twice the selected data size.
- The reported size-related failure was reproduced with synthetic uploads, not the original large Pyanodons or 15.5 GB archives. Successful preflight does not guarantee that an archive will pass all subsequent validation.
- Backup/restore still requires a fully stopped game server. Portable format 1 supports zipped mods, up to 256 profiles and 30,000 entries; unpacked mod directories are unsupported.
- The production container remains `linux/amd64`, matching the official Factorio headless archive.

## Verification

- `factorio-server-manager-linux-0.19.1.zip`
- `factorio-server-manager-windows-0.19.1.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.19.1` (`linux/amd64`)
- Node 24 UI tests and production build; Go 1.26.8 tests, `go vet` and `govulncheck` reachability analysis.
- Regression coverage for administrator access, default/custom/capped limits, early request rejection, streamed requests, all four backup upload paths, multipart reserve, unavailable metadata and scoped network-error messages.
- Browser checks with the built UI and actual preview handler: an oversized 556 MiB file sends no upload request at the default limit; a valid synthetic profile archive previews correctly; limit-fetch failure and retry recover safely.
- Windows/Linux CI, CodeQL, Docker-mounted backup restore, production-container persistence checks, Unraid release metadata and release archive/checksum/image validation.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
