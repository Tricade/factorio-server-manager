const test = require("node:test");
const assert = require("node:assert/strict");
const {saveModImportFeedback, skipReasonLabel} = require("../App/views/Mods/components/saveModImportFeedback.cjs");

test("successful save-mod import retains the success notification", () => {
    assert.deepEqual(saveModImportFeedback("world.zip", {mods: [], skipped: []}), {
        message: "Mods imported from world.zip.",
        color: "green"
    });
});

test("partial save-mod import reports each supported skip reason", () => {
    assert.deepEqual(saveModImportFeedback("old-world.zip", {
        skipped: [
            {name: "Aircraft", version: "1.8.6.0", reason: "release-unavailable"},
            {name: "Laser_Tanks_kr", version: "1.0.1.0", reason: "archive-identity-mismatch"}
        ]
    }), {
        message: "Not all mods could be imported from old-world.zip. Available mods were installed; 2 mods were skipped: Aircraft 1.8.6.0 (release unavailable); Laser_Tanks_kr 1.0.1.0 (archive metadata mismatch).",
        color: "gray-light"
    });
});

test("partial save-mod import bounds long notifications and hides unknown reason codes", () => {
    const skipped = ["one", "two", "three", "four"].map(name => ({name, version: "1.0.0.0", reason: "private-detail"}));
    const feedback = saveModImportFeedback("world.zip", {skipped});

    assert.match(feedback.message, /one 1\.0\.0\.0 \(could not be imported\)/);
    assert.match(feedback.message, /and 1 more/);
    assert.doesNotMatch(feedback.message, /four 1\.0\.0\.0/);
    assert.doesNotMatch(feedback.message, /private-detail/);
    assert.equal(skipReasonLabel("unknown"), "could not be imported");
});
