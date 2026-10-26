## Highlights

- Update the Go backend's certificate-verification security baseline to 1.26.10.
- Patch the Node/Brotli build runtime and the affected URI, serialization and redirect dependencies, with targeted regression tests.
- Keep the existing interface, game version, profiles and storage layout. No data migration is required.

## Added

- Regression coverage for encoded URI schemes and incomplete script end tags in build-tool dependencies, plus checks of every locked copy.
- Loopback regression coverage for credential-header stripping across redirects, including array-form headers and literal special characters in header names.

## Changed

- Reviewed updates include React Hook Form 7.90.0, Webpack 5.112.1 and its schema/watch dependencies, plus `golang.org/x/crypto` 0.58.0 and related Go modules.
- The Ubuntu 24.04 runtime image digests and pinned Docker registry login action are refreshed. The production image remains `linux/amd64`.
- Browser and release-toolchain CI use the official Ubuntu archive instead of a regional mirror that repeatedly stalled downloads. All tests and package-signature checks remain enabled.

## Fixed

- Build dependencies no longer accept the known encoded-scheme aliasing and incomplete script-end-tag cases covered by the new tests.
- The Axios Node redirect dependency correctly strips credential headers in the tested cross-origin cases. The production browser UI does not use that Node HTTP adapter.
- All Go module, image, test and release build paths agree on the same patched toolchain version.

## Security and privacy

- [Go 1.26.10](https://pkg.go.dev/vuln/GO-2026-6809) rejects compromised intermediate certificates through the upstream `crypto/x509` denylist (CVE-2026-111943 / GO-2026-6809).
- [Node 24.22.1](https://nodejs.org/en/blog/release/v24.22.1/) includes the Brotli decoder backport (CVE-2026-111497), DNS/TLS fixes and updated certificate trust data. Node is only a build tool; it is not installed in the final Go/Ubuntu container.
- Build-only `fast-uri` 3.1.9 and `serialize-javascript` 7.1.4 include the upstream URI parsing/normalization and script-end-tag fixes. The latter also includes its follow-up method-serialization compatibility correction.
- `follow-redirects` 1.16.1 includes GHSA-q3jh-wrc6-jgq8 and GHSA-8r9p-f939-6h3c. Its affected Node adapter is not part of the running Go server or browser UI.
- The current npm audit reports no known vulnerabilities. Go vulnerability checks report no findings in reachable code or imported packages. These are point-in-time results, not a guarantee against future advisories.
- No new external service, telemetry, AI runtime or credential-storage behavior is introduced. Test fixtures are synthetic; private saves, accounts and credentials are not shipped.

## Compatibility and migration

- No migration is required. Existing profiles, saves, mods, startup settings, selected game mode, exact Factorio version, release target and internal autostart preference remain unchanged.
- Image/executable names, `FSM_*` variables, ports and database/backup formats are unchanged.
- Keep `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split saves/mods/config mounts. Never mix the two Factorio layouts.
- The browser baseline remains Chrome 111+, Safari 16.4+ or Firefox 128+. There is no interface or menu redesign; existing screenshots remain representative.

## Upgrade and rollback

1. Save and stop Factorio cleanly. Back up `/opt/fsm-data` and the complete selected Factorio data layout.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.20.2` or the updated `latest` alias, then recreate the container with the existing mappings and 180-second stop timeout.
3. Reload the browser once and confirm 0.20.2 in the navigation footer. Check the active profile, exact Factorio version and saves; the existing internal autostart setting still applies.

The immutable `0.20.1` image remains available for rollback with the pre-upgrade full-volume backup. It lacks these security updates, so rollback is a temporary recovery measure. Do not assume that every future downgrade can read newer stored state safely. Portable profile exports supplement rather than replace a full offline backup.

## Known limitations

- Go's module-level advisory GO-2026-5932 concerns the unused `golang.org/x/crypto/openpgp` package. The manager does not import that package, and it is not included in the built executable.
- One playable Factorio instance runs at a time. Profile switching still requires a stopped server, and map views remain isolated periodic snapshots rather than a graphical live stream.
- Automated real-engine checks cover representative base Factorio and Space Age versions with small generated worlds, not every historical save or third-party mod combination. Authenticated Mod Portal downloads remain a separate integration check.
- Dependency and OS-package security data changes over time. Keep monitoring upstream advisories after this release.

## Verification

- `factorio-server-manager-linux-0.20.2.zip`
- `factorio-server-manager-windows-0.20.2.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.20.2` (`linux/amd64`, with provenance and SBOM)
- 55 frontend tests, production Webpack build and 60 browser scenarios across Chromium, Firefox, WebKit and mobile layouts.
- Go 1.26.10 short tests, `go vet`, `govulncheck`, targeted race checks and Windows/Linux CI; 13 Python repository tests and Unraid release-metadata validation.
- CodeQL, production-image builds, mounted-directory transaction checks and disposable real-engine combined/split persistence scenarios.
- The guarded publication workflow verifies both platform archives, documentation, executable permissions, checksums and the immutable image before advancing `latest` and making the release public.

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
