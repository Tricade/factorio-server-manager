const test = require("node:test");
const assert = require("node:assert/strict");
const {
    cloneSettingValue,
    settingValuesEqual,
    normalizeSettingDraft,
    formatSettingValue,
    colorToHex,
    hexToColor
} = require("../App/views/Mods/modStartupSettings.cjs");

test("setting drafts are cloned and compared structurally", () => {
    const source = {r: 0.25, g: 0.5, b: 0.75, a: 1};
    const clone = cloneSettingValue(source);
    assert.deepEqual(clone, source);
    assert.notEqual(clone, source);
    assert.equal(settingValuesEqual(source, {a: 1, b: 0.75, g: 0.5, r: 0.25}), true);
    clone.r = 1;
    assert.equal(settingValuesEqual(source, clone), false);
});

test("numeric drafts are normalized before requests", () => {
    assert.equal(normalizeSettingDraft({type: "int-setting", display_name: "Count"}, "42"), 42);
    assert.equal(normalizeSettingDraft({type: "double-setting", display_name: "Ratio"}, "1.25"), 1.25);
    assert.throws(() => normalizeSettingDraft({type: "int-setting", display_name: "Count"}, "1.5"), /whole number/);
    assert.throws(() => normalizeSettingDraft({type: "double-setting", display_name: "Ratio"}, ""), /requires a number/);
});

test("Factorio colors round-trip through the native color control", () => {
    const color = {r: 1, g: 0.5, b: 0, a: 0.75};
    assert.equal(colorToHex(color), "#ff8000");
    const decoded = hexToColor("#ff8000", color.a);
    assert.equal(decoded.r, 1);
    assert.ok(Math.abs(decoded.g - (128 / 255)) < 0.0001);
    assert.equal(decoded.b, 0);
    assert.equal(decoded.a, 0.75);
    assert.equal(formatSettingValue(color), "#ff8000 · 75% alpha");
});
