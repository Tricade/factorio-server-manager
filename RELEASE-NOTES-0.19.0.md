## Highlights

- Move or archive complete Factorio profiles from the UI, with a preview before import and optional saves/checkpoints.
- Restore existing **Mods → Download all** ZIPs without copying files into container volumes by hand.
- Existing installations keep their storage layout, active profile, exact game version and autostart settings. No migration is required.

## Added

- **Profiles → Export profiles** exports one, several or all profiles with exact Factorio versions, release targets, game modes, installed mods, enabled states, startup settings, server settings and checkpoint schedules. Save and checkpoint files are optional.
- **Profiles → Import backup** previews the archive and lets you select and rename profiles. Every imported profile receives a new ID and remains inactive; existing profiles are never overwritten. Activation and starting the game remain separate actions.
- **Mods → Restore backup** previews historical mod-download ZIPs and requires explicit confirmation before replacing the active profile's mods/settings. Saves and the installed Factorio version stay unchanged.

## Changed

- Unraid Community Applications now displays the change history as version headings and bullet lists. Backup features, documentation links and screenshots are updated.
- Profile-library and backup-preview screenshots use synthetic data; the README and Docker documentation explain portable migration, privacy and archive limits.
- Includes the dependency updates already reviewed and merged since 0.18.1: Autoprefixer 10.6.1, React Router 7.18.4, React Hook Form 7.88.0, Sass 1.104.1 and Webpack 5.111.0; Go `x/crypto` 0.57.0, `x/sys` 0.48.0 and `x/text` 0.42.0; pinned Node/Ubuntu image and Docker Action updates.

## Fixed

- Release publication and interrupted-release recovery now validate the parsed Unraid XML/CDATA changelog, including its leading version, date and completed change list. The old literal single-line check no longer blocks publication after the changelog-format change.

## Security and privacy

- Profile exports exclude manager accounts, sessions, Mod Portal/RCON credentials, host-specific engine paths and server-settings authentication fields. Imported profiles start with public and LAN listing disabled; review access settings before starting them.
- Saves, player lists and opaque custom mod settings can still contain player or private mod data. Keep backup ZIPs private; the exporter is not an anonymization tool.
- Backup endpoints are administrator-only and require a stopped Factorio runtime. Uploads are bounded and staged; validation rejects unsafe paths, symlinks, collisions, unsupported archive formats and missing enabled mods before activation.
- Imports preserve the current installation on validation or transaction failure. Mod restores use entry-level replacement inside the persistent mount rather than renaming the mount itself.
- No additional outbound service, telemetry or AI runtime is introduced.

## Compatibility and migration

- No database, existing profile-manifest, environment-variable, port or container-path migration is required. The portable backup format is independently versioned and does not change installed profile schema 1.
- Keep `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts. Do not combine both Factorio layouts.
- Updating the manager does not change the active profile, selected save, installed game version, game mode or global autostart preference.
- Imported profiles use destination-local network bindings and engine paths. Import does not download or start Factorio; later activation can install the profile's recorded exact official Factorio version through the existing version-switch mechanism.
- Old mod ZIPs without configuration files default to base Factorio with the archived community mods enabled. Review the preview before confirming a restore.

## Upgrade and rollback

1. Save and stop Factorio cleanly, then back up `/opt/fsm-data` and the selected Factorio persistence layout.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.19.0` (or the updated `latest` alias) and recreate the container with the existing mappings and 180-second stop timeout.
3. Confirm the UI footer reports 0.19.0 and verify the active profile, selected save, game mode and exact Factorio version. The existing internal autostart preference remains in effect.

For rollback, stop the container cleanly and use the retained immutable `0.18.1` image with the pre-upgrade full-volume backup. Existing stored formats are unchanged, but 0.18.1 has no portable-backup import UI. Keep the original backup until the upgrade and any profile migration are verified. Portable profile ZIPs supplement full offline backups; they do not contain manager accounts, global settings or reusable global mod packs.

## Known limitations

- Backup/restore requires the game server to be fully stopped; it never stops a running game automatically. One Factorio game process per manager remains the supported model.
- Portable format 1 supports zipped mods, at most 256 profiles, 30,000 archive entries and 16 GiB expanded outer-archive contents. Unpacked mod directories are not supported.
- The existing `FSM_MAX_UPLOAD` limit applies (512 MiB by default, including multipart overhead); backup uploads also have a 16 GiB endpoint cap. Large transfers may require a higher configured limit or smaller exports without saves/checkpoints.
- Both managers need sufficient temporary disk space. Export can temporarily require approximately twice the selected data size. Engine availability and mod compatibility still apply when activating or starting an imported setup.
- The production container remains `linux/amd64`, matching the official Factorio headless archive.

## Verification

- `factorio-server-manager-linux-0.19.0.zip`
- `factorio-server-manager-windows-0.19.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.19.0` (`linux/amd64`)
- Node 24 dependency installation, UI tests and production builds; Go 1.26.8 tests, `go vet` and `govulncheck` reachability analysis.
- Windows/Linux regression coverage for backup round trips, credential redaction, inactive imports, preserved existing state, invalid paths, partial-failure rollback and legacy mod archives.
- Linux race tests and Docker-mounted replacement tests, including mod-backup restore; production-container persistent-volume guards.
- Browser smoke checks of export, preview, selective import and mod restore with isolated synthetic data; no real saves or private credentials are shipped.
- Unraid release-metadata regression tests, complete PR CI and CodeQL; release archive content, executable-mode, checksum, image-label, provenance and SBOM checks.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
