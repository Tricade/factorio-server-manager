# Project quality review — 2026-10-17

This historical review is retained. See the [2026-10-28 maintainability review](QUALITY-REVIEW-2026-10-28.md) for the current source decomposition, additional failure-path corrections, developer references and measured verification boundaries.

This review started from `main` at `d74221f`, with application version 0.20.0. The follow-up work below addresses its five outstanding items. It covers runtime boundaries, HTTP handlers, authentication, profile/UI state, archives, locking, dependencies, build configuration, tests and documentation. It does not publish a release, change deployed installations or certify that every possible workflow is defect-free.

The main weaknesses found were inconsistent behavior between old and new code paths, asynchronous UI state ownership, and gaps in regression coverage. The fixes preserve the one-server model, existing profiles, saved game versions, credentials, storage paths and mount layouts.

## Findings and corrections

| Area | Finding | Correction and regression coverage |
| --- | --- | --- |
| Connection lifecycle | A callback from an obsolete WebSocket could close or replace a newer connection; late messages could arrive after logout. A send failure could escape to the caller. | Bind callbacks to their connection, detach them on logout, cancel stale reconnect timers and report failed sends without replaying commands. `ui/tests/socket.test.cjs` exercises these sequences. |
| Profile state | An older list response could revert a completed profile mutation or replace the newest loading/error state. | Generation-based response ownership and profile-manifest validation in the actual React provider. Deferred-response tests cover mutation races, invalid responses, failures and retry. |
| Save uploads | The UI offered uploads while Factorio was running, although the API requires a stopped server. | Disable the visible file picker and submit control when locked, and guard submission itself. The form regression and browser checks cover the restriction. |
| New passwords | Validation accepted passwords longer than bcrypt's 72-byte limit, turning invalid input into a hashing failure. | Match API and UI validation to 10–72 UTF-8 bytes. Test ASCII/multibyte boundaries and retention of the original hash/login after a rejected change. Existing hashes are unchanged. |
| Directory downloads | ZIP creation flattened subdirectories, producing duplicate names for different nested files. | Preserve relative forward-slash paths. Tests inspect archive entries and bytes, failure response behavior, locks and symlink rejection. |
| File locks | Equivalent absolute paths containing `..` used different lock keys. Released files remained in the map indefinitely. | Clean path keys and remove idle entries after the last unlock. Add reader/writer, alias and concurrent-reader tests, plus Linux race checks in CI. This is process-local coordination, not a cross-process filesystem lock. |
| Legacy save creation | The old create-save helper launched a separate unguarded engine command outside the new world-generation pipeline. | Keep the legacy endpoint but use the shared staged generator, reject existing destinations, enforce stopped state and omit raw child output from success responses. Regression tests prove existing bytes survive and direct calls cannot bypass the running-state guard. |
| Version parser | Reusing a version value retained omitted components; invalid input could partially mutate the receiver. | Parse into a new value and commit only on success. Repeated/invalid-input tests cover the contract without removing accepted short version forms. |
| API error reporting | Cancelled requests could report the manager as offline; nested error objects could reach notification handling. | Keep cancellation quiet and normalize error text. Client tests retain the existing authentication, size-limit and backup-error behavior. |
| CSS production build | A second extraction plugin occupied the CSS minimizer slot. | Use the already-installed CSS minimizer beside Terser and test development/production configurations. The production build succeeds. |
| Manual live smoke test | Save removal and server stop used obsolete GET methods; cleanup did not distinguish an existing save from one created by the test. | Use DELETE/POST, require `FSM_SMOKE_DISPOSABLE=1`, and track successfully created test saves before cleanup. Static script checks cover methods and the opt-in. The live script was not executed against a game in this review. |

Each bug fix was preceded by a failing regression where the behavior is covered by the new automated tests. Additional request-decoding tests cover world-generation unknown fields, malformed/trailing JSON, numeric input, request bounds and wrapped conflict/locked error mapping.

## Cleanup decisions

- Remove unused direct dependencies: `classnames`, `locks`, `prop-types`, `react-native-listener`, `style-loader` and `webpack-fix-style-only-entries`. Keep required transitive dependencies in the lockfile.
- Declare `events` directly because the browser code imports it; do not depend on Webpack to supply it accidentally.
- Classify Tailwind as a build dependency and add the matching React 18 test renderer as a test dependency. Classification alone did not resolve the advisory; the subsequent Tailwind 4 migration removes the affected dependency chain.
- Remove the unused authentication helper and empty router declaration; preserve public routes and their method/role/stopped-state guards.
- Stop tracking the stale generated `app/index.html`. The complete `app/` directory is rebuilt from `ui/` and ignored, matching existing container/release builds and repository policy.
- Document operations, recovery, local isolation and test boundaries in [OPERATIONS.md](OPERATIONS.md), [CONTRIBUTING.md](../CONTRIBUTING.md) and [TECHNICAL-NOTES.md](../TECHNICAL-NOTES.md). Correct the earlier claim that ordinary CI runs a live RCON/checkpoint scenario.

No business feature was removed just to shorten the code, and no cosmetic rewrite of working backend modules was made.

## Feature consistency

