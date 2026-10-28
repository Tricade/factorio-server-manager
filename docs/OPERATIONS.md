# Operating Factorio Server Control

One manager owns one playable Factorio server. Profiles store alternative setups; activating one changes the stopped runtime, not the number of running servers. Keep the manager on a trusted network or behind authenticated, correctly configured HTTPS access. Deployment and migration details are in the [Docker guide](../docker/README.md).

## Scope and permissions

| Operation | Scope | Required state |
| --- | --- | --- |
| Start, stop, force stop and console | Active profile/runtime | Administrator; start requires stopped state, console requires a ready live connection |
| Upload, create or delete a save | Active profile | Administrator and stopped server; downloads remain available while running |
| Change installed mods, startup settings, game version or mode | Active profile | Administrator and stopped server |
| View Factorio process configuration | Active profile | Read-only; multiplayer rules are edited under Server settings |
| Create, clone, activate or delete profiles | Profile library | Administrator and stopped runtime; these are not background parallel servers |
| Create a checkpoint | Active profile | Administrator; a running server must be ready to save through RCON |
| Restore a checkpoint | Active profile | Administrator and stopped server; creates a new save, never overwrites the checkpoint |
| Map rendering | Profile snapshot with manager-wide scheduling | Administrator to generate/configure; can run alongside a game in an isolated workspace |
| Users, portal credentials, reusable packs and autostart | Manager-wide | Administrator, except each user may change their own password |

Viewer accounts can inspect non-secret server data and download authorized files. Download access can still expose player names, save contents or mod data. It is not an anonymous public spectator role. Startup-setting values and manager users are administrator-only. Unknown server status is not permission to mutate data; wait for reconnection and a confirmed stopped state.

New-world creation appears only for an empty profile. For another world, create another profile. Game settings is deliberately read-only; Server settings contains editable multiplayer rules. Factorio's rotating autosaves, retained checkpoints and portable profile exports solve different problems and are not interchangeable.

## Storage and recovery

Persist `/opt/fsm-data` **and** either the complete `/opt/factorio` directory or all three split saves/mods/config directories. Do not overlap the layouts or share the same paths between concurrent managers. Profile archives intentionally duplicate game data for switching and rollback, so allow space for active files, inactive profiles and temporary export/staging files.

For a complete recoverable backup:

1. Save and stop the game, then stop the manager container.
2. Back up manager storage and the selected Factorio data layout together. Keep the image version/digest with the backup.
3. Restore to separate test paths and verify login, profile selection, mods, exact Factorio version and save startup before replacing the original installation.

Portable profile exports remove known manager/server credentials and import as new inactive profiles. They do not contain manager accounts, global configuration, engine files or reusable global mod packs. Saves and opaque mod settings are not anonymized. A backup is not verified until it has been restored and checked.

The default upload limit is 512 MiB. `FSM_MAX_UPLOAD` changes the container limit in MiB; recreate the container to apply it. Backup forms query the effective cap before sending data. Reverse proxies can impose a smaller limit, and ZIP validation also enforces expanded-size and entry-count bounds. Prefer separate profile exports near the limit; there is no need to increase every installation's limit for one oversized archive.

## Troubleshooting

If a Factorio release replacement reports an incomplete rollback, keep the server stopped and back up the whole game data root and manager state. The error names the retained `.factorio-release-backup-*` recovery directory inside the Factorio root. Do not delete it or retry until the old program entries and runtime metadata have been reconciled; the next replacement refuses to consume retained backups as ordinary program files. Preserve the reported error and directory contents for recovery. A completed rollback removes its temporary backup normally; this rule concerns unresolved recovery state, including failed backup cleanup.

### Recovering a refused legacy migration

Back up the configuration, account databases and relevant game/mod-pack storage before manually resolving a migration. Stop the manager first and inspect a disposable copy; do not delete a database, force an overwrite or rename a persistence mount to make an error disappear.

