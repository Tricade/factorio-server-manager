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
