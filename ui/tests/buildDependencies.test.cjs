const test = require("node:test");
const assert = require("node:assert/strict");
const {createRequire} = require("node:module");

// Exercise the URI parser actually used by the build tools' AJV dependency.
const uri = createRequire(require.resolve("ajv"))("fast-uri");

test("build dependency normalizes percent-encoded host case consistently", () => {
    // Regression for GHSA-hrr3-gc8f-f4qj: scheme-relative hosts must agree.
    for (const reference of ["//a.com", "//A.com", "//%41.com", "//%61.com"]) {
        assert.equal(uri.parse(reference).host, "a.com", reference);
        assert.equal(uri.normalize(reference), "//a.com", reference);
        assert.equal(uri.equal(reference, "//a.com"), true, reference);
    }
});

test("all selector-parser build dependencies include the CPU-exhaustion fix", () => {
    const lock = require("../../package-lock.json");
    const semver = require("semver");
    const parsers = Object.entries(lock.packages)
        .filter(([path]) => path.endsWith("node_modules/postcss-selector-parser"));
    assert.ok(parsers.length > 0);
    for (const [path, metadata] of parsers) {
        assert.ok(semver.gte(metadata.version, "7.1.6"), `${path}: ${metadata.version}`);
    }
    const fromTailwind = createRequire(require.resolve("tailwindcss"));
    assert.ok(semver.gte(fromTailwind("postcss-selector-parser/package.json").version, "7.1.6"));
});

test("patched selector parser preserves nested build selectors", async () => {
    const postcss = require("postcss");
    const nested = require("postcss-nested");
    const result = await postcss([nested]).process(
        '.card { &:hover, &[data-state="open"] { color: red; } .child { display: block; } }',
        {from: undefined}
    );
    const selectors = [];
    result.root.walkRules(rule => selectors.push(rule.selector));
    assert.deepEqual(selectors, ['.card:hover, .card[data-state="open"]', '.card .child']);
});