- Historical LevelDB accounts are migrated as one SQLite transaction. The original LevelDB directory is retained even after success because the later configuration write is a separate operation. A verified retry preserves already migrated passwords. A conflicting existing SQLite account or missing/malformed credential proof is a real conflict: retain both databases and reconcile them deliberately. Do not expose `migrated-user-passwords.jsonl`; it contains private one-time credentials and may include entries from an unsuccessful attempt. Use it only after the migration has completed successfully.
- Old flat archives under the Factorio `modpacks/` directory are converted into the configured reusable pack root. A successful conversion retains its input as `name.zip.migrated`; no unrelated files or entire legacy directory are deleted. It is not re-imported on each restart.
- `name.zip.migrating` records an interrupted operation. If no destination exists, startup can validate and resume it. If both it and a destination exist, or a source/recovery path conflicts, startup refuses to guess which data to replace. Back up both, inspect the destination and archive offline, then move the unneeded recovery input out of the active legacy directory or restore the intended source after resolving the collision. Preserve a verified copy before doing either.
- The no-overwrite source-tracking step requires same-directory hard-link support. If that filesystem cannot provide it, the error leaves the input intact. Convert a backed-up copy on a supported disposable filesystem or move the legacy inputs out of automatic startup processing and rebuild the reusable pack deliberately; do not disable validation.

Initialization failures exit nonzero and log the failed stage so a supervisor can distinguish failed startup from a normal exit. Bad server settings must be a single non-null JSON object; an admin list must be an array of names. Repair a backed-up copy rather than replacing it with an empty object just to get a green status.

| Symptom | Check first | Safe next action |
| --- | --- | --- |
| Password changes after replacing the container | `/opt/fsm-data` mapping and existing SQLite database | Restore the intended mount; do not delete the database to bypass login |
| Login succeeds but returns to the login page | HTTP versus HTTPS and secure-cookie configuration | Use HTTPS with secure cookies, or private loopback/LAN HTTP with the documented direct-access setting |
| Controls are disabled | Viewer role, running/stopping state, unavailable status or active operation | Wait for completion/reconnection; do not bypass the API guard |
| Profile data cannot load | Manager connection and response error | Retry; never treat missing profile context as global data |
| Console reconnects or logs stop updating | Reverse proxy WebSocket support and manager reachability | Check `/ws` upgrade forwarding; subscriptions reconnect automatically, commands are never replayed |
| Mod startup settings fail after an update | Enabled mod compatibility and the exact engine version | Retry after fixing/updating the mod; keep the previous settings file intact |
| Uploaded archive is rejected | Effective upload cap, proxy cap, free temporary disk space and ZIP validity | Export smaller per-profile backups; do not disable archive validation |
| Split-layout recreation cannot start | Access to official Factorio downloads | Restore connectivity so the recorded exact version can be installed; do not silently switch channels |
| Map generation is slow or memory-intensive | Generation mode, interval, online players and platforms | Choose manual-only/disabled, increase the interval or exclude platforms; the last completed snapshot is retained |
| Force stop seems necessary | Whether a clean save/stop is still progressing | Allow the configured shutdown grace period; force stop may lose unsaved progress |

Collect the UI version/revision, exact Factorio version, relevant request/status and redacted manager/container logs for a bug report. Do not post credentials, cookies, `conf.json`, `factorio.auth`, complete backups or private saves publicly. Screenshots also need review for player names and private server details.

## API integration rules

Use authenticated same-origin requests and the exact methods in the [HTTP/WebSocket API reference](API.md). Mutations require an administrator unless explicitly documented otherwise. Common controls are `POST /api/server/start`, `POST /api/server/stop`, `POST /api/server/kill` and `DELETE /api/saves/rm/{save}`. Save names and profile IDs must be URL-encoded path components, not paths supplied by clients.

Handle `401` as an expired/invalid login, `403` as insufficient permission, `405` as the wrong HTTP method, `409` as a conflict/stale operation, `413` as an upload limit, and `423` as a stopped-server requirement. Some older handlers return JSON strings while newer endpoints return structured JSON or plain error text; check status and content type rather than assuming every response is a success object. Retry reads where appropriate, but never automatically replay a potentially completed mutation.

The legacy `POST /api/saves/create/{save}` route remains available and uses the guarded world generator. It does not overwrite an existing save and no longer returns raw child-process output.
