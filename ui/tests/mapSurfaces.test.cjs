const test = require("node:test");
const assert = require("node:assert/strict");
const {groupMapSurfaces, mapSurfaceKind, mapSurfaceLabel, stripFactorioRichText} = require("../App/views/mapSurfaces.cjs");

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

test("surface labels hide Factorio icon markup", () => {
    assert.equal(mapSurfaceLabel({name: "Example Name[icon=example]", surface_name: "platform-7"}), "Example Name");
    assert.equal(mapSurfaceLabel({name: "Iron [item=iron-plate] Express [fluid=water]", surface_name: "platform-8"}), "Iron Express");
    assert.equal(mapSurfaceLabel({name: "[img=item.iron-plate]", surface_name: "platform-9"}), "platform-9");
});

test("plain Factorio labels retain text and escaped or unknown brackets", () => {
    assert.equal(stripFactorioRichText("[color=red]Red[/color] [font=default-bold]Line[/font]"), "Red Line");
    assert.equal(stripFactorioRichText("[[item=iron-plate] [custom=value]"), "[[item=iron-plate] [custom=value]");
    assert.equal(stripFactorioRichText("[quality=legendary][space-platform=3] Cargo"), "Cargo");
});

test("all supported Factorio inline icon tag types are hidden", () => {
    const iconTags = [
        "achievement=minions", "armor=Player", "entity=small-biter", "fluid=water", "gps=0,0",
        "icon=example", "img=item.iron-plate", "item=iron-plate", "item-group=combat", "planet=gleba",
        "player=Player", "quality=legendary", "recipe=iron-plate", "shortcut=give-spidertron-remote",
        "space-age", "space-location=shattered-planet", "space-platform=3", "special-item=blueprint",
        "surface=nauvis", "technology=logistics", "tile=grass-3", "tip=spidertron-control", "train=93",
        "train-stop=100", "virtual-signal=signal-A"
    ];
    assert.equal(stripFactorioRichText(`${iconTags.map(tag => `[${tag}]`).join("")} Cargo`), "Cargo");
});
