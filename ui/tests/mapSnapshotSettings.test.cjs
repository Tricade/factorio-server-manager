const test = require("node:test");
const assert = require("node:assert/strict");
const {mapSnapshotFormValues, mapSnapshotRequestValues} = require("../App/views/mapSnapshotSettings.cjs");

test("stored map settings map to distinct automatic, manual-only and disabled UI modes", () => {
    assert.deepEqual(mapSnapshotFormValues({
        enabled: true,
        interval_minutes: 90,
        automatic_only_when_no_players: true,
        include_space_platforms: false
    }), {
        mode: "automatic",
        automatic_interval_minutes: 90,
        automatic_only_when_no_players: true,
        include_space_platforms: false
    });
    assert.equal(mapSnapshotFormValues({enabled: true, interval_minutes: 0}).mode, "manual");
    assert.deepEqual(mapSnapshotFormValues({enabled: false, interval_minutes: 120}), {
        mode: "disabled",
        automatic_interval_minutes: 120,
        automatic_only_when_no_players: false,
        include_space_platforms: true
    });
});

test("manual-only keeps manual rendering enabled while disabled blocks every render", () => {
    assert.deepEqual(mapSnapshotRequestValues({
        mode: "manual",
        automatic_interval_minutes: 30,
        automatic_only_when_no_players: true,
        include_space_platforms: true
    }), {
        enabled: true,
        intervalMinutes: 0,
        automaticOnlyWhenNoPlayers: true,
        includeSpacePlatforms: true
    });
    assert.deepEqual(mapSnapshotRequestValues({
        mode: "disabled",
        automatic_interval_minutes: 30,
        include_space_platforms: false
    }), {
        enabled: false,
        intervalMinutes: 30,
        automaticOnlyWhenNoPlayers: false,
        includeSpacePlatforms: false
    });
});
