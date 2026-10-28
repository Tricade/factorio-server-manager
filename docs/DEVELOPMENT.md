# Human development guide

This guide is for local source changes to Factorio Server Control. It describes the current implementation; it is not a production deployment recipe. Read [CONTRIBUTING.md](../CONTRIBUTING.md) for the full test matrix and [OPERATIONS.md](OPERATIONS.md) for installation, persistence and recovery procedures. [ARCHITECTURE.md](ARCHITECTURE.md) and [API.md](API.md) are the companion design and HTTP contract references.

## Prerequisites and supported boundaries

- Node.js 24 and npm, using the checked-in `package-lock.json` (`npm ci`).
- Go 1.26.10, the exact version declared by `src/go.mod`. When needed, set `GOTOOLCHAIN=go1.26.10` (PowerShell: `$env:GOTOOLCHAIN='go1.26.10'`).
- Python 3 for repository validators and the disposable container harness.
- Git. Docker is optional for native development and required only for the disposable image/integration workflows.
- A separate, locally installed Factorio binary capable of running a server for live engine testing. Linux has the official headless download; native Windows development uses a separate installation of the Windows Factorio executable, not the Linux headless archive. The manager does not download or bundle the game as part of a native build. The shipped production image is built for Linux/amd64; the real-engine CI scenarios pin Factorio 2.0.77 and 2.1.20. Do not infer that every Factorio version, mod or host architecture has been integration-tested from those representative cases.

