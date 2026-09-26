const {test} = require("node:test");
const assert = require("node:assert/strict");
const {importSelection, validImportSelection} = require("../App/components/backupSelection.cjs");

test("backup import uses only selected source IDs and trimmed new names", () => {
    assert.deepEqual(importSelection([{id: "a", name: " New profile ", selected: true}, {id: "b", name: "No", selected: false}]), [{id: "a", name: "New profile"}]);
});
test("backup import rejects empty, duplicated, overlong and control-character names", () => {
    for (const names of [[], [" "], ["Factory", " factory "], ["x".repeat(65)], ["bad\nname"]]) {
        assert.equal(validImportSelection(names.map((name, id) => ({id, name, selected: true}))), false);
    }
    assert.equal(validImportSelection([{id: "a", name: "Imported factory", selected: true}]), true);
});
