const test = require("node:test");
const assert = require("node:assert/strict");
const {mapOverlayGeometry} = require("../App/components/mapOverlayGeometry.cjs");

test("small platform overlays target a useful drawing resolution without exceeding their cap", () => {
    assert.deepEqual(mapOverlayGeometry({width: 120, height: 240}, true), {
        outputScale: 1024 / 240,
        width: 512,
        height: 1024
    });
    assert.deepEqual(mapOverlayGeometry({width: 39, height: 29}, true), {
        outputScale: 1024 / 39,
        width: 1024,
        height: 761
    });
});

test("large and ordinary overlays stay within the shared canvas limit", () => {
    assert.deepEqual(mapOverlayGeometry({width: 1000, height: 500}, false), {
        outputScale: 1,
        width: 1000,
        height: 500
    });
    assert.deepEqual(mapOverlayGeometry({width: 4096, height: 2048}, true), {
        outputScale: 0.5,
        width: 2048,
        height: 1024
    });
    assert.equal(mapOverlayGeometry({width: 0, height: 100}, true), null);
});
