## Highlights

- Maintenance release combining the Sass and SVGO dependency updates.
- Existing UI, profiles, saves, mods and exact Factorio-version choices continue to work without migration.

## Added

- No new user-facing features.

## Changed

- Updated the Sass frontend compiler from 1.103.1 to 1.104.0.
- Updated the transitive SVGO build dependency from 4.0.2 to 4.1.0, together with its compatible `css-select` and `css-what` dependencies.
- Updated release metadata and the Unraid change summary. The interface and existing screenshots remain unchanged.

## Fixed

- Included upstream Sass compatibility fixes for special numeric and color values in generated CSS.
- No changes to game-server behavior are bundled in this maintenance release.

## Security and privacy

- SVGO 4.1.0 fixes `removeScripts` bypasses involving executable HTML inside SVG `foreignObject` elements and executable links: [GHSA-4vpr-x523-8j87](https://github.com/svg/svgo/security/advisories/GHSA-4vpr-x523-8j87) and [GHSA-w27v-7q3p-w38r](https://github.com/svg/svgo/security/advisories/GHSA-w27v-7q3p-w38r).
- SVGO belongs to the development dependency tree; the production manager does not expose it as a runtime SVG-processing service. This is dependency hardening, not a change to uploaded saves or mods.
- Authentication, credential handling, outbound services and privacy boundaries are unchanged. No telemetry or AI runtime is added.

## Compatibility and migration

- No database, API, environment-variable, port or persistent-storage migration is required.
- Keep `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three documented split saves/mods/config mounts. Do not combine both Factorio layouts.
- Profiles, saves, installed mods, startup settings, Space Age state, exact Factorio-version selections, checkpoints and manager users are preserved.

## Upgrade and rollback

1. Stop Factorio cleanly and back up manager data plus the selected Factorio persistence layout.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.18.1` and recreate the container with the existing mappings and 180-second stop timeout.
3. Confirm the UI footer reports 0.18.1 and that the active profile, selected save, game mode and exact Factorio version still match your setup.

To roll back, stop the container cleanly and recreate it with the immutable `0.18.0` image. Stored formats are unchanged; retain the pre-upgrade backup and restore it if operational state does not match expectations. The manager's existing internal autostart preference remains in effect.

## Known limitations

- The development dependency tree still includes `js-yaml` 4.3.1, which is flagged by [GHSA-2883-xcg3-v3hh](https://github.com/advisories/GHSA-2883-xcg3-v3hh) for excessive CPU use with malicious YAML. It is used to load local build configuration and is not shipped in the production manager; this patch remains scoped to the two merged dependency updates.
- The production image remains `linux/amd64`, matching the official Factorio headless archive.
- One Factorio game process runs per manager; this patch does not introduce concurrent profiles or change existing feature limitations.

## Verification

- `factorio-server-manager-linux-0.18.1.zip`
- `factorio-server-manager-windows-0.18.1.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.18.1` (`linux/amd64`)
- Node 24 installation, UI tests and production builds on Windows and Linux
- Go 1.26.8 tests, `go vet` and `govulncheck` reachability analysis
- Unraid template validation and production-container persistence checks
- Release archive content, executable-mode, checksum, image-label, provenance and SBOM verification

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