| Feature group | Assessment |
| --- | --- |
| Profiles and process controls | One active playable server with stopped profile switching remains coherent. The profile library is not a set of concurrent servers. Preserve this boundary in both UI and documentation. |
| Saves, new worlds and checkpoints | Initial creation belongs to an empty profile; an established profile retains save upload/download and checkpoints. Autosaves rotate, checkpoints retain snapshots, and profile exports transfer setups. These are distinct functions, not redundant buttons to remove. |
| Versions, game mode and mods | Exact engine version, release target and built-in expansion state are separate choices. Required dependencies, optional defaults, startup settings, pack addition and full replacement have different effects; retain explicit reviews and confirmations. |
| Backups and persistence | Portable profile/mod archives are not a substitute for a consistent backup of manager storage plus all selected game-data mounts. Mount-safe entry replacement and existing rollback tests remain required. |
| Factory map and player data | The map is a periodic isolated chart snapshot, not graphical live streaming. Player-time snapshot metadata is not a live productivity score. Keep scheduling, manual-only/disabled mode and resource limits explicit. |
| Permissions | Viewers can read and download permitted content but cannot operate the game. They are not anonymous public spectators; downloaded saves can contain private player data. Backend enforcement remains authoritative. |

The existing saves screenshot depicts a stopped server and remains representative: uploads stay available in that state. No layout, menu or branding redesign was introduced by this review.

## Initial audit verification

Windows, Node.js 24.19.0 and the required Go 1.26.9 toolchain:

- Clean `npm ci`, **47 passing Node/UI tests**, and a successful production Webpack build.
- `go test ./... -short -count=1` and `go vet ./...` pass. The short suite reports **49.7% aggregate Go statement coverage**; this is a measurement of the exercised Windows paths, not a completeness claim.
- Nine Python repository-validator tests and the Unraid metadata validator pass.
- `govulncheck@v1.7.0` finds no reachable vulnerability and no finding in imported packages. It reports one advisory in an unused package of a required module; this is not equivalent to zero module-level advisories.
- Browser checks of the production UI with a synthetic loopback-only API confirm running/stopped/unknown upload restrictions, viewer-only save downloads, disabled running-profile switching and Escape closing the profile chooser. No real server, savegame or credential was used.
- `git diff --check` passes. No release version, image tag, deployment configuration, private save or credential was added.

Pull requests must also pass the existing Linux/Windows CI matrix, production image build, mounted-directory/persistence checks and the added file-lock/WebSocket race checks before merging. Local synthetic browser checks do not replace those jobs or a live-engine test.

## Follow-up implementation and verification

| Original follow-up | Implementation and evidence |
| --- | --- |
| Unpatched build advisory | Migrate to Tailwind 4 and its PostCSS integration; remove the unused Sass/resolve-url/autoprefixer chain. The lockfile contains no `braces`, and `npm audit` reports **zero vulnerabilities** on 2026-10-17. Tests compile the actual stylesheet and check existing colors, responsive utilities and opacity. Browser support is now Chrome 111+, Safari 16.4+ and Firefox 128+. |
| Disposable real-engine tests | `scripts/test-container-lifecycle.py` creates its own labelled containers and volumes. Both **Factorio 2.0.77 with combined storage** and **Space Age 2.1.20 with split storage** pass locally: original fixture-mod upload, engine-evaluated startup settings, world creation, byte-preserving save upload, live RCON readiness, checkpoint creation/restore, running-state guards, autostart, restart and full container recreation. The same two scenarios are mandatory CI steps after the existing mounted-directory checks. |
| Browser matrix | Add **60 scenarios** across desktop Chromium, Firefox and WebKit plus mobile Chromium/WebKit emulation. They cover all ten authenticated routes, anonymous/viewer access, keyboard profile selection, running/unknown-state restrictions, failed operations with retained drafts, lazy-asset navigation and recovery. The synthetic API is loopback-only and unexpected external requests fail the tests. The matrix exposed and now covers mobile focus loss, nested Escape handling, hidden-tooltip overflow and off-screen help text. Release verification also reproduced missing focus restoration in Windows WebKit; disabling native pointer focus makes that regression reproducible on every engine. Shared buttons and the mobile-navigation trigger now focus before opening an overlay. |
| Startup and shutdown risk | New regressions reproduce a stopped-server `Stop` call incorrectly locking the runtime, and a failed startup leaving a queued stop/reservation behind. Fix both using the existing state mutex. Shutdown also waits for a pending startup, not just an already running child, within its existing timeout. Tests cover invalid settings without losing the selected save, stopped/pending/stopping shutdown and bounded error handling. The targeted Linux race suite includes these paths. |
| Frontend size and loading | Lazy-load the ten management views while retaining the shell, controls and login. The production main bundle drops from about **817 KiB to 450 KiB uncompressed**; shared and current-route chunks still load as needed. Hashed chunks use same-origin root-relative URLs, including direct deep links. A failed chunk leaves navigation available and offers an explicit reload rather than discarding work automatically. All existing views remain. |

The follow-up local baseline passes **50 Node/UI tests**, **13 Python validator tests**, the Unraid validator, the full short Go suite, `go vet`, the production UI/image builds and both real-engine scenarios above. `govulncheck@v1.7.0` still reports no reachable or imported-package finding; the unused-module advisory described in the initial audit remains distinct from the clean npm result. CI runs the browser matrix, Windows/Linux suites, race checks, mounted-directory tests and real-engine scenarios before merge. The existing desktop screenshots remain representative; no menu, branding or business-feature redesign is part of this work.

## Remaining test boundaries

- Successful authenticated Mod Portal download/import needs a Factorio account. CI includes no portal account or private save: the real-engine scenarios instead prove that an import rejected for missing credentials preserves installed mods, game mode and save bytes. Existing Go tests cover successful import planning, staged changes and rollback using controlled fixtures. A credentialed production-portal smoke test remains a manual check.
- The engine matrix uses two exact representative versions and a tiny generated world, not every historical version, third-party mod combination or large factory. Browser mobile projects emulate devices rather than running on physical phones; native operating-system file dialogs are outside this matrix.
- Coverage is risk-based, not a claim of complete branch or behavior coverage. The fixes and continuous checks close the five scoped follow-ups without declaring the software defect-free.
