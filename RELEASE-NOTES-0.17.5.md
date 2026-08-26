## Highlights

- Save-mod import now installs every available mod even when an old save references an exact release that the Mod Portal no longer offers.
- Authenticated portal archives whose `info.json` identifies a different mod are safely omitted instead of aborting the complete import.
- Partial imports clearly list each skipped mod and version in the interface, while transient and systemic failures retain the existing rollback behavior.

## Added

- Added structured `skipped` results to save-mod import responses with stable reasons for unavailable releases and archive identity mismatches.
- Added regression coverage for partial activation, response encoding, portal metadata/download availability, mismatched archive identity, bounded UI feedback and mounted-directory replacement.

## Changed

- The Mods page now explains before confirmation that permanently unavailable releases and mismatched archives will be skipped, then shows a bounded informational notification when an import is incomplete.
- Successful imports with no skipped mods retain the existing green success notification and `mods` response field.

## Fixed

- Fixed old saves losing the entire prepared mod replacement when one exact Mod Portal release is no longer listed or its official authenticated download identifies a different mod.
- Authentication, rate-limit, network, Mod Portal server, malformed archive, validation and activation failures still abort safely and preserve the previous active mod set and game mode.

## Security and privacy

- No new credential store, telemetry, AI service, public endpoint or outbound destination is introduced. Mod Portal authentication continues to use the existing locally stored service credential without returning authenticated download URLs to clients.
- Archive names, save names, request sizes and expanded ZIP contents retain their existing validation and bounds. Identity-mismatched archives are rejected before activation.

## Compatibility and migration

- No database, profile, environment-variable or persistent-storage migration is required. Existing combined `/opt/factorio` mounts and documented split saves, mods and config mounts remain compatible.
- The save-mod import API retains its existing `mods` field and adds a `skipped` array. Clients that only read `mods` remain compatible.
- An unavailable required mod can still prevent Factorio itself from loading that save; this release recovers and reports the available mod set but cannot replace a removed or invalid third-party mod.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.5` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. Stop the Factorio server, confirm the Mod Portal login, select the save on the Mods page and run **Import mods from save**. Review any skipped names and versions before starting Factorio.

To roll back, stop the container cleanly and recreate it with the immutable `0.17.4` image. This release does not intentionally change persisted formats; restore the pre-upgrade backup if required.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- Only a missing exact release, Mod Portal HTTP 404/410 or an archive identity mismatch is skipped. Temporary portal, authentication and validation failures intentionally abort the transaction so they can be retried without replacing the working set.
- Save import detects required mods and game mode but does not automatically install another Factorio version or make saves compatible with removed third-party mods.

## Verification

- `factorio-server-manager-linux-0.17.5.zip`
- `factorio-server-manager-windows-0.17.5.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.5` (`linux/amd64`)
- SemVer image labels, Git revision, provenance and SBOM match the 0.17.5 tag
- Linux and Windows Node/Go tests, production container build, mounted-directory transactions, persistence guards, Unraid split mounts, template validation, `go vet` and `govulncheck`
- Official Mod Portal state verified for an unavailable Aircraft release and a SHA-1-correct Laser Tanks archive with mismatched identity; the attached Space Age save also completed the normal browser import with 19 mods and the disposable portal account

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
