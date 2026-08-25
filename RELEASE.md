# Release process and notes

Factorio Server Control releases are user-facing changes, not generated commit lists. Prepare the notes while the release change is still reviewable, and describe observable behavior, upgrade impact and recovery steps in plain language.

The prepared source for the next release is `RELEASE-NOTES-0.17.1.md`. Keep the file publish-ready; the workflow treats draft markers, missing sections and mismatched versions as release blockers.

## Release checklist

1. Move the shipped entries from `CHANGELOG.md`'s **Unreleased** section into a dated SemVer section. Keep empty headings out of the changelog.
2. Set the same strict three-part version (without a `v` prefix) in `package.json`, `package-lock.json`, the versioned `RELEASE-NOTES-VERSION.md` file, the Unraid changes text and the Git tag. Remove `(planned)` from the Unraid entry, update its date and refresh screenshots whenever visible UI or branding changed.
3. Review the pinned GitHub Action commits, Go toolchain patch version and Docker base-image digests against their trusted upstream versions. Run the verification baseline in `TECHNICAL-NOTES.md`, including `govulncheck`, the Unraid validator, both local release builders where available and the production-container persistence tests.
4. Merge the signed-off release commit to `main`, create the SemVer tag on that exact commit and push the tag without force. Configure repository rules to prevent release-tag updates or deletion; the workflow also compares the remote tag object before every publication phase.
5. From the `main` branch, manually dispatch **Publish verified release** with the existing tag. Do not create or publish a GitHub release beforehand. The workflow checks the tag, versioned notes, finalized changelog and unused release/image targets; then it runs tests, creates and verifies both checksummed archives, and pushes and verifies only the immutable SemVer image.
6. After the image is verified, the workflow creates a hidden draft with the prepared notes and all three assets. It advances `latest` only when that alias still has the exact digest of the previous latest stable release, verifies the new digest, and makes the GitHub release public as its final mutation. Announce the release only after the workflow succeeds.
7. Keep the previous SemVer image available for rollback. Before an application downgrade, back up `/opt/fsm-data` and `/opt/factorio`; a newer database or profile format is not automatically guaranteed to be backward compatible.

The release workflow attaches versioned Linux and Windows ZIP files plus `SHA256SUMS`, and publishes a `linux/amd64` image with provenance and an SBOM. It never creates or moves the requested Git tag and never overwrites an existing SemVer image or release. `latest` is the sole moving container alias and is advanced only by the guarded forward-only release path.

A failed run after the SemVer image or hidden draft has been created is intentionally not rerunnable: the existing target causes the next attempt to fail rather than overwrite evidence. Inspect the verified image digest, draft assets and workflow log, resolve the partial state deliberately, and complete or withdraw it manually. Never delete and recreate a public SemVer release, move its tag or replace its image in place. Never put credentials, private hostnames, local filesystem paths or personal account data in release notes, screenshots or workflow output.

## GitHub release-notes template

```markdown
## Highlights

- Lead with the two or three outcomes that matter most to server operators.

## Added

- Describe new capabilities and where users find them.

## Changed

- Describe changed defaults, workflows or visible behavior.

## Fixed

- Describe resolved failure modes and who was affected.

## Security and privacy

- Describe hardening, credential handling or exposure changes without disclosing secrets. Write "No security-relevant changes" when appropriate.

## Compatibility and migration

- State storage, environment, image and API compatibility explicitly. Include required backup or migration steps, or "No migration required".

## Upgrade and rollback

- State prerequisites, expected restart behavior, persistent-volume requirements and the supported rollback path.

## Known limitations

- Call out material residual risks, platform limits or deferred work.

## Verification

- `factorio-server-manager-linux-VERSION.zip`
- `factorio-server-manager-windows-VERSION.zip`
- `SHA256SUMS`
- `ghcr.io/tricade/factorio-server-manager:VERSION` (`linux/amd64`)

Factorio Server Control is a maintained fork of the original Factorio Server Manager project and is not affiliated with or endorsed by Wube Software. Factorio is a trademark of Wube Software.
```
