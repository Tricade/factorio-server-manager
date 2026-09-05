# Web interface redesign draft

This draft builds on version 0.18.0 and reorganizes the web interface as an industrial factory control room. It is intended for visual review before a release.

## Design direction

- Steel-green surfaces, copper edges, amber controls, squared corners and strong headings give the interface an industrial Factorio character. A subtle construction grid sits behind the workspace.
- The factory map is the first major content on Overview. Surface selection and snapshot generation sit above it; snapshot metadata sits below it. Zoom, panning and fullscreen use the existing viewer.
- The selected world, save metrics and runtime information share a side column. They stack after the map on small screens. Players follow the workspace, and a compact quick-access row replaces the separate Operations card.
- Installed mods are the primary content on the Mods page. The Add mods section expands to reveal the existing portal, upload and save-import tabs. Collapsing it preserves the mounted form state.
- Profiles appear as a responsive library grid with two columns on larger screens. Active-profile indicators and existing edit, activate and delete actions remain visible.
- Navigation labels, exact installed version, persistent process controls and manager/profile scope remain explicit. Mobile layouts retain status text, touch targets and keyboard focus indicators.

## Preview

These previews use synthetic local fixture data, not a connected Factorio server. They demonstrate the actual React interface and production stylesheet. The draft does not add a demo mode to the deployed application.

### Desktop overview

![Industrial factory overview draft](../screenshots/Redesign_Overview.png)

### Mobile overview

![Mobile overview draft](../screenshots/Redesign_Mobile.png)

### Mod management

![Mod management draft](../screenshots/Redesign_Mods.png)

### Profiles

![Profile library draft](../screenshots/Redesign_Profiles.png)

## Scope and review

The existing shared API client, profile context, routes and administrator/viewer boundary remain in use. Process lifecycle, profile switching, persistence, map generation and backend APIs retain their existing behavior. No production dependencies, external fonts, telemetry, deployment options or data migrations are introduced.

Review the map prominence, industrial styling and everyday navigation before promoting the draft. Screenshots in the main README remain the released UI for comparison.

## Browser regression check

The standalone check serves the built application on an ephemeral loopback port with synthetic read-only API fixtures. It verifies hidden-navigation focus behavior and Escape handling at 1024, 1050 and 1099 pixels, desktop navigation at 1100 pixels, named process controls with 40-pixel touch targets at 390 pixels, and mobile Mods overflow. It also checks keyboard expansion of Add mods, form-state retention across collapse/reopen, and viewer/running-server restrictions. It rejects outbound browser requests and never connects to a real Factorio server.

After the standard `npm ci`, `npm test` and `npm run build` checks, run from the repository root:

```sh
npm install --prefix build/ui-browser-check --no-save --package-lock=false playwright@1.63.0
node build/ui-browser-check/node_modules/playwright/cli.js install chromium
node scripts/verify-ui-layout.cjs build/ui-browser-check/node_modules/playwright
```

On Windows, use `npm.cmd` if PowerShell blocks the npm script shim. To use an existing Chromium installation, set `BROWSER_EXECUTABLE_PATH` to its executable and omit the browser installation command. Playwright is isolated in the ignored build directory and is not a project or production dependency.
