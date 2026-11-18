## Highlights

- Update the manager to Go 1.26.11 to fix an HTTP/1 request-body-limit race that can make an affected server panic.
- Refresh the build-only Node.js toolchain and update frontend and backend dependencies. Existing installs can upgrade without changing their data or mappings.

## Added

- CodeQL now scans dependency-only pull requests for GitHub Actions, Go, JavaScript/TypeScript and Python.

## Changed

- Update Axios, React Hook Form, React Router, Webpack, Babel, Tailwind CSS and related frontend tools; refresh Playwright for browser verification.
- Update the Go crypto, system, text and INI modules within their existing major versions.
- Use reviewed, immutable official Node, Go and Ubuntu image digests and the updated Docker registry login action. Node is not included in the final manager image.

## Fixed

- Use the corrected Go standard library consistently in the released archives, container and CI. The pinned build and test environments no longer disagree on the Go patch version.
- Keep the redirect credential-header regression covered, including arrays created in a different JavaScript realm.

## Security and privacy

- Go 1.26.11 corrects the [HTTP/1 request-body-limit race](https://pkg.go.dev/vuln/GO-2026-6859) (GO-2026-6859 / CVE-2026-97054) flagged in the older Go toolchain.
- The [Node 24.22.3 security update](https://nodejs.org/en/blog/release/v24.22.3/) includes upstream HTTP/2 and TLS fixes and applies only to the build environment.
- The Node HTTP adapter uses [`follow-redirects` 1.16.2](https://github.com/advisories/GHSA-mrwp-f3q8-3g9p) (CVE-2026-116689); Axios 1.21.1 also requires the fixed minimum. The browser UI and the Go server do not use the Node HTTP adapter.
- No new outbound services, telemetry or AI runtime are introduced.

## Compatibility and migration

- No data migration or UI redesign. Existing accounts, profiles, saves, mods, settings, game mode, exact Factorio version and manager autostart are preserved.
- Keep the same ports, environment variables, executable name and storage paths: `/opt/fsm-data` plus either a combined `/opt/factorio` mount or all three split saves, mods and config mounts. Do not combine the layouts.
- The existing browser baseline and production image architecture (Linux amd64) are unchanged.

## Upgrade and rollback

1. Save and stop Factorio cleanly. Back up `/opt/fsm-data` and the complete configured Factorio data layout together.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.21.1` or the updated `latest` alias. Recreate the container with the same mounts and 180-second stop timeout.
3. Reload the UI and confirm 0.21.1, the active profile and its Factorio version. The saved internal autostart preference still determines whether the game starts automatically.

For recovery, the immutable `0.21.0` image remains available. Restore from a verified pre-upgrade volume backup if needed. Avoid leaving the older, unpatched manager exposed; use rollback only while diagnosing a problem.

## Known limitations

- A single Factorio process runs at a time. This update does not change game-server behavior or automatically select a different game version.
- Real-engine CI covers representative base-game and Space Age worlds. It does not exhaustively test all third-party mods, historical saves or authenticated Mod Portal operations.
- Dependency checks are a point-in-time result. Module-only findings for packages not imported by the manager are distinct from findings in shipped code.

## Verification

- `factorio-server-manager-linux-0.21.1.zip`
- `factorio-server-manager-windows-0.21.1.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.21.1` (`linux/amd64`, with provenance and SBOM)
- Windows and Linux unit checks, Linux Go race tests, vet, Staticcheck, reachability-aware vulnerability scanning, CodeQL and Unraid metadata validation.
- Production UI build and browser matrix, built container, mounted-directory and persistence guards, and disposable base-game/Space Age creation, checkpoints and restart/recreation tests.
- The release workflow verifies the two archives, executable permissions, checksums and the pinned image before updating `latest`.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
