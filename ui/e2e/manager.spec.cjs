const {test, expect} = require("@playwright/test");

test.beforeEach(async ({page}) => {
    page.testFailures = [];
    page.on("pageerror", error => {
        if (!page.expectChunkFailure || !/chunk/i.test(error.message)) page.testFailures.push(error.message);
    });
    page.on("response", response => {
        if (response.status() === 501) page.testFailures.push(`Missing fixture: ${response.url()}`);
    });
    await page.route(/^https?:\/\//, route => {
        if (new URL(route.request().url()).origin === "http://127.0.0.1:18089") return route.continue();
        page.testFailures.push(`Unexpected external asset: ${route.request().url()}`);
        return route.abort();
    });
    // No real socket or console commands, even when a scenario says running.
    await page.routeWebSocket("**/ws", () => {});
});

test.afterEach(async ({page}) => {
    expect(page.testFailures).toEqual([]);
});

async function navigate(page, label) {
    const nav = page.getByRole("navigation", {name: "Primary navigation"});
    if (!(await nav.isVisible())) await page.getByRole("button", {name: "Open navigation", exact: true}).click();
    await nav.getByRole("link", {name: label, exact: true}).click();
}

async function disableNativePointerFocus(page) {
    // Safari does not focus buttons on pointer activation. Reproduce that
    // behavior on every engine so focus restoration cannot pass by accident.
    await page.addInitScript(() => document.addEventListener("mousedown", event => {
        if (event.target.closest("button")) event.preventDefault();
    }));
}

const routes = [
    ["/", "Overview"], ["/saves", "Saves & checkpoints"], ["/mods", "Mods & mod packs"],
    ["/server-settings", "Server settings"], ["/game-settings", "Game settings"],
    ["/console", "Console"], ["/logs", "Factorio logs"], ["/releases", "Version & mode"],
    ["/profiles", "Profiles"], ["/user-management", "Users & access"]
];

test("every authenticated page opens directly with its own lazy assets", async ({page}) => {
    for (const [url, title] of routes) {
        const response = await page.goto(url);
        expect(response.headers()["cache-control"]).toContain("must-revalidate");
        await expect(page.getByRole("heading", {level: 1, name: title, exact: true})).toBeVisible();
        await expect(page.getByRole("status", {name: "Loading page…"})).toHaveCount(0);
        await expect(page.getByText("This page could not be loaded.", {exact: false})).toHaveCount(0);
        const layout = await page.evaluate(() => ({
            width: document.documentElement.scrollWidth, viewport: window.innerWidth,
            overflow: [...document.querySelectorAll("main *")].filter(el => el.getBoundingClientRect().right > window.innerWidth + 1)
                .slice(0, 8).map(el => ({tag: el.tagName, class: el.className, right: el.getBoundingClientRect().right}))
        }));
        expect(layout.width, `${url}: ${JSON.stringify(layout)}`).toBeLessThanOrEqual(layout.viewport);
        await expect.poll(() => page.locator(".ui-brand-mark img").first().evaluate(img => img.complete && img.naturalWidth > 0)).toBe(true);
    }
});

test("navigation loads only the requested page and preserves the dark palette", async ({page}) => {
    const chunks = [];
    page.on("request", request => { if (request.url().includes("/chunks/")) chunks.push(request.url()); });
    await page.goto("/");
    await expect(page.getByRole("heading", {level: 1, name: "Overview"})).toBeVisible();
    expect(chunks.some(url => /\/overview\./.test(url))).toBe(true);
    expect(chunks.some(url => /\/mods\./.test(url))).toBe(false);
    await navigate(page, "Mods");
    await expect(page.getByRole("heading", {level: 1, name: "Mods & mod packs"})).toBeVisible();
    expect(chunks.some(url => /\/mods\./.test(url))).toBe(true);
    await expect(page.locator(".ui-profile-switcher__icon")).toHaveCSS("color", "rgb(244, 163, 64)");
    await page.goBack();
    await expect(page.getByRole("heading", {level: 1, name: "Overview"})).toBeVisible();
});

