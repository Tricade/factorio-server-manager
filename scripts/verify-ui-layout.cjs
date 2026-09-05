// Run after npm run build. Install Playwright in an ignored build directory;
// pass its module directory as the first argument (resolved from the current directory).
// BROWSER_EXECUTABLE_PATH optionally selects an already installed Chromium binary.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const {chromium} = require(process.argv[2] ? path.resolve(process.argv[2]) : 'playwright');
const appRoot = path.resolve(__dirname, '../app');
const profile = {
    id: 'layout-check', name: 'Layout check', active: true, game_mode: 'factorio',
    installed_version: '2.0.72', release_target: '2.0.72', selected_save: 'Example.zip',
    save_count: 1, mod_count: 0, bind_ip: '0.0.0.0', port: 34197
};
const runningStatus = {running: true, stopping: false, fac_version: '2.0.72', savefile: 'Example.zip'};
const fixtures = {
    '/api/user/status': {username: 'operator', role: 'admin'},
    '/api/profiles': {active_profile_id: profile.id, profiles: [profile]},
    '/api/server/status': runningStatus,
    '/api/saves/list': [{name: 'Example.zip', size: 1024, last_mod: '2026-01-01T00:00:00Z'}],
    '/api/checkpoints': {checkpoints: []},
    '/api/map-snapshot': {running: false, settings: {enabled: false}, snapshot: null},
    '/api/server/players': {profile_id: profile.id, server_running: true, live_available: true,
        online_players: [], online_count: 0, players: []}
};
const server = http.createServer((request, response) => {
    const pathname = new URL(request.url, 'http://localhost').pathname;
    if (request.method !== 'GET') {response.writeHead(405); response.end(); return;}
    if (pathname.startsWith('/api/')) {
        response.writeHead(Object.hasOwn(fixtures, pathname) ? 200 : 404, {'Content-Type': 'application/json'});
        response.end(JSON.stringify(fixtures[pathname] ?? {error: 'No test fixture for this route.'}));
        return;
    }
    const candidate = path.resolve(appRoot, '.' + decodeURIComponent(pathname));
    if (candidate !== appRoot && !candidate.startsWith(appRoot + path.sep)) {
        response.writeHead(400); response.end(); return;
    }
    const file = fs.existsSync(candidate) && fs.statSync(candidate).isFile() ? candidate : path.join(appRoot, 'index.html');
    if (!fs.existsSync(file)) {response.writeHead(503); response.end('Run npm run build first.'); return;}
    const contentType = {'.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css',
        '.png': 'image/png', '.ico': 'image/x-icon'}[path.extname(file)] || 'application/octet-stream';
    response.writeHead(200, {'Content-Type': contentType});
    fs.createReadStream(file).pipe(response);
});

(async () => {
    let browser;
    try {
        await new Promise((resolve, reject) => {
            server.once('error', reject);
            server.listen(0, '127.0.0.1', resolve);
        });
        browser = await chromium.launch({headless: true,
            ...(process.env.BROWSER_EXECUTABLE_PATH ? {executablePath: process.env.BROWSER_EXECUTABLE_PATH} : {})});
        const origin = `http://127.0.0.1:${server.address().port}`;
        const page = await browser.newPage({viewport: {width: 1050, height: 900}, reducedMotion: 'reduce'});
        const pageErrors = [];
        const failures = [];
        page.on('pageerror', error => pageErrors.push(error.message));
        page.on('response', response => {if (response.status() >= 400) failures.push(`${response.status()} ${response.url()}`);});
        await page.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
        await page.routeWebSocket('**/ws', socket => {
            socket.onMessage(() => socket.send(JSON.stringify({room_name: 'server_status', message: runningStatus})));
        });
        await page.goto(origin);
        await page.getByRole('heading', {name: 'Overview', exact: true}).waitFor();
        // Wait for real profile and running-process data, not the initial loading shell.
        await page.locator('.ui-context-action-label').filter({hasText: 'Save & stop'}).waitFor({state: 'attached'});
        await page.waitForLoadState('networkidle');
        for (const width of [1024, 1050, 1099]) {
            await page.setViewportSize({width, height: 900});
            const menu = page.getByRole('button', {name: 'Open navigation', exact: true});
            await menu.waitFor({state: 'visible'});
            assert.equal(await page.locator('.ui-sidebar').evaluate(element => element.inert), true,
                `${width}px: the closed mobile sidebar must not accept keyboard focus`);
            await menu.click();
            await page.waitForFunction(() => document.querySelector('.ui-sidebar')?.classList.contains('is-open'));
            assert.equal(await page.locator('.ui-sidebar').evaluate(element => element.inert), false,
                `${width}px: opening navigation must enable its controls`);
            await page.keyboard.press('Escape');
            await page.waitForFunction(() => !document.querySelector('.ui-sidebar')?.classList.contains('is-open'), null, {timeout: 3000});
            console.log(`PASS ${width}px: closed navigation is inert; Escape closes the drawer.`);
        }
        await page.setViewportSize({width: 1100, height: 900});
        await page.waitForFunction(() => !document.querySelector('.ui-sidebar')?.inert);
        assert.equal(await page.getByRole('button', {name: 'Open navigation', exact: true}).isVisible(), false,
            '1100px: desktop navigation must be available without the mobile toggle');
        console.log('PASS 1100px: desktop navigation remains keyboard accessible.');
        await page.setViewportSize({width: 390, height: 844});
        for (const label of ['Save & stop', 'Force stop']) {
            const button = page.getByRole('button', {name: label, exact: true});
            await button.waitFor({state: 'visible', timeout: 3000});
            assert.equal(await button.isEnabled(), true, `${label} should be enabled for the running fixture`);
            const bounds = await button.boundingBox();
            assert.ok(bounds.width >= 40 && bounds.height >= 40, `${label} must retain a 40px mobile target`);
        }
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true,
            '390px: the page must not overflow horizontally');
        assert.deepEqual(pageErrors, [], 'The rendered app must not throw browser errors');
        assert.deepEqual(failures, [], 'All application fixture requests must succeed');
        console.log('PASS 390px: both process controls retain accessible names and mobile targets; no page overflow.');
    } finally {
        if (browser) await browser.close();
        server.closeAllConnections();
        await new Promise(resolve => server.close(resolve));
    }
})().catch(error => {console.error(error); process.exitCode = 1;});
