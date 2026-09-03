## Highlights

- Factorio release installation now accepts downloads and redirects only from the expected official HTTPS endpoints.
- Save, profile, world-generation and mod operations enforce local path boundaries and bounded archive processing before touching persistent data.
- The build, dependency and Ubuntu runtime baselines are refreshed, with weekly non-major dependency maintenance configured for the repository.

## Added

- Added weekly Dependabot coverage for npm, Go modules, GitHub Actions and Docker definitions. Major dependency and platform migrations remain deliberate review work.
- Added targeted regression coverage for official Factorio download URL validation and redirect restrictions.

## Changed

- Updated the release toolchain to Go 1.26.8, `golang.org/x/crypto` to 0.56.0, `testify` to 1.12.1 and the canonical `gopkg.in/ini.v1` module path.
- Updated safe non-major frontend dependencies, including Axios 1.20.0, React Router 7.18.3, React Hook Form 7.87.0, Sass 1.103.1 and Webpack 5.110.3.
- Refreshed the pinned Node and Go builder images; the production build now applies available Ubuntu package upgrades.
- Reduced the normal test workflow token to read-only repository contents.

## Fixed

- Prevented a selected release value or HTTP redirect from moving Factorio downloads away from the official Factorio HTTPS hosts and expected paths.
- Prevented oversized Factorio release and legacy mod-pack archives from being read without hard limits.
- Rejected non-local save, profile, world and mod path inputs before filesystem joins, while preserving valid existing names and mounted-directory behavior.

## Security and privacy

- This release hardens outbound Factorio downloads against untrusted destinations and hardens persistent filesystem operations against path traversal and resource-exhaustion inputs.
- No credential format, telemetry, AI service, public endpoint or outbound destination was added. Existing Mod Portal credentials remain local to the manager and official Factorio/Mod Portal traffic retains its current purpose.
- Automated CodeQL, dependency, secret and vulnerability checks reported no open repository alerts at release preparation time.

## Compatibility and migration

- No database, profile, API, environment-variable or persistent-storage migration is required.
- Existing combined `/opt/factorio` mounts and documented split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts remain compatible.
- Existing profiles, saves, mods, Space Age state, exact-version targets, checkpoints and manager users are preserved across the update.

## Upgrade and rollback

1. Stop Factorio cleanly and back up `/opt/fsm-data` plus either the combined `/opt/factorio` mount or all documented split Factorio mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.6` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. Confirm the active profile, selected save, game mode and exact Factorio version before starting the game server.

To roll back, stop the container cleanly and recreate it with the immutable `0.17.5` image. Persisted formats are unchanged; restore the pre-upgrade backup if operational state does not match expectations.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- For compatibility with the established port and mounted-directory layout, the process still runs as root inside an unprivileged container. Do not enable privileged mode or grant unnecessary host devices, capabilities or filesystem access.
- Dependency automation intentionally excludes major migrations, Go minor toolchain jumps and Ubuntu non-LTS series changes; those require separate compatibility review.

## Verification

- `factorio-server-manager-linux-0.17.6.zip`
- `factorio-server-manager-windows-0.17.6.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.6` (`linux/amd64`)
- Node 24 tests and production frontend build on Windows and Linux
- Go 1.26.8 tests, `go vet` and `govulncheck`
- Unraid template validation, CodeQL analysis and production-container persistence checks
- Release archive content, executable-mode, checksum, image-label, provenance and SBOM verification

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