test("help text remains inside the viewport when focused on a narrow screen", async ({page}) => {
    await page.goto("/mods");
    await expect(page.getByText("No mods in this set", {exact: true})).toBeVisible();
    const help = page.getByRole("button", {name: "Mods & mod packs help"});
    await help.focus();
    const tooltip = page.getByRole("tooltip");
    await expect(tooltip).toBeVisible();
    const bounds = await tooltip.boundingBox();
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(page.viewportSize().width);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("a missing page chunk retains navigation and offers an explicit reload", async ({page}) => {
    page.expectChunkFailure = true;
    await page.route("**/chunks/logs.*.js", route => route.fulfill({status: 404, body: ""}));
    await page.goto("/logs");
    await expect(page.getByRole("button", {name: "Reload page"})).toBeVisible();
    await navigate(page, "Saves & Checkpoints");
    await expect(page.getByRole("heading", {level: 1, name: "Saves & checkpoints"})).toBeVisible();
    await page.unroute("**/chunks/logs.*.js");
    await navigate(page, "Logs");
    await page.getByRole("button", {name: "Reload page"}).click();
    await expect(page.getByRole("heading", {level: 1, name: "Factorio logs"})).toBeVisible();
    await expect(page.getByText("Synthetic test log", {exact: false})).toBeVisible();
});

test("running and unknown process states prevent destructive edits", async ({page}) => {
    for (const scenario of ["running", "unknown"]) {
        await page.setExtraHTTPHeaders({"X-FSM-Test-Scenario": scenario});
        await page.goto("/saves");
        await expect(page.getByRole("button", {name: "Delete demo.zip"})).toBeDisabled();
        await expect(page.locator('input[type="file"]')).toBeDisabled();
        await expect(page.getByRole("link", {name: "Download demo.zip"})).toBeVisible();
        await navigate(page, "Server settings");
        await expect(page.getByRole("textbox", {name: "Name", exact: true})).toBeDisabled();
        await navigate(page, "Version & mode");
        await expect(page.getByRole("radio", {name: /Experimental/})).toBeDisabled();
        await expect(page.getByRole("radio", {name: /Experimental/})).toBeChecked();
    }
});

test("viewer pages do not expose mutations or the admin console", async ({page}) => {
    await page.setExtraHTTPHeaders({"X-FSM-Test-Scenario": "viewer"});
    await page.goto("/saves");
    await expect(page.getByRole("link", {name: "Download demo.zip"})).toBeVisible();
    await expect(page.getByRole("button", {name: "Delete demo.zip"})).toHaveCount(0);
    await page.goto("/console");
    await expect(page.getByRole("heading", {level: 1, name: "Overview"})).toBeVisible();
    await page.goto("/user-management");
    await expect(page.getByRole("heading", {level: 1, name: "Account & access"})).toBeVisible();
    await expect(page.getByRole("heading", {name: "Create user"})).toHaveCount(0);
});

test("failed save deletion keeps the save and its confirmation recoverable", async ({page}) => {
    await disableNativePointerFocus(page);
    await page.goto("/saves");
    const remove = page.getByRole("button", {name: "Delete demo.zip"});
    await remove.click();
    const dialog = page.getByRole("dialog", {name: "Delete save?"});
    await dialog.getByRole("button", {name: "Confirm", exact: true}).click();
    await expect(page.getByText("Synthetic operation failed; existing data was kept.").first()).toBeVisible();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", {name: "Cancel"}).click();
    await expect(dialog).toHaveCount(0);
    await expect(remove).toBeFocused();
    await expect(page.getByRole("link", {name: "Download demo.zip"})).toBeVisible();
});

test("a dismissed mod-pack dialog restores focus without native pointer focus", async ({page}) => {
    await disableNativePointerFocus(page);
    await page.goto("/mods");
    const create = page.getByRole("button", {name: "Save current mod set", exact: true});
    await create.click();
    const dialog = page.getByRole("dialog", {name: "Create mod pack"});
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(create).toBeFocused();
});

test("failed settings save preserves the draft", async ({page}) => {
    await page.goto("/server-settings");
    const name = page.getByRole("textbox", {name: "Name", exact: true});
    await expect(name).toBeEnabled();
    await name.fill("Unsaved browser test");
    await page.getByRole("button", {name: "Save changes", exact: true}).click();
    await expect(page.getByText("Synthetic operation failed; existing data was kept.").first()).toBeVisible();
    await expect(name).toHaveValue("Unsaved browser test");
    await expect(page.getByRole("button", {name: "Discard changes", exact: true})).toBeEnabled();
});

test("nested API failures use safe toast text and keep the application shell usable", async ({page}) => {
    const privateDetail = "synthetic-private-backend-detail";
    await page.route("**/api/settings/update", route => route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({error: {message: privateDetail, details: privateDetail}})
    }));
    await page.goto("/server-settings");
    const name = page.getByRole("textbox", {name: "Name", exact: true});
    await expect(name).toBeEnabled();
    await name.fill("Structured error browser test");
    await page.getByRole("button", {name: "Save changes", exact: true}).click();

    await expect(page.locator(".ui-toast")).toContainText("The request could not be completed.");
    await expect(page.getByRole("heading", {level: 1, name: "Server settings"})).toBeVisible();
    await expect(name).toHaveValue("Structured error browser test");
    await expect(page.locator("body")).not.toContainText(privateDetail);
});

