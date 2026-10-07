## Highlights

- Combine saved mod packs with **Add to profile** without losing the mods, enabled states or settings already in the active profile.
- Choose whether optional and recommended Mod Portal dependencies are preselected in new download reviews.
- Include reviewed dependency and container-base updates, plus targeted build-tool security fixes without a Tailwind migration or UI redesign.

## Added

- **Mods → Mod packs → Add to profile** shows missing mods, retained mods and conflicts before applying a pack. The game server must be stopped and the user must be an administrator.
- **Server settings → Mod Portal → Preselect optional and recommended dependencies** stores a manager-wide preference. It defaults to off for both new and existing installations; every optional selection can still be changed before downloading.

## Changed

- The original pack-loading action is labelled **Replace mods** and retains its full-replacement confirmation, including pack settings. Additive loading instead preserves the active settings and ignores incoming pack settings.
- Pack additions validate the combined enabled dependency graph and reject conflicting versions or stale previews. Activation stages entries inside the existing mods filesystem and restores the previous state on failure; Docker mount points are never renamed.
- Included the reviewed Sass, Webpack, React Hook Form and transitive updates, and pinned Node, Go and Ubuntu image refreshes merged since 0.19.1.
- Updated the README, technical notes, synthetic 16:9 mod-pack screenshots and Unraid change history.

## Fixed

- Updated `fast-uri` to 3.1.8 for consistent normalization of percent-encoded hosts, `source-map-js` to 1.2.2 for malicious indexed-source-map handling, and `postcss-selector-parser` to 7.1.6 for linear-time flat-selector parsing.
- The selector-parser fix uses an npm override rather than the failing automatic Tailwind 4 upgrade. The existing Tailwind 3 build integration remains in place; production CSS is byte-identical before and after the parser change.

## Security and privacy

- The dependency patches address GHSA-hrr3-gc8f-f4qj, CVE-2026-93749 and GHSA-rj75-hqrm-r3gf in build tooling, not new server-side parsing services.
- Pack operations enforce authentication, administrator access, stopped-server guards, bounded regular files and ZIP metadata, profile/lifecycle locking, stale-review detection and transactional rollback. Settings and mod contents are not exposed in diagnostics.
- No new external services, telemetry or AI runtime are introduced. Documentation screenshots use synthetic data; no real saves or credentials are shipped.
- The existing `braces` build-only advisory GHSA-vfj7-8cjw-p6xm remains unresolved upstream. The affected build tool is not shipped in the runtime image; do not process untrusted source patterns during builds. A major styling-framework migration is not included in this release.

## Compatibility and migration

- No migration is required. Existing profiles, selected saves, exact Factorio versions, game modes, autostart behavior, database/profile formats and portable backup format 1 remain unchanged.
- Existing mod packs retain their format. The original replacement API stays available; additive loading and Mod Portal preferences use additional endpoints.
- The new preference is created only when saved and persists in manager data. It does not automatically download, enable, remove or update any mod.
- Keep `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts. Never configure both Factorio layouts together. No new environment variables or container ports are required.

## Upgrade and rollback

1. Save and stop Factorio cleanly. Back up `/opt/fsm-data` and the selected Factorio persistence layout before updating.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.20.0` or the updated `latest` alias, then recreate the container with the same persistent mappings and 180-second stop timeout.
3. Confirm the UI footer reports 0.20.0 and review the active profile, exact Factorio version and mod list. The existing internal autostart preference still applies.

For rollback, stop the container cleanly and use the retained immutable `0.19.1` image with the pre-upgrade full-volume backup. The older release lacks additive pack loading and the optional-default preference; use **Replace mods** only when replacement is intended. Keep the backup until the update is verified. Portable profile ZIPs supplement, but do not replace, full offline backups of manager data.

## Known limitations

- Additive loading accepts regular ZIP mods with one archive per mod. Unpacked directories, symlinks and ambiguous duplicate versions are rejected. Different versions of an already installed mod must be aligned or removed from the incoming pack first; this action does not upgrade existing mods.
- Dependency and Factorio-version checks use mod metadata. They cannot guarantee compatibility between arbitrary mods' Lua code or startup settings.
- Optional defaults cover the initial review's available optional/recommended dependencies and their required dependencies, not recursively discovered optional integrations. If that default combination cannot resolve, the basic review remains available with a warning and manual selection.
- Pack operations still require a fully stopped game server. Profiles do not run concurrently.
- The known build-only `braces` advisory remains as described above. The production image remains `linux/amd64`, matching the official Factorio headless server.

## Verification

- `factorio-server-manager-linux-0.20.0.zip`
- `factorio-server-manager-windows-0.20.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.20.0` (`linux/amd64`, with provenance and SBOM)
- Node 24 UI tests and production build; Go 1.26.8 tests, `go vet` and `govulncheck` reachability analysis.
- Regression coverage for preserved mods/settings/profile data, dependency and version conflicts, no-op/stale previews, rollback, mounted-directory activation, authorization, bounded requests, preference persistence, editable defaults and original replacement behavior.
- Browser checks with synthetic data for additive review/apply, conflicts, replacement confirmation, preference persistence, optional preselection/clear-all and stopped/running controls.
- Selector-parser lockfile and nested-selector regressions, plus byte-identical production CSS across the security override.
- Windows/Linux CI, targeted race tests, CodeQL, production-container persistence guards, split-mount checks, Unraid release metadata and verified release archives/checksums/image publication.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
