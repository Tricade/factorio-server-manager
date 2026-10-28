# HTTP and WebSocket API reference

This is a human-maintained map of the current general and mod route catalogs in [`src/api/routes_catalog.go`](../src/api/routes_catalog.go) and [`src/api/routes_mod_catalog.go`](../src/api/routes_mod_catalog.go), plus the separately registered login and WebSocket routes in [`src/api/routes.go`](../src/api/routes.go). Handler code remains authoritative for exact field validation and evolving result structures. The API is not currently versioned independently of the manager release.

All REST routes below use the `/api` prefix. Except `POST /api/login`, REST calls require a valid cookie session. `A` means administrator; `V` means viewer; `A/V` means either authenticated role. Mutations are administrator-only by default. The explicit non-admin exceptions are self-service logout and password change. The three administrator-only reads are called out individually. `Stopped` means the route is registered behind `ServerOffMiddleware`, which returns HTTP 423 while the Factorio process is busy; other routes can still have operation-specific stopped/busy checks in their handlers.

## Authentication and responses

Login uses a cookie-backed session. Successful login returns the current user without a password field. The cookie is HttpOnly and SameSite=Lax; its Secure bit follows the configured `secure` value. Login input is capped at 1 MiB, rejects unknown/trailing JSON, and is rate-limited. A credential change invalidates previous sessions through a derived credential-version value; the current password-change request refreshes its own session.

The common `ReadRequestBody` helper caps JSON at 1 MiB and is used by legacy handlers as well as login, server start/settings/release/game-mode, mod management and Portal login/install requests. Profile JSON is capped at 64 KiB; world-generation JSON at 1 MiB; checkpoint settings at 8 KiB; autostart, map-snapshot settings, Portal preferences and mod-pack merge revisions use smaller 4 KiB caps. A few handlers have their own decoders and may report a size violation as 400 instead of 413. Multipart handlers have operation-specific request/file caps; use the backup limits endpoint for the effective backup cap.

There is no universal success or error envelope. Legacy handlers often JSON-encode a string, object, array or `null`; newer handlers may return a typed JSON object, binary payload, or `http.Error`'s `text/plain` body. Do not assume every failure is `{ "error": ... }`. The shared UI client displays `text/plain` errors and recognizes object `error`/`message` values, while 401 triggers reauthentication. Common status meanings are 400 malformed/invalid request, 401 unauthenticated or invalid credentials, 403 authenticated but not an administrator, 404 absent resource, 409 conflicting/busy operation, 413 request too large, 423 stopped-runtime requirement/operation lock, 422 invalid domain operation, 502 upstream service error and 500 unexpected internal error. Exact statuses vary by endpoint and are listed where stable in handler code.

Routes use explicit HTTP methods. The router adds path-only fallbacks after real routes so a wrong method on a known API path gets 405 instead of falling through to the single-page app. Those fallbacks remain behind API authentication. Query parameters and path variables are URL encoded normally. JSON request decoders generally reject malformed or trailing JSON; size caps are noted below. Multipart forms must use the exact field names shown.

## Login, user and session routes

| Method and path | Access | Stopped | Request | Success |
| --- | --- | --- | --- | --- |
| `POST /api/login` | Public | No | JSON `{ "username": string, "password": string }` | User object, password omitted; sets session cookie. 400 invalid shape, 401 bad credentials, 429 rate limit. |
| `POST /api/logout` | A/V | No | No body | JSON string; clears session cookie. |
| `GET /api/user/status` | A/V | No | None | Current user object, password omitted. |
| `GET /api/user/list` | A only | No | None | Array of users, password omitted. |
| `POST /api/user/add` | A | No | JSON User fields `username`, `password`, `role` (`admin` or `viewer`), optional `email` | JSON string. Usernames are 1–64 letters/digits/dot/dash/underscore; passwords are 10–72 UTF-8 bytes. |
| `POST /api/user/remove` | A | No | JSON containing `username` | JSON string. 404 missing user; 409 if removing the last administrator. |
| `POST /api/user/password` | A/V (own account only) | No | JSON `{ "old_password": string, "new_password": string, "new_password_confirmation": string }` | JSON `true`; verifies old password and refreshes this session. |

Authentication is also enforced by middleware on the API subrouter, so a viewer cannot use a guessed mutating route even when the UI hides its control. Sessions are read from the authentication cookie and checked against the current stored user/credential version on each request.

## Profiles, saves, world generation and checkpoints

