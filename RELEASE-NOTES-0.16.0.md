## Highlights

- A new player overview combines live connected-player state with a snapshot-based playtime ranking, including honest offline, error and freshness states.
- Factory-map snapshots can now add a lazy-loaded Canvas building layer with a color legend at detailed zoom levels, while legacy snapshots continue to render their base map.
- The maintained fork now appears publicly as **Factorio Server Control**, with new app and maintainer artwork, safer container defaults and a more complete verifiable release pipeline.

## Added

- Live player data while Factorio is running and persisted snapshot player-time metadata for stopped-server context.
- Bounded, authenticated entity-detail exports for map surfaces and a keyboard-operable zoom, pan and fullscreen viewer.
- A canonical Unraid Community Applications template, separate app/maintainer icons and automated local metadata validation.
- Versioned Linux and Windows archives, `SHA256SUMS`, license/document inclusion, OCI provenance and an image SBOM.

## Changed

- Login, navigation, connection state, document titles and public package/OCI metadata use the Factorio Server Control identity. Technical repository, image, executable, module, environment and storage identifiers remain stable.
- Profile-scoped screens discard stale form and polling state after a profile change. Mutating controls fail safe while server status is unknown, running or stopping, and scope badges distinguish profile data from manager-wide settings.
- Modals, tabs and mobile navigation have stronger focus, Escape-key, keyboard and scroll-lock behavior; loading, empty, validation and error states are more explicit throughout the UI.
- SIGTERM now stops new manager HTTP work and requests a clean Factorio shutdown inside a 180-second container grace period.
- Split-mount container recreation reinstalls the exact recorded official Factorio version; a combined `/opt/factorio` mount continues to retain the installed program tree.

## Fixed

- Direct Unraid/private-HTTP login now uses an explicit non-Secure session-cookie setting, while the HTTPS/Traefik example keeps Secure cookies enabled.
- Empty example RCON credentials now generate a random persisted localhost-only password instead of encouraging a reusable placeholder.
- Local Docker and PowerShell release builds now include the current backend, recursive frontend output, favicons, nested brand assets, license and operator documentation.
- Invalid manager JSON is rejected without overwriting the source file, and generated session/RCON secrets receive restrictive permissions.
- Factorio release activation retains the previous program tree until the new binary metadata and runtime state have both validated, allowing post-activation failures to restore the prior release.
- Checkpoint player counting now uses the read-only `/players online count` command; RCON access is serialized and local, and WebSocket shutdown avoids the previous race/deadlock paths.
- Save/checkpoint uploads now require structurally valid Factorio ZIPs and activate atomically; bounded mod-pack/ZIP handling no longer reads unbounded archives into memory.

## Security and privacy

- GitHub Actions and Docker base images are pinned to reviewed immutable revisions. Release builds require the patched Go 1.25.14 toolchain and a `govulncheck` result with no reachable vulnerabilities; published images include provenance and an SBOM, while downloadable archives include SHA-256 checksums.
- Stable publication refuses moved tags and existing SemVer release/image targets, verifies the checksummed archives and immutable container image before staging a hidden draft, and exposes the GitHub release only after every artifact is complete.
- Container startup refuses missing manager-data persistence and incomplete game-data mappings, preventing silent credential or save loss on image replacement.
- Initial Factorio installation validates an exact requested version and copies the staged program tree before making its executable available.
- Map-detail files reject symlinks, invalid geometry, excessive line/file/entity counts and changed files between validation and open.
- State-changing GET routes were replaced with appropriate methods. Mutations and RCON-console WebSockets are administrator-only by default, while the `viewer` role is read-only.
- Login now has bounded attempts, strict JSON, generic failure responses and finite sessions; authenticated responses add CSP, frame, MIME-sniffing, referrer and permissions protections.
- The manager has no built-in analytics or telemetry. Operational state remains in the documented persistent mounts; release/mod operations contact official Factorio services, a submitted Factorio password is exchanged for a locally persisted user key rather than retained, and optional external links open only when selected.
- No credentials, Factorio binaries or private deployment data are included in the repository or release archives. Factorio is downloaded separately from its official service.

## Compatibility and migration

- No branding-only data migration is required. The `factorio-server-manager` repository/image/executable namespace and default Unraid root `/mnt/user/appdata/factorio-server-manager/`, the upstream Go module path, `FSM_*` environment names, `/opt/fsm-data`, `/opt/factorio` and existing database/config filenames are intentionally unchanged so original-manager data can be adopted without renaming its technical folders or mounts.
- To migrate an original Factorio Server Manager installation, stop it first, back up its data and give the replacement exclusive access to the copied manager data plus either the complete Factorio mount or all three documented split mounts. Locate and copy legacy anonymous volumes manually.
- Legacy factory-map snapshots without entity details remain usable; the detailed building layer simply stays unavailable until a new snapshot is generated.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and `/opt/factorio` (or all three split game-data mounts).
2. Review `FSM_COOKIE_SECURE`: use `false` only for a trusted direct HTTP endpoint and `true` when every user enters through HTTPS.
3. Pull `ghcr.io/tricade/factorio-server-manager:0.16.0`, recreate the container with all required mappings and keep the 180-second stop timeout.
4. Verify login, active profile, selected save/version, mods, map snapshot and server start/clean stop before removing the backup.

To roll the application back, stop it cleanly and recreate it with the previous immutable SemVer image. Restore the pre-upgrade manager/game-data backup if a newer persisted format is not accepted by the older application; backward database/profile compatibility is not guaranteed. Factorio game-version changes remain separate stopped-server operations in the UI.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- The application process currently runs as root inside its container so it can retain port 80 and write original-manager data with existing ownership. The app container is `privileged=false`, receives only the documented explicit data mounts and does not mount the Docker socket; keep the UI on a trusted LAN/VPN or behind a properly secured HTTPS reverse proxy.
- Follow-up hardening should move the internal UI to an unprivileged port and add an explicit, tested ownership-migration mode covering existing combined/split Unraid mounts before making a non-root UID the default.
- Factory maps are periodic snapshots, not a live graphical client. Detailed geometry contains prototype footprints, not Factorio sprites, and requires a newly generated snapshot.
- Factorio archives come from the official HTTPS download service, which does not provide a checksum consumed by the manager; split-mount recreation needs that service to restore the recorded version.

## Verification

- `factorio-server-manager-linux-0.16.0.zip`
- `factorio-server-manager-windows-0.16.0.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.16.0` (`linux/amd64`)
- SemVer image label, Git revision, provenance and SBOM match the 0.16.0 tag

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
