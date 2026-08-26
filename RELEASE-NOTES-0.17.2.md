## Highlights

- Importing mods from a Space Age save now keeps Space Age active and recognizes `elevated-rails`, `quality` and `space-age` as built-in Factorio features instead of trying to download them from the Mod Portal.
- Save imports prepare and validate the complete replacement before activation. A missing or unavailable community mod leaves the previous mod set and game mode untouched.
- Structured GitHub forms now collect reproducible environment details, continuous log context and compatibility/security expectations for bug reports and feature requests.

## Added

- Added bug-report and feature-request forms plus a pull-request template with required diagnostic, reproduction, verification, compatibility and secret-removal checks.
- Added strict CI validation for the repository forms so malformed fields or weakened required-reporting standards fail the normal Go test suite.

## Changed

- The Mods page now submits save imports as one server-side transaction and explains that the existing set remains active when preparation fails.
- Blank GitHub issues are disabled in favor of the guided bug and feature forms.

## Fixed

- Fixed importing mods from Space Age saves reverting the active profile to base Factorio and leaving no imported mods when built-in expansion modules were absent from the Mod Portal.
- Fixed partial save-import failures deleting or replacing the active profile's working mod configuration before all required community archives were available and valid.

## Security and privacy

- The new import endpoint is administrator-only, accepts only `POST` while the Factorio server is stopped, and retains the existing profile-data and process-lifecycle locks.
- Save names, mod names, archive counts and built-in metadata are bounded and path-validated; symlinked or non-regular built-in metadata is rejected before activation.
- Client errors remain generic while diagnostic details stay in server logs. No credentials, private file contents, telemetry or new outbound service are exposed; community archives still use only the documented Factorio Mod Portal.

## Compatibility and migration

- No database, profile or persistent-storage migration is required. Combined `/opt/factorio` mounts and the documented split saves, mods and config mounts remain supported.
- The added `POST /api/saves/mods/import` route does not remove the existing mod APIs. Repository, image, executable, environment and persistent-storage identifiers remain unchanged.
- Base-Factorio saves explicitly disable the Space Age feature set, while Space Age saves enable the installed built-in features recorded in the save.

## Upgrade and rollback

1. Back up `/opt/fsm-data` and either the combined `/opt/factorio` mount or all three split `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config` mounts.
2. Pull `ghcr.io/tricade/factorio-server-manager:0.17.2` and recreate the manager container with the existing mappings and 180-second stop timeout.
3. With the Factorio server stopped, import a known Space Age save on the Mods page and verify that the Space Age feature mods and any recorded community mods are enabled before starting the server.

To roll back, stop the container cleanly and recreate it with the previous immutable `0.17.1` image. Restore the pre-upgrade backup if required; this release does not intentionally change persisted formats.

## Known limitations

- The production image is `linux/amd64`, matching the official Factorio headless archive.
- The Mods import screen still requires valid Factorio portal credentials, and community mods must still have the exact recorded version available from the Mod Portal. A failed lookup or download preserves the existing active set.
- Built-in Space Age components are supplied by the installed Factorio program and are not copied into the persistent mods directory.

## Verification

- `factorio-server-manager-linux-0.17.2.zip`
- `factorio-server-manager-windows-0.17.2.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:0.17.2` (`linux/amd64`)
- SemVer image labels, Git revision, provenance and SBOM match the 0.17.2 tag

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