| Method and path | Access | Stopped | Request | Success |
| --- | --- | --- | --- | --- |
| `GET /api/profiles` | A/V | No | None | `ProfileState`: schema version, active profile ID and profiles. |
| `POST /api/profiles` | A | No | JSON `{ "name", "description", "source" }`; source is `clone` or `empty` | `ProfileState` (201). |
| `PATCH /api/profiles/{profile}` | A | No | JSON `{ "name", "description" }` | Updated `ProfileState`. |
| `PUT /api/profiles/{profile}/startup` | A | Yes | JSON `{ "bind_ip", "port", "selected_save" }` | Updated `ProfileState`. |
| `DELETE /api/profiles/{profile}` | A | No | None | Updated `ProfileState`. Active profile cannot be deleted. |
| `POST /api/profiles/{profile}/activate` | A | No | None | Updated `ProfileState`; activation validates/rolls back on failure and leaves Factorio stopped. |
| `GET /api/saves/list` | A/V | No | Optional query `latest=true|false` | Array of save metadata; when requested and available, includes a “Load Latest” entry. |
| `GET /api/saves/dl/{save}` | A/V | No | Path save name | Save archive as `application/octet-stream` attachment. |
| `POST /api/saves/upload` | A | Yes | Multipart file field `savefile`; one `.zip` only | JSON string. File size follows `FSM_MAX_UPLOAD` when configured. |
| `DELETE /api/saves/rm/{save}` | A | Yes | Path save name | JSON string. |
| `POST /api/saves/create/{save}` | A | Yes | Save name in path; `.zip` appended if absent | JSON string. |
| `GET /api/saves/generation/options` | A/V | No | None | `WorldGenerationOptions`: installed version/mode, planets, presets, controls and preview sizes. |
| `POST /api/saves/generation/preview` | A | Yes | JSON `WorldGenerationRequest` (preset, seed; optional planet/name/preview size/dimensions/starting area/enemy options/controls) | PNG image. Headers include `X-Factorio-Planet` and `X-Factorio-Seed`. |
| `POST /api/saves/generation/create` | A | Yes | Same request shape as preview; server validates final-save constraints | Save metadata JSON. |
| `POST /api/saves/mods` | A | Yes | JSON `{ "saveFile": string }` | Decoded Factorio save-header metadata for the named save. |
| `POST /api/saves/mods/import` | A | Yes | JSON `{ "saveFile": string }` | Import result with installed mods and skipped unavailable/mismatched mods. Import is staged; failures preserve the prior mod set and game mode. |
| `GET /api/checkpoints` | A/V | No | None | `CheckpointState`: profile ID, settings, checkpoint metadata and last error/time when present. |
| `PUT /api/checkpoints/settings` | A | No | `CheckpointSettings`: interval enabled/minutes, last-player enabled, clean-stop enabled, retention count | Updated `CheckpointState`. Body cap 8 KiB. |
| `POST /api/checkpoints` | A | No | No body; creates a manual checkpoint | `CheckpointState` (201). Live-save creation uses Factorio/RCON when the server is running. |
| `GET /api/checkpoints/{checkpoint}/download` | A/V | No | Checkpoint ID | ZIP attachment; `Cache-Control: no-store`. |
| `POST /api/checkpoints/{checkpoint}/restore` | A | Yes | Checkpoint ID in path; no body | New save metadata (201); checkpoint itself is retained. |
| `DELETE /api/checkpoints/{checkpoint}` | A | No | Checkpoint ID | Updated `CheckpointState`. |

Profile request bodies are limited to 64 KiB. World generation requests are limited to 1 MiB. Checkpoint settings are limited to 8 KiB. Save upload body cap is configured for file size plus bounded multipart overhead; the handler also validates portable filenames and ZIP activation.

## Server, settings, player, map and release routes

