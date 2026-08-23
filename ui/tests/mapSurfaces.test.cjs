const test = require("node:test");
const assert = require("node:assert/strict");
const {groupMapSurfaces, mapSurfaceKind, mapSurfaceLabel} = require("../App/views/mapSurfaces.cjs");

test("map surfaces are grouped with planets first and platforms second", () => {
    const groups = groupMapSurfaces([
        {id: "1", name: "nauvis", surface_name: "nauvis", kind: "planet"},
        {id: "2", name: "Supply Express", surface_name: "platform-1", kind: "platform"},
        {id: "3", name: "vulcanus", surface_name: "vulcanus", kind: "planet"},
        {id: "4", name: "Factory floor", surface_name: "factory-floor", kind: "surface"}
    ]);

    assert.deepEqual(groups.map(group => group.label), ["Planets", "Space platforms", "Other surfaces"]);
    assert.deepEqual(groups[0].surfaces.map(surface => surface.name), ["nauvis", "vulcanus"]);
    assert.deepEqual(groups[1].surfaces.map(surface => surface.name), ["Supply Express"]);
});

test("legacy platform surfaces remain grouped and labels have safe fallbacks", () => {
    assert.equal(mapSurfaceKind({name: "platform-9"}), "platform");
    assert.equal(mapSurfaceKind({name: "Cargo Bus", surface_name: "platform-9"}), "platform");
    assert.equal(mapSurfaceKind({name: "nauvis"}), "planet");
    assert.equal(mapSurfaceLabel({id: "7", name: "", surface_name: "platform-7"}), "platform-7");
});
