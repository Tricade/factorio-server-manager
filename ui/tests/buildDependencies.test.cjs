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

test("build URI parsing does not turn encoded schemes into trusted schemes", () => {
    // GHSA-w74x-v6f9-j3xg: examine strings only; no requests are made.
    for (const reference of ["ht%74ps://example.invalid/", "%u0068ttps://example.invalid/", "foo%2Bbar:value"]) {
        assert.match(uri.parse(reference).error, /scheme is malformed/i, reference);
        assert.equal(uri.normalize(reference), reference);
        assert.throws(() => uri.resolve("https://example.invalid/", reference), /scheme is malformed/i);
    }
    assert.equal(uri.equal("ht%74ps://example.invalid/", "https://example.invalid/"), false);
    assert.equal(uri.normalize("HTTPS://EXAMPLE.INVALID:443/"), "https://example.invalid/");
});

test("build serialization neutralizes a script end tag at the end of a function", () => {
    const serialize = createRequire(require.resolve("css-minimizer-webpack-plugin"))("serialize-javascript");
    // GHSA-m5mm-p8h7-m29v: synthetic source is serialized but never executed.
    for (const suffix of ["//</script", "</ScRiPt"]) {
        const synthetic = function () {};
        synthetic.toString = () => `function synthetic() { return 1 }${suffix}`;
        const output = serialize({nested: {synthetic}}, {space: 2});
        assert.doesNotMatch(output, /<\/script(?=[\t\n\f\r />]|$)/i);
    }
    assert.equal(serialize({value: "plain text"}), '{"value":"plain text"}');
});

test("all URI and serialization build dependencies include the security patches", () => {
    const lock = require("../../package-lock.json");
    const semver = require("semver");
    for (const [name, minimum] of [["fast-uri", "3.1.9"], ["serialize-javascript", "7.1.3"]]) {
        const copies = Object.entries(lock.packages).filter(([path]) => path.endsWith(`node_modules/${name}`));
        assert.ok(copies.length > 0, name);
        for (const [path, metadata] of copies) {
            assert.ok(semver.gte(metadata.version, minimum), `${path}: ${metadata.version}`);
        }
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
});

test("the build dependency tree no longer includes unpatched braces", () => {
    const lock = require("../../package-lock.json");
    assert.deepEqual(Object.keys(lock.packages).filter(path => path.endsWith("node_modules/braces")), []);
});

test("Tailwind builds the existing palette and responsive utilities from the real stylesheet", async () => {
    const fs = require("node:fs");
    const path = require("node:path");
    const postcss = require("postcss");
    const from = path.resolve(__dirname, "../index.css");
    const result = await postcss([require("@tailwindcss/postcss")]).process(fs.readFileSync(from, "utf8"), {from});
    assert.match(result.css, /\.text-orange\s*\{\s*color: #F4A62A/i);
    assert.match(result.css, /\.sm\\:flex-row/);
    assert.match(result.css, /\.border-white\\\/5/);
    assert.match(result.css, /\.bg-orange\\\/10/);
    assert.match(result.css, /\.shrink-0/);
    assert.match(result.css, /\.ui-sidebar/);
    assert.doesNotMatch(result.css, /@import "tailwindcss"/);
});
