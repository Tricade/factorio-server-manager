# Contributing to Factorio Server Control

Work from `main` on a focused branch and preserve existing installations. The runtime model, storage paths and profile formats are compatibility boundaries, not cleanup targets. Read [AGENTS.md](AGENTS.md) for repository-specific invariants and [TECHNICAL-NOTES.md](TECHNICAL-NOTES.md) for the implementation.

## Source map

| Area | Location | Responsibility |
| --- | --- | --- |
| Application and authentication | `src/main.go`, `src/bootstrap/`, `src/api/auth.go` | Bootstrap, persistent accounts, sessions, graceful shutdown |
| HTTP contracts | `src/api/routes.go`, `src/api/*_handler.go` | Methods, permissions, bounded inputs, profile locks and status codes |
| Runtime and storage | `src/factorio/` | One playable process, profiles, saves, checkpoints, mods and releases |
| Isolated exporters | `src/factorio/map_snapshot_exporter/`, startup-settings exporter | Disposable engine workspaces; never write exported state back into a playable profile |
| UI | `ui/App/`, `ui/api/` | React views, shared profile state, API and WebSocket clients |
| UI tests | `ui/tests/`, `ui/e2e/` | Node helpers/components and production-bundle browser regressions |
| Deployment and release | `docker/`, `templates/`, `scripts/`, `.github/workflows/` | Images, persistence, Unraid metadata, CI and opt-in publication |

`ui/index.html` is the HTML source. `npm run build` generates the complete `app/` directory; do not edit or commit build output. Keep test data synthetic and small. Savegames, user data, tokens and locally generated credentials do not belong in fixtures or screenshots.

## Toolchain and local checks

Use Node.js 24, Python 3, and the exact Go patch in `src/go.mod`. Set `GOTOOLCHAIN=go1.26.10` in the shell when the installed Go differs (`$env:GOTOOLCHAIN='go1.26.10'` in PowerShell). Recheck that value when the module changes.

From the repository root:

```sh
npm ci
npm test
npm run build
python3 -m unittest discover -s scripts/tests -v
python3 scripts/validate-unraid-template.py
cd src
go version
go test ./... -short -count=1
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
cd ..
git diff --check
```

On Windows, use `python` if `python3` is unavailable. `-short` avoids credential-dependent Mod Portal integration tests. Clear any production `FSM_*` environment variables before tests: the API harness can use explicitly supplied paths. Never run a suite against production saves or manager storage.

Run a single UI file with `node --test ui/tests/profileContext.test.cjs`; run a Go subset from `src/` with `go test ./api -short -run TestDirectoryZip -count=1`. To inspect coverage, use `go test ./... -short -count=1 -coverprofile=../build/coverage.out` and `go tool cover -html=../build/coverage.out` after creating the ignored `build/` directory. Quote those complete flag arguments in PowerShell.

Concurrency changes also require race checks on a supported C toolchain. CI runs Linux race checks for additive mod packs, optional-dependency preferences, file locks, the WebSocket hub and startup/shutdown reservations. For a local lifecycle or lock/hub change:

```sh
cd src
go test ./lockfile ./api/websocket -race -short -count=1
go test . ./factorio -race -short -run 'Test(Shutdown|StopAlreadyStopped|FailedStartup|InvalidStartupSettings|ParallelStart)' -count=1
```

## Isolated native development

1. Build the UI, then run `go build -o ../build/factorio-server-manager .` from `src/` (use an `.exe` suffix on Windows).
2. Put a **separate** platform-appropriate Factorio installation in `build/dev-runtime/factorio/`. Do not point the development manager at a playing server or your normal client data.
3. Create `build/dev-runtime/manager/` and a private `conf.json` there. Start from `conf.json.example`, set the SQLite database, log and Factorio credential paths inside that private directory, and leave generated keys blank. Use `secure: false` only for loopback HTTP. JSON configuration overrides matching command-line settings.
4. Keep the working directory at the repository root so the backend serves `app/`. Pass `--dir`, `--conf`, `--mod-pack-dir`, `--host 127.0.0.1`, an unused `--port`, and the relative `--bin` for the installed Factorio platform. Configure Factorio's own `config.ini` with a private `write-data` directory too; Windows otherwise defaults to client AppData paths.
5. Read the generated one-time credential privately from the test manager-data directory. Do not paste it into a commit, screenshot, test fixture or PR. Keep autostart off for UI-only work.

Use `npm run watch` while editing, or rebuild once and reload the browser. The UI is served by Go, not by a separate Webpack development server. The [Docker guide](docker/README.md) covers a disposable container setup when native Factorio is not available.

## Test boundaries

