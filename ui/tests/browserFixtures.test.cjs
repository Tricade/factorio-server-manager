const {test} = require("node:test");
const assert = require("node:assert/strict");
const loadModule = require("./helpers/loadModule.cjs");
const browserConfig = require("../../playwright.config.cjs");

function pageFixture() {
    let fixture;
    loadModule("e2e/fixtures.cjs", {
        "@playwright/test": {
            test: {extend: fixtures => { fixture = fixtures.page; return {}; }},
            expect: {}
        }
    });
    return fixture;
}

test("browser cold start has an isolated budget without loosening behavior tests", async () => {
    const [run, options] = pageFixture();
    assert.equal(options.timeout, 60000);
    assert.equal(browserConfig.timeout, 30000);
    assert.equal(browserConfig.retries, 0);
    const events = [];
    const page = {close: async () => events.push("closed")};
    await run({context: {newPage: async () => { events.push("created"); return page; }}}, async value => {
        assert.equal(value, page);
        events.push("used");
    });
    assert.deepEqual(events, ["created", "used", "closed"]);
});

test("browser fixture closes its page when the test fails", async () => {
    const [run] = pageFixture();
    const failure = new Error("Synthetic assertion failure");
    let closed = false;
    await assert.rejects(run({context: {newPage: async () => ({close: async () => { closed = true; }})}},
        async () => { throw failure; }), error => error === failure);
    assert.equal(closed, true);
});

test("browser startup failures propagate without executing test actions", async () => {
    const [run] = pageFixture();
    const failure = new Error("Synthetic browser startup failure");
    let used = false;
    await assert.rejects(run({context: {newPage: async () => { throw failure; }}},
        async () => { used = true; }), error => error === failure);
    assert.equal(used, false);
});