| Method and path | Access | Stopped | Request | Success |
| --- | --- | --- | --- | --- |
| `GET /api/log/tail` | A/V | No | None | JSON-encoded recent log lines. |
| `GET /api/config` | A/V | No | None | Factorio config sections as JSON; viewer responses omit credentials and filesystem-location fields. |
| `POST /api/server/start` | A | Yes | JSON fields `savefile`, `bindip`, `port` | JSON string after the child is observed running. Validates IPv4, port 1–65535 and save. |
| `POST /api/server/stop` | A | No | None | JSON string describing safe-shutdown request; conflicting state returns 409. |
| `POST /api/server/kill` | A | No | None | JSON string; requests immediate process kill. |
| `GET /api/server/status` | A/V | No | None | Server snapshot: save, latency, bind IP, port, running/stopping and versions. |
| `GET /api/server/players` | A/V | No | None | Player overview JSON; bounded recent snapshot history is not live membership. |
| `GET /api/server/autostart` | A/V | No | None | `{ "enabled": boolean }`, no-store. |
| `PUT /api/server/autostart` | A | No | JSON `{ "enabled": boolean }` | Updated autostart object, no-store; setting is manager-wide. Body cap 4 KiB. |
| `GET /api/map-snapshot` | A/V | No | None | Snapshot settings/runtime status and active profile snapshot metadata, if available. |
| `POST /api/map-snapshot/refresh` | A | No | None | Updated snapshot state (202); 409 if disabled/already busy. |
| `PUT /api/map-snapshot/settings` | A | No | JSON with `interval_minutes`, `enabled`, `automatic_only_when_no_players`, `include_space_platforms` | Updated settings/state. Body cap 4 KiB. |
| `GET /api/map-snapshot/surfaces/{surface}` | A/V | No | Surface ID | PNG image with generated timestamp/cache metadata. |
| `GET /api/map-snapshot/surfaces/{surface}/entities` | A/V | No | Surface ID | JSON array of bounded player-force entity footprints; older snapshots may return 404 when details are absent. |
| `GET /api/server/facVersion` | A/V | No | None | JSON `{ "version", "base_mod_version" }`. |
| `POST /api/server/release/install` | A | Yes | JSON `{ "target": string }`; legacy `{ "channel": string }` accepted | JSON target/installed version/message. Requires stopped server and supported target. |
| `GET /api/server/release/status` | A/V | No | None | Release status JSON. Viewer metadata errors are generalized. |
| `GET /api/server/game-mode` | A/V | No | None | Game mode/status and built-in feature flags. |
| `POST /api/server/game-mode` | A | Yes | JSON `{ "mode": "factorio" | "space-age" }` | Updated game-mode status. `custom` is descriptive status, not an accepted update target. |
| `GET /api/settings` | A/V | No | None | Factorio server-settings JSON; viewer response removes sensitive values. |
| `POST /api/settings/update` | A | Yes | JSON object containing Factorio server-settings fields | Saved settings response; active process is locked. |
| `GET /api/backups/limits` | A only | No | None | `{ "max_upload_bytes", "max_file_bytes" }`, no-store. |
| `POST /api/profiles/export` | A | Yes | JSON profile-backup options: `profile_ids`, `include_saves`, `include_checkpoints` | ZIP attachment, no-store. |
| `POST /api/profiles/import/preview` | A | Yes | Multipart file field `backup` containing one ZIP | Preview JSON; does not activate imported profiles. |
| `POST /api/profiles/import` | A | Yes | Multipart file `backup` plus JSON `selection` form field | New `ProfileState` (201). Fresh IDs are assigned. |
| `POST /api/mods/backup/preview` | A | Yes | Multipart file `backup` containing supported mod-only ZIP | Preview JSON. |
| `POST /api/mods/backup/restore` | A | Yes | Multipart file `backup` plus `profile_id` form field | Restore result JSON; active profile is checked again at commit. |

`Stopped` reflects the route catalog flag, not every possible handler-level condition. Some state-changing operations return 409/423 based on a live state check even when that column says `No`. Profile backups require a stopped runtime because they capture profile data. Backup upload defaults to 512 MiB when no cap is configured and has a 16 GiB hard request ceiling; `/api/backups/limits` reports the effective request and file caps.

## Mod Portal and installed-mod routes

| Method and path | Access | Stopped | Request | Success |
| --- | --- | --- | --- | --- |
| `GET /api/mods/portal/preferences` | A/V | No | None | `{ "preselect_optional": boolean }`, no-store. |
| `PUT /api/mods/portal/preferences` | A | No | JSON `{ "preselect_optional": boolean }` | Updated preference; manager-wide; 4 KiB cap. |
| `GET /api/mods/portal/list` | A/V | No | None; server requests the installed Factorio major/minor line | Portal result JSON with `factorio_version`; upstream status is propagated by handler. |
| `GET /api/mods/portal/info/{mod}` | A/V | No | Mod name in path | Portal mod detail JSON. |
| `POST /api/mods/portal/install` | A | Yes | JSON `{ "downloadUrl", "fileName", "modName" }` | Installed-mod summary. Legacy direct-download contract; only trusted official Portal URLs should be used. |
| `POST /api/mods/portal/install/plan` | A | No | `ModInstallPlanRequest` JSON: exact mod/release and reviewed dependency selections | Resolved dependency plan; does not activate it. |
| `POST /api/mods/portal/install/resolved` | A | Yes | Same plan request JSON | Installed-mod result; failure may include `{ "error", "plan" }`. |
| `POST /api/mods/portal/login` | A | No | JSON `{ "username", "password" }` for Factorio Mod Portal | JSON result/status; credentials are stored as Factorio credentials, never manager account credentials. |
| `GET /api/mods/portal/loginstatus` | A/V | No | None | JSON boolean indicating whether Factorio Portal auth is present. |
| `DELETE /api/mods/portal/logout` | A | No | None | JSON boolean; removes stored Portal auth. |
| `POST /api/mods/portal/install/multiple` | A | Yes | JSON array of `{ "name", "version" }`, maximum 128 | Installed-mod summary. |
| `GET /api/mods/list` | A/V | No | None | Installed mod list and enabled states. |
| `POST /api/mods/toggle` | A | Yes | JSON `{ "name": string }` | Handler returns JSON-encoded result; active server is locked. |
| `POST /api/mods/delete` | A | Yes | JSON `{ "name": string }` | JSON-encoded mod name/result. |
| `POST /api/mods/delete/all` | A | Yes | None | JSON `null` on success. |
| `POST /api/mods/update` | A | Yes | JSON `{ "modName", "downloadUrl", "fileName" }` | Updated mod entry or 404 if not found afterward. |
| `POST /api/mods/upload` | A | Yes | Multipart `mod_file`; ZIP, `mod-settings.dat`, or `mod-list.json` | Installed mod list. Upload is bounded by configured max. |
| `GET /api/mods/download` | A/V | No | None | ZIP attachment of installed mods. |
| `GET /api/mods/startup-settings` | A only | Yes | None | Evaluated startup setting schema/value groups; no-store. |
| `PATCH /api/mods/startup-settings` | A | Yes | `{ "revision": string, "changes": [{ "name": string, "value": JSON value }] }` | Updated evaluated settings; stale revision 409; invalid/unsupported 422; operation/server lock 423. |

