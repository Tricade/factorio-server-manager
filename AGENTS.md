# Repository instructions for coding agents

These instructions apply to the whole repository. They supplement the user request; they do not authorize publishing, destructive data operations or changes outside the requested scope.

## Start with the repository state

- Base work on `main`. The historical upstream `develop` branch has different history and is not a valid base for this fork.
- Before editing, inspect `git status`, the relevant tests and the nearest implementation. Preserve unrelated user changes.
- Use a focused `feature/...`, `fix/...`, `docs/...` or `release/...` branch and open pull requests against `main`.
- Keep commits coherent. Update `CHANGELOG.md` under `Unreleased` for observable behavior changes, and update documentation, deployment examples and screenshots when their described behavior changes.
- Do not commit generated frontend output under `app/`, dependencies, build archives, local databases, credentials, logs or `.env` files. The exceptions already documented by `.gitignore` are the source examples and templates.

## Architecture and compatibility boundaries

- `ui/` is a React 18 single-page application built with Webpack. `app/` is generated output served by the Go binary. Put testable pure UI helpers in CommonJS `.cjs` modules and cover them in `ui/tests/*.test.cjs`.
- `src/` is the Go backend. Keep the upstream module path `github.com/OpenFactorioServerManager/factorio-server-manager`; it is a compatibility boundary.
- `src/factorio/map_snapshot_exporter/` is a manager-only Lua exporter run in an isolated Factorio workspace. Never copy its temporary save, mod state or script output back into a playable profile.
- One manager process owns exactly one Factorio child process. Profiles switch a stopped runtime; they are not concurrent servers. Do not introduce a second process path that bypasses the shared lifecycle, profile-data or world-generation locks.
- Preserve the technical executable/image/storage namespace `factorio-server-manager`, the `FSM_*` environment names, `/opt/fsm-data`, `/opt/factorio` and existing database/configuration names. The public product name is `Factorio Server Control`.
- Support exactly one Factorio persistence layout per container: either a combined `/opt/factorio` mount or all three split mounts `/opt/factorio/saves`, `/opt/factorio/mods` and `/opt/factorio/config`. Never configure both layouts.
- Treat each persistent mount point as immovable. Replacement operations must stage, validate and roll back entries inside the mounted filesystem; never rename or replace the mount directory itself. Keep the Docker-mounted integration tests current when directory transactions change.
- A combined mount retains the program tree. A split layout restores the exact recorded official Factorio version after recreation. Do not silently advance a persisted rolling release target.
- Keep manager-wide state and active-profile state distinct. Users, portal/RCON credentials, manager configuration, reusable mod packs and container autostart are global; saves, mods, Factorio configuration, selected save, game mode, exact version and checkpoints are profile-scoped.

## Backend safety rules

- Mutating API routes are administrator-only by default. Preserve authentication, explicit HTTP methods, the wrong-method `405` fallback, server-stopped guards and profile-data locking when adding routes.
- Validate every user-controlled path component with the portable path rules before joining it to storage. Do not accept absolute paths, traversal, control characters, Windows device names or symlink escapes.
- Bound request bodies, uploads, archive entry counts, expanded sizes, line lengths and external error bodies. Validate ZIP structure before activation and clean temporary multipart files.
- Use sibling temporary files/directories, restrictive permissions, atomic activation and rollback for persistent mutations. Do not report success until the committed state is valid; retain or restore the prior state on failure.
- Return safe client errors and log enough server-side context to diagnose failures, but never expose or print passwords, cookies, session keys, Factorio user keys, RCON secrets, the one-time administrator credential or private file contents.
- Maintain race-safe server state through the existing mutexes and snapshot methods. Run race-relevant changes through the targeted concurrency tests as well as the standard suite.
- Keep outbound traffic limited to the documented official Factorio release, authentication and Mod Portal services unless a change explicitly updates the privacy boundary and documentation. The deployed application contains no AI service or telemetry path.

## Frontend rules

