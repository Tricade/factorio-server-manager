## Highlights

- Administrators can configure the startup settings exposed by already installed and enabled mods directly from the Mods page.
- Settings remain isolated per profile and are validated by the active profile's exact Factorio engine before persistent data changes.
- Existing profile, save, mod-pack and container layouts continue to work without migration.

## Added

- Added a searchable Mod startup settings panel with localized mod and setting names, descriptions, defaults and reset controls.
- Added controls for Boolean, integer, floating-point, string, color and fixed-choice startup settings.
- Added administrator-only API endpoints for reading and updating the active profile's effective startup settings.

## Changed

- Mod packs now explicitly snapshot and restore `mod-settings.dat` together with their installed mod set.
- Save-mod imports preserve the active profile's existing startup settings byte for byte. Newly imported mods without stored values continue to use their Factorio defaults.
- The public documentation and Unraid metadata now describe profile-scoped Mod startup settings.

## Fixed

- No unrelated runtime defect fixes are bundled in this feature-focused release.

## Security and privacy

- Only administrators can read Mod startup settings because string settings can contain private operator data.
- Values are parsed with explicit resource limits, checked against their declared types and constraints, and re-evaluated by Factorio in an isolated temporary workspace before atomic activation.
- Unknown properties and the complete runtime-global and runtime-per-user branches remain unchanged. Submitted values and private child-process details are not written to client errors or manager logs.
- No telemetry, AI runtime, credential format, public endpoint or new outbound service is introduced. Evaluation uses the locally installed Factorio executable and local enabled-mod files.

## Compatibility and migration

- No database, API consumer, environment-variable, port or persistent-storage migration is required.
- Existing combined `/opt/factorio` mounts and documented split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts remain compatible.
- Existing profiles, saves, installed mods, Space Age state, exact-version targets, checkpoints, users and current `mod-settings.dat` content are preserved.
- The editor supports startup settings only; Factorio runtime-global and per-player settings remain outside this interface.

## Upgrade and rollback

1. Stop Factorio cleanly and back up `/opt/fsm-data` plus either the combined `/opt/factorio` mount or all documented split Factorio mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.18.0` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. Confirm the active profile, selected save, game mode, exact Factorio version and installed mods before opening **Mods → Mod startup settings**.

To roll back, stop the container cleanly and recreate it with the immutable `0.17.6` image. Persisted formats are unchanged; restore the pre-upgrade backup if operational state does not match expectations.

## Known limitations

- Factorio must be fully stopped before startup settings can be loaded or changed.
- Only settings declared by currently enabled mods are shown. Mods must still be installed or imported through the existing Mod Portal, archive or save workflows.
- A malformed or unsupported `mod-settings.dat` disables only the settings panel and leaves normal mod management available.
- The production image is `linux/amd64`, matching the official Factorio headless archive.
- For compatibility with the established port and mounted-directory layout, the process still runs as root inside an unprivileged container. Do not enable privileged mode or grant unnecessary host devices, capabilities or filesystem access.

## Verification

- `factorio-server-manager-linux-0.18.0.zip`
- `factorio-server-manager-windows-0.18.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.18.0` (`linux/amd64`)
- Node 24 tests and production frontend build on Windows and Linux
- Go 1.26.8 tests, `go vet` and `govulncheck`
- API authorization, stale-revision, concurrency, state-integrity and mounted-filesystem replacement tests
- Compatibility checks using official Factorio 1.1.110 and 2.0.77 Linux headless builds and Factorio 2.1.17 on Windows
- Unraid template validation, CodeQL analysis, Docker builds and production-container persistence checks
- Release archive content, executable-mode, checksum, image-label, provenance and SBOM verification

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