| Check | What it verifies | What it does not establish |
| --- | --- | --- |
| Node tests | Helpers, real profile-provider ordering, upload guards, client errors, socket lifecycle and build configuration | Browser rendering, layout or native file dialogs |
| Browser matrix | Every route, lazy assets, keyboard, mobile layout, permissions and failure recovery in Chromium/Firefox/WebKit | Real game state; the loopback API fixture is synthetic |
| Short Go suite | Parsers, authorization, transactions, rollback fixtures, settings and handler contracts | Every combination of real third-party mods |
| Race tests | Concurrent code exercised by those tests | Freedom from all possible races |
| Docker CI | Production image, mounted-directory transactions, persistence guard, real-engine creation/import/checkpoints and restart/recreation on combined and split mounts | Credentialed Mod Portal downloads, arbitrary mods, all game releases or full backup disaster recovery |
| Live world smoke | Real previews, world creation, start/stop and checkpoint round trip | Production load, all Factorio releases or large modded saves |

Add a regression that fails before every bug fix. Use temporary directories and explicit dependency boundaries. Keep the shared client/provider behavior in the test rather than duplicating it in a test-only implementation. Browser smoke tests should cover admin and viewer permissions, stopped/running/unknown state, keyboard navigation, failed requests, profile switching and narrow layouts.

### Browser regression matrix

After `npm ci` and `npm run build`, run `npx playwright install --with-deps chromium firefox webkit` and `npm run test:browser`. The suite owns a loopback fixture on port 18089 and refuses to reuse an existing listener. It never opens a real manager or reads local saves. The five projects cover desktop Chromium, Firefox and WebKit, plus mobile Chromium/WebKit viewports. External asset requests fail the tests. Native file dialogs and physical mobile devices are outside that matrix. Test traces and screenshots stay in ignored `test-results/`; CI retains only synthetic failure diagnostics for seven days.

### Disposable real-engine container matrix

The Docker CI job runs `scripts/test-container-lifecycle.py` twice: Factorio 2.0.77/base with a combined mount, and 2.1.20/Space Age with split Unraid mounts. These are representative pinned scenarios, not a claim to cover every cross-product of versions, modes and mods. The script requires Python's standard library, Docker and official Factorio download access. To reproduce:

```sh
docker build -f docker/Dockerfile.registry -t factorio-server-manager:ci .
python3 scripts/test-container-lifecycle.py --disposable --version 2.0.77 --mode factorio --layout combined
python3 scripts/test-container-lifecycle.py --disposable --version 2.1.20 --mode space-age --layout split
```

It creates uniquely labelled containers and named volumes, publishes only the manager HTTP port on loopback, and creates a small world and an original two-file fixture mod. It checks engine-evaluated startup settings, byte-preserving save uploads, locked running operations, live RCON/checkpoints and checkpoint restore. Restart and recreation must retain the administrator credential, active profile, selected save, exact installed engine, game mode, autostart, fixture mod/settings and fixed checkpoints—even if the first-install channel environment changes.

No existing endpoint, volume, host directory, user save or account can be passed to this runner. It reads its generated administrator credential only into memory, does not publish game ports, and never prints credentials or raw container logs. Cleanup verifies both ownership labels and generated names before removing its containers/volumes. An uncleanly killed runner may leave labelled test resources; inspect their names and labels before manually removing them. Never use broad Docker prune commands to clean a test run.

The successful *Import mods from save* download flow needs Mod Portal authentication. Credential-free CI tests its rejection and proves that the old mods, game mode and save remain intact; staged discovery/activation and rollback have separate Go fixture tests. A successful authenticated third-party download remains an explicit manual test with a disposable account-backed installation, not a secret embedded in CI.

### Optional live world test

`scripts/smoke-new-world.sh` changes the active game mode when requested, creates a world, starts/stops it, creates/restores a checkpoint and removes its own test files. It must target a disposable manager with no players and a real local Factorio engine. It needs POSIX `sh`, `curl`, `jq`, `od` and the one-time credential file of that disposable instance. It is not automatically run by the normal CI workflow.

```sh
FSM_SMOKE_DISPOSABLE=1 \
FSM_BASE_URL=http://127.0.0.1:8080 \
FSM_CREDENTIAL_FILE=/path/to/disposable/initial-admin-password.txt \
sh scripts/smoke-new-world.sh
```

An interrupted run can leave test data if the manager is unreachable; inspect the disposable installation before removing it. Never use the opt-in as permission to point this script at a real server.

## Dependency and security review

Use `npm audit` and the reachability-aware Go scanner in addition to tests. Inspect the advisory and the code path before deciding whether an alert affects build tools, shipped browser code or server runtime. A clean `npm audit --omit=dev` alone does not prove that a frontend is safe: the browser bundle also uses packages classified as development dependencies.

Do not use `npm audit fix --force` to change the framework as an incidental security fix. Keep immutable container/action pins and exact Go toolchain alignment. The Tailwind 4 migration and removal of the unpatched Tailwind 3 build chain are documented in [Technical notes](TECHNICAL-NOTES.md#frontend-dependency-compatibility).

## Pull requests and releases

Keep commits focused, add user-visible changes to `CHANGELOG.md` under `Unreleased`, update affected documentation and use synthetic screenshots when controls or layouts change. Include the failure, preserved invariants, tests and remaining limits in the PR. Merge only after the full required CI matrix passes. Do not update the release version, tag, Unraid release date or registry image unless a release was explicitly requested; [RELEASE.md](RELEASE.md) owns that separate procedure.