- Use the shared API client, profile context, components and notification behavior instead of introducing parallel request or state systems.
- Treat backend locks and manifests as authoritative. UI loading state must not pretend unavailable profile data is global or safe to mutate.
- Preserve the administrator/viewer boundary in the interface, while relying on backend authorization for enforcement.
- Render untrusted values as React text. Do not use raw HTML for Factorio names, logs or API responses. Factorio rich-text markup must be rendered deliberately or converted to safe plain text.
- Keep controls keyboard operable and preserve focus trapping, Escape handling, labels, status text and reduced ambiguity in loading, empty and error states.
- The UI is English-only until a complete message catalogue exists; do not introduce isolated translated strings.

## Deployment files move together

- `docker/Dockerfile.registry` is the self-contained production build and supports `linux/amd64`, matching the official headless server. Keep base images and GitHub Actions pinned to reviewed immutable digests/commits.
- When adding or changing an environment option, review `src/bootstrap`, `README.md`, `docker/README.md`, both example env files, all relevant Compose files, `templates/factorio-server-manager.xml` and `scripts/validate-unraid-template.py` together.
- Keep the Unraid template on the non-overlapping split layout, required persistence guard, 180-second stop timeout, stable technical appdata root and accurate `<Changes>`/`<Date>` metadata.
- Shell scripts under `docker/` and `scripts/` must retain LF endings. Keep PowerShell path cleanup constrained to verified children of the intended build directory.
- Retain `AI-DISCLOSURE.md`, upstream attribution, the MIT license and the Wube non-affiliation/trademark notice in source and shipped artifacts.

## Verification

Use Node 24 and the exact Go patch version declared in `src/go.mod`. The baseline for source changes is:

```sh
npm ci
npm test
npm run build
cd src
go version
go test ./... -short -count=1
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
cd ..
python3 scripts/validate-unraid-template.py
```

- `go version` must report the version required by `src/go.mod`.
- Also run `git diff --check` and validate each changed Compose file with its documented example env file.
- Build `docker/Dockerfile.registry` for production-container changes. Run the mounted-directory, persistence-guard and split-mount checks from `.github/workflows/test-workflow.yml` whenever storage or entrypoint behavior changes.
- Use `scripts/build-release.ps1` on Windows or `make gen_release` on Linux when packaging changes. Inspect the ZIP contents, executable mode, included documentation and checksums.
- Add a regression test that fails before each bug fix. Exercise Windows-sensitive file behavior on Windows CI and Linux mount semantics in Docker where applicable.
- A documentation-only change may use the directly relevant validators locally, but the pull request still waits for the complete required CI matrix. Do not merge with pending or failing checks.

## Release discipline

Only perform release mutations when explicitly requested.

- Releases are strict three-part SemVer without a `v` prefix. A release preparation must align `package.json`, both version fields in `package-lock.json`, a publish-ready `RELEASE-NOTES-VERSION.md`, a dated `CHANGELOG.md` section, the Unraid `<Changes>` entry/date and the Git tag.
- Release notes must contain every heading required by `RELEASE.md` and describe user-visible outcomes, compatibility, migration, rollback, limitations and verification. Do not publish generated commit lists, draft markers, secrets, private hosts, personal data or local filesystem paths.
- Merge the release PR to `main`, wait for CI, then create the tag on that exact clean `main` commit and push it without force. Never move or delete a published release tag.
- Do not create a GitHub release in advance. Dispatch `.github/workflows/create-release-workflow.yml` from `main` with the already-existing tag. That workflow alone owns immutable SemVer images, the `latest` alias, checksummed Linux/Windows archives, provenance and SBOM publication.
- Never push `latest` or a SemVer image with `scripts/publish-image.ps1` or the non-release container workflow. Those paths are only for local builds or explicit non-release tags.
- Keep the previous SemVer image available. Before recommending rollback, require backups of `/opt/fsm-data` and the selected Factorio data layout; newer stored formats are not guaranteed to be backward compatible.
- A partially published release is evidence to inspect, not a target to overwrite. Use `resume_staged` only when the workflow's guarded preconditions match the verified hidden draft and immutable image; otherwise resolve the partial state deliberately as described in `RELEASE.md`.

## Pull-request handoff

- Explain the failure mode, the chosen invariant-preserving fix and the verification performed. Use `Fixes #N` when the merged PR should close an issue.
- Keep a PR open or draft only when the user requests manual review. Otherwise merge only after local verification and all GitHub checks pass.
- After merge, verify the PR state, linked issue state and clean synchronized `main` before starting the next task or a release.