## Mod-pack routes

| Method and path | Access | Stopped | Request | Success |
| --- | --- | --- | --- | --- |
| `GET /api/mods/packs/list` | A/V | No | None | Mod-pack list. |
| `POST /api/mods/packs/create` | A | No | JSON `{ "name": string }` | Updated pack list. |
| `POST /api/mods/packs/{modpack}/delete` | A | No | Pack name in path | JSON-encoded name/result. |
| `GET /api/mods/packs/{modpack}/download` | A/V | No | Pack name in path | ZIP attachment. |
| `POST /api/mods/packs/{modpack}/load` | A | Yes | Pack name in path; no body | Installed-mod result. This is the replace-style load. |
| `GET /api/mods/packs/{modpack}/list` | A/V | No | Pack name in path | Installed mods in that pack. |
| `POST /api/mods/packs/{modpack}/mod/toggle` | A | No | JSON `{ "name": string }` | Updated pack mod list. |
| `POST /api/mods/packs/{modpack}/mod/delete` | A | No | JSON `{ "name": string }` | JSON `true` on success. |
| `POST /api/mods/packs/{modpack}/mod/delete/all` | A | No | Pack name in path; no body | JSON `true` on success; empties/recreates the pack. |
| `POST /api/mods/packs/{modpack}/mod/update` | A | No | JSON `{ "modName", "downloadUrl", "filename" }` | Updated mod entry. |
| `POST /api/mods/packs/{modpack}/mod/upload` | A | No | Multipart `mod_file` ZIP | Updated mod list; bounded by configured upload size. |
| `POST /api/mods/packs/{modpack}/portal/install` | A | No | JSON `{ "downloadUrl", "fileName", "modName" }` | Updated mod list. |
| `POST /api/mods/packs/{modpack}/portal/install/multiple` | A | No | JSON array of `{ "name", "version" }` | Updated mod list. |
| `POST /api/mods/packs/{modpack}/merge/preview` | A | Yes | Pack name in path; no body | Merge plan/revision JSON; preview validates compatibility without activation. |
| `POST /api/mods/packs/{modpack}/merge` | A | Yes | JSON `{ "revision": string }` returned by preview | `{ "error", "plan" }` on domain failure; commits only if revision is still current. |

Mod-pack mutation routes without the generic `Stopped` flag may still be constrained by a handler or operation-level lock. Preview/merge are explicitly stopped-only. For the additive merge workflow, preserve the current mod/settings state and use the revision returned by preview; do not construct a revision or Portal download URL in the browser.

## WebSocket `/ws`

`GET /ws` is an authenticated WebSocket upgrade. It uses the same current-session validation as REST. The server checks the account again for privileged commands so downgrading/deleting a user or changing their password revokes command capability on an already-open socket.

Client control frames are JSON in this shape:

```json
{
  "room_name": "",
  "controls": { "type": "subscribe", "value": "server_status" }
}
```

Supported control `type` values are `subscribe`, `unsubscribe` and `command`. Allowed room names are `server_status` and `gamelog`. Both roles may subscribe to `server_status`; only a currently valid administrator may subscribe to `gamelog` because it can contain RCON output. `command` is administrator-only and sends `controls.value` to the running server's persistent RCON connection. Commands are not queued for reconnect/replay. Unknown rooms/controls are ignored. Client messages are limited to 2,048 bytes.

Server event frames have shape `{ "room_name": "server_status|gamelog", "message": ... }`. A status message is the server snapshot/event payload. A game-log message is text; a new `gamelog` subscriber receives the bounded in-memory log cache before live messages. The browser client reconnects and restores subscriptions, but does not replay commands. Ping/pong keepalive and read/write deadlines are enforced by the WebSocket pumps.