test("a successful mod upload with a failed list refresh keeps the tab mounted and offers retry", async ({page}) => {
    let uploadRequestStarted = false;
    let uploadCommittedResolve;
    const uploadCommitted = new Promise(resolve => { uploadCommittedResolve = resolve; });
    let refreshFailureUsed = false;
    await page.route("**/api/mods/upload", async route => {
        uploadRequestStarted = true;
        await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({uploaded: true})
        });
        uploadCommittedResolve();
    });
    await page.route("**/api/mods/list", async route => {
        if (uploadRequestStarted) {
            // A refresh request can be observed as soon as the POST response
            // is delivered. Wait for its successful commit before failing one.
            await uploadCommitted;
            if (!refreshFailureUsed) {
                refreshFailureUsed = true;
                return route.fulfill({status: 503, contentType: "text/plain", body: "Synthetic list refresh unavailable"});
            }
        }
        return route.continue();
    });
    await page.goto("/mods");
    await expect(page.getByRole("heading", {level: 1, name: "Mods & mod packs"})).toBeVisible();
    await expect(page.locator(".ui-profile-switcher strong")).toHaveText("Demo factory");
    await expect(page.getByText("No mods in this set", {exact: true})).toBeVisible();
    const uploadTab = page.getByRole("tab", {name: "Upload archive"});
    await uploadTab.click();
    const file = page.locator('input[type="file"]');
    await file.setInputFiles({name: "synthetic-mod.zip", mimeType: "application/zip", buffer: Buffer.from("synthetic archive")});
    await page.getByRole("button", {name: "Upload mod", exact: true}).click();

    await expect(page.locator(".ui-toast")).toContainText("The change was saved, but the latest mod data could not be loaded.");
    await expect(page.getByText("Installed mods could not be refreshed. Use Retry to load the latest list.", {exact: true})).toBeVisible();
    await expect(page.getByRole("button", {name: "Retry", exact: true})).toBeEnabled();
    await expect(uploadTab).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toContainText("Choose a mod archive…");
    await expect(file).toBeDisabled();
    await expect(page.getByRole("button", {name: "Upload mod", exact: true})).toBeDisabled();
    if (process.env.FSM_CAPTURE_MODS_RETRY_SCREENSHOT === "1") {
        await page.screenshot({path: "screenshots/mods-retry.png"});
    }

    await page.getByRole("button", {name: "Retry", exact: true}).click();
    await expect(page.getByText("Installed mods could not be refreshed. Use Retry to load the latest list.", {exact: true})).toHaveCount(0);
    await expect(uploadTab).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tabpanel")).toContainText("Choose a mod archive…");
    await expect(file).toBeEnabled();
    await expect(page.getByRole("button", {name: "Upload mod", exact: true})).toBeEnabled();
});

test("a failed log request can be retried without reloading the application", async ({page}) => {
    await page.route("**/api/log/tail**", route => route.fulfill({status: 503, contentType: "text/plain", body: "Unavailable"}));
    await page.goto("/logs");
    await expect(page.getByRole("alert")).toContainText("Factorio logs could not be loaded");
    await page.unroute("**/api/log/tail**");
    await page.getByRole("button", {name: "Refresh", exact: true}).click();
    await expect(page.getByText("Synthetic test log", {exact: false})).toBeVisible();
});

test("profile picker works from the keyboard and links to new profile setup", async ({page}) => {
    await disableNativePointerFocus(page);
    await page.goto("/");
    await expect(page.getByRole("heading", {level: 1, name: "Overview"})).toBeVisible();
    const openNavigation = page.getByRole("button", {name: "Open navigation", exact: true});
    const compact = await openNavigation.isVisible();
    if (compact) {
        await openNavigation.click();
        await expect(page.getByRole("button", {name: "Close navigation", exact: true})).toBeFocused();
    }
    const picker = page.getByRole("button", {name: /Active profile Demo factory/});
    await picker.focus();
    await page.keyboard.press("Enter");
    await expect(picker).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Escape");
    await expect(picker).toHaveAttribute("aria-expanded", "false");
    await expect(picker).toBeFocused();
    if (compact) {
        await page.keyboard.press("Escape");
        await expect(openNavigation).toBeFocused();
        await openNavigation.click();
        await expect(page.getByRole("button", {name: "Close navigation", exact: true})).toBeFocused();
    }
    await picker.focus();
    await page.keyboard.press("Enter");
    await expect(picker).toHaveAttribute("aria-expanded", "true");
    await page.getByRole("link", {name: "New profile", exact: true}).click();
    await expect(page).toHaveURL(/\/profiles\?new=fresh$/);
    await expect(page.getByRole("heading", {level: 1, name: "Profiles"})).toBeVisible();
});

test("anonymous deep links lead to login without loading protected page chunks", async ({page}) => {
    await page.setExtraHTTPHeaders({"X-FSM-Test-Scenario": "anonymous"});
    const chunks = [];
    page.on("request", request => { if (request.url().includes("/chunks/")) chunks.push(request.url()); });
    await page.goto("/mods");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.locator('input[type="password"]')).toBeVisible();
    expect(chunks).toEqual([]);
});