The ordinary unit suites use temporary directories and synthetic data. The browser suite is synthetic and owns its own loopback fixture. Credentialed Mod Portal downloads and live-world smoke tests are separate opt-in activities; see [CONTRIBUTING.md](../CONTRIBUTING.md#test-boundaries). Never point a test process at a real server, user save directory, or persistent deployment volume.

## Build and run an isolated native instance

Run commands from a disposable clone/worktree. Keep the repository root as the manager process working directory: the Go server serves the generated `./app/` directory relative to its current directory.

1. Install UI dependencies and build the production UI:

   ```sh
   npm ci
   npm run build
   ```

2. Create two independent scratch roots under the ignored `build/` directory: one for manager state and one for a test Factorio installation. Install or unpack a platform-appropriate Factorio runtime into the latter. Do not point `--dir` at your normal Factorio client directory.

   ```sh
   mkdir -p build/dev-runtime/manager build/dev-runtime/factorio
   ```

   PowerShell equivalent:

   ```powershell
   New-Item -ItemType Directory -Force build/dev-runtime/manager, build/dev-runtime/factorio | Out-Null
   ```

3. Copy `conf.json.example` to `build/dev-runtime/manager/conf.json` and edit the copy. Keep the cookie key and RCON password empty so startup generates fresh values. Set `secure` to `false` only for this loopback-only HTTP instance. Set `server_ip` to `127.0.0.1`, `server_port` to an unused local port, and set `sq_lite_database_file`, `factorio_credentials_file` and `log_file` to absolute paths inside the manager scratch directory. Set `logfile`, `console_log_file` and `chat_log_file` inside the isolated Factorio directory too: native Windows defaults can otherwise refer to the desktop user's Factorio AppData. Keep `settings_file` and `factorio_admin_file` under the test Factorio `config/` directory. The manager restricts the config/database permissions on platforms that support Unix-style modes, but scratch files should still remain outside a shared or published location.

4. Build the Go binary. From the repository root, build into the ignored scratch directory:

   ```sh
   (cd src && go build -o ../build/dev-runtime/manager/factorio-server-manager .)
   ```

   On Windows, use an `.exe` suffix:

   ```powershell
   Push-Location src
   $env:GOTOOLCHAIN = 'go1.26.10'
   go build -o ../build/dev-runtime/manager/factorio-server-manager.exe .
   Pop-Location
   ```

5. Start the binary from the repository root, with `--dir` set to the isolated Factorio root and `--conf` set to the private config you just edited. `--bin` is the Factorio executable relative to `--dir` (the default is `bin/x64/factorio`; use the host's actual executable name/path). Choose an unused manager port and an unused Factorio game port. The sample config's `secure:false` must not be used for a network-accessible service.

   ```sh
   ./build/dev-runtime/manager/factorio-server-manager \
     --dir ./build/dev-runtime/factorio \
     --conf ./build/dev-runtime/manager/conf.json \
     --bin bin/x64/factorio \
     --mod-pack-dir ./build/dev-runtime/manager/mod-packs \
     --host 127.0.0.1 --port 18080 --rcon-port 0
   ```

   `conf.json` is required and is read at startup. Values present in JSON override command-line and `FSM_*` flag/environment values; command-line and environment inputs are resolved by the flag parser before JSON is loaded. The JSON `server_ip` and `server_port` fields therefore override `--host` and `--port` when present. See [Configuration precedence and secret handling](#configuration-precedence-and-secret-handling).

6. Read the generated first-admin credential from the one-time credential file next to the configured SQLite database (`initial-admin-password.txt`). Treat it as a secret, use it only in this disposable instance, and change it immediately. It is removed after the initial admin changes their password. Do not paste it into chat, terminal transcripts, screenshots, tests or commits.

Factorio's own `config.ini` also needs a private `write-data` directory. In particular, on Windows the game runtime may otherwise use the desktop user's normal AppData locations. Keep that directory inside `build/dev-runtime/factorio/` and confirm the resulting saves/mods/config directories are all under the disposable Factorio root before testing.

## Configuration precedence and secret handling

`src/bootstrap/config.go` defines the supported command-line flags, `FSM_*` environment names, defaults and JSON config fields. The parser first resolves defaults, environment and explicit flags. The manager then reads the required JSON config file into that resolved config, so a JSON field wins over the corresponding CLI/environment value. When adding a setting, trace all three layers and the container/deployment wiring; do not assume an environment value wins.

| CLI flag | Environment | Default | Notes |
| --- | --- | --- | --- |
| `--conf` | `FSM_CONF` | `./conf.json` | Required existing JSON file; stores manager secrets and persistent paths. |
| `--dir` | `FSM_DIR` | `./` | Factorio installation and active saves/mods/config root. |
| `--host` | `FSM_SERVER_IP` | `0.0.0.0` | Manager HTTP bind address; JSON `server_ip` can override it. |
| `--game-bind-address` | `FSM_FACTORIO_IP` | `0.0.0.0` | Default Factorio game bind address. |
| `--port` | `FSM_PORT` | `80` | Manager HTTP port; JSON `server_port` can override it. |
| `--config` | `FSM_FACTORIO_CONFIG_FILE` | `config/config.ini` | Factorio `config.ini` path relative to `--dir` unless JSON replaces the resolved path. |
| `--max-upload` | `FSM_MAX_UPLOAD` | `512` MiB | General upload cap; some backup paths also enforce a 16 GiB hard ceiling. |
| `--bin` | `FSM_BINARY` | `bin/x64/factorio` | Factorio executable; relative paths are joined to `--dir`. |
| `--rcon-port` | `FSM_RCON_PORT` | `0` | `0` asks startup to choose a random local port. |
| `--glibc-custom` | `FSM_GLIBC_CUSTOM` | `false` | Optional custom loader configuration. |
| `--glibc-loc` | `FSM_GLIBC_LOCATION` | `/opt/glibc-2.18/lib/ld-2.18.so` | Used only with custom glibc. |
| `--glibc-lib-loc` | `FSM_GLIBC_LIB` | `/opt/glibc-2.18/lib` | Used only with custom glibc. |
| `--autostart` | `FSM_AUTOSTART` | `false` | Initial default only; a persisted manager-wide autostart setting is authoritative after it exists. |
| `--mod-pack-dir` | `FSM_MODPACK_DIR` | `./mod_packs` | Reusable manager-wide mod packs. |

The configuration's persistent JSON keys include `factorio_dir`, `saves_dir`, `basemod_dir`, `mods_dir`, `mod_pack_dir`, `config_file`, `config_directory`, `logfile`, `factorio_binary`, `rcon_port`, `rcon_pass`, `factorio_credentials_file`, `factorio_ip`, `factorio_admin_file`, `server_ip`, `server_port`, `max_upload_size`, `database_file`, `sq_lite_database_file`, `cookie_encryption_key`, `settings_file`, `log_file`, `console_cache_size`, `console_log_file`, `chat_log_file` and `secure`. Fields with `json:"-"` are not persisted by this struct. `conf.json.example` is a starter, not a template to copy into a live data directory without reviewing paths and `secure`.

Secret rules for development and code review:

- `rcon_pass`, `cookie_encryption_key`, Mod Portal user credentials, session cookies and the one-time admin credential are secrets. Startup replaces blank/legacy cookie and RCON values with generated values and saves them in `conf.json`.
- `factorio.auth` (or the configured `factorio_credentials_file`) stores Factorio service credentials. Keep it in the private scratch directory. Never place a password, user key, session cookie, generated credential, or real server settings in fixtures or logs.
- Cookie `secure` must remain true behind HTTPS. Only an isolated loopback test may use `secure:false`; never bind that instance to `0.0.0.0` or expose it through a proxy.
- Manager account hashes are in SQLite; raw passwords are not. The database and config are local state, not source files. File permission behavior differs across Windows and POSIX, so do not treat a Windows ACL setup as equivalent just because a Unix mode bit is present in code.
- Logs and user-visible errors should carry enough context to diagnose failures without embedding secrets, raw private file contents, or third-party mod setting values.

## Iteration and debugging

For UI changes, run `npm run watch` in one terminal and the Go binary in another. The UI is built into `app/` and served by the Go process; this project does not need a separate frontend dev server. Start the backend from the repo root so its static-file root is correct. Reload the browser after Webpack has emitted the changed bundle. Use browser devtools Network/Console to inspect synthetic requests, failed chunks and WebSocket lifecycle; do not capture real credentials or private game data in diagnostics.

For backend changes, use focused tests from `src/`, for example `GOTOOLCHAIN=go1.26.10 go test ./api -short -run 'TestProfile|TestAuth' -count=1` (PowerShell uses `$env:GOTOOLCHAIN='go1.26.10'`). Run package tests after a focused test, then vet where relevant. For lock, startup/shutdown or WebSocket changes, use the race checks in [CONTRIBUTING.md](../CONTRIBUTING.md#toolchain-and-local-checks). Tests use injected functions and temporary roots; prefer those seams to a real engine or service.

The backend logs to standard Go logging output; inspect that terminal for local failures. Factorio child-process output is consumed by the manager and may include mod-authored text, so do not paste raw logs into public issue reports without checking them for credentials, player names and private paths. There is no supported debug HTTP endpoint that exposes internal state.

## Safe change recipes

### Add an API route

1. Put a focused handler in the nearest `src/api/*_handler.go`; put filesystem/process/business behavior in `src/factorio/` or the owning package rather than adding another mixed handler block.
2. Add one `Route` entry in the appropriate catalog: `src/api/routes_catalog.go` for general routes or `src/api/routes_mod_catalog.go` for mod/Portal routes. `routeRequiresAdministrator` in `routes.go` defaults non-read methods to admin-only; only self-service logout/password changes are non-admin mutations. Explicit admin-only read routes are special-cased there. Set `ServerOff` only when the full operation requires a stopped Factorio process; that middleware returns 423 while the runtime is busy.
3. Preserve the API subrouter's authentication and `ProfileDataMiddleware`. If a handler acquires its own profile gate, update the middleware's narrowly matched bypass to avoid recursive lock/deadlock. Do not bypass the lock for convenience.
4. Bound every request before decoding. Reject unknown/trailing JSON where the handler's contract requires one object; validate path segments with `factorio.ValidatePathElement` (and file containment/symlink rules at the storage layer). For uploads, stream to a sibling temporary file, cap bytes and multipart overhead, remove multipart temp files, and validate archive contents before activation.
5. Keep response/error behavior explicit. Use safe client messages and server-side diagnostic context; never return passwords, cookies, session keys, RCON credentials, Factorio user keys or private file contents. Preserve the router's wrong-method 405 fallback.
6. Add handler tests for success, malformed/oversized input, auth/admin/viewer access, stopped/running state and rollback/error paths as relevant. Check route authorization and the method fallback in `src/api/authorization_test.go`.

See [API contract](API.md) for current route access and body shapes.

### Add a UI view that consumes the API

Use the shared Axios client in `ui/api/client.js`, resource modules under `ui/api/resources/`, existing notifications and route/layout conventions in `ui/App/`. Authenticated pages should use `ProfileContext` for active-profile state and explicitly identify profile-scoped versus manager-wide state. Do not create a second fetch/auth state system or infer that unavailable profile data is global. Render API values as React text, never raw HTML. Add pure behavior tests in `ui/tests/*.test.cjs`; add browser matrix coverage only for behavior the production UI needs to verify in a real browser layout.

### Add persistent state

First choose its owner: manager-global settings belong beside the manager config, while saves/mods/config/game mode/version/checkpoints belong to a profile. SQLite/GORM currently stores user accounts; most manager and Factorio state is JSON or files. Add a bounded, validated decoder, restrictive permissions, sibling temporary write and atomic replacement with rollback. Define a schema/version policy and test old/missing/corrupt/unsupported data before changing a stored format. Do not change the installed profile manifest or portable-backup format implicitly; see [ARCHITECTURE.md](ARCHITECTURE.md#database-schemas-and-compatibility).

For state that affects a running server, serialize through the existing lifecycle/profile/world-generation locks and snapshot methods. Avoid introducing another process launch path or replacing mounted directories. For a new environment option, update the bootstrap flag/config layer and the deployment/example files listed in [AGENTS.md](../AGENTS.md); source-only changes are not sufficient for a shipped option.

## Tests, packaging and release are different workflows

Run `python3 scripts/validate-source-layout.py` (Windows: `python`) after extracting or adding a module. Production imperative source has a 500-physical-line ceiling; keep responsibilities cohesive rather than compressing statements. Run `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...` from `src/` in addition to vet. Linux CI runs the complete short suite with `-race`; the targeted commands in CONTRIBUTING remain useful during iteration.

Internal Go helper refactors now use `(payload, status, error)` for `ModPortalList`/`ModPortalModDetails`, `(status, error)` for `FactorioLogin`, and error-last results for `ReadModPackRequest`. External source callers need to adjust tuple assignments; HTTP payloads and persistence layouts are not changed by this source-level update.

When testing a completed mutation followed by a failed refresh, assert both outcomes: the operation must not be falsely retried or reported as rolled back, while stale controls must not remain writable. Keep raw structured API data available for programmatic use, but render errors through `ui/api/messageText.cjs`; never pass arbitrary objects to notifications or plain-text Alert state. Shared process-log readers omit and drain oversized records before continuing; do not reintroduce a Scanner failure that abandons the child pipe.

Start with the relevant unit/handler/UI tests, then run the baseline in [CONTRIBUTING.md](../CONTRIBUTING.md#toolchain-and-local-checks) for normal source changes. Integration tests that mount Docker volumes or start Factorio must use their explicit disposable guard. Keep fixtures synthetic and small; never feed live data into tests.

For packaging changes, use `scripts/build-release.ps1` on Windows or `make gen_release` on Linux and inspect the generated archive, executable mode, included docs and checksums. These build artifacts belong in ignored output, not in source.

Release preparation and publishing are opt-in only. An ordinary code or documentation change must not change version numbers, tags, release dates or container aliases. When a release is explicitly requested, follow [RELEASE.md](../RELEASE.md) and the repository release rules; this guide does not replace that gated procedure.
