const test = require("node:test");
const assert = require("node:assert/strict");
const {createEntityAccumulator, validateEntity} = require("../api/resources/mapEntitiesParser.cjs");

const entity = (name = "assembling-machine-3") => ({
    name,
    type: "assembling-machine",
    direction: 0,
    bounding_box: {
        left_top: {x: -1.5, y: 2.25},
        right_bottom: {x: 1.5, y: 5.25}
    }
});

test("NDJSON entity parsing handles chunk boundaries, CRLF, and a final unterminated record", () => {
    const accumulator = createEntityAccumulator();
    const first = JSON.stringify(entity("assembler-a"));
    const second = JSON.stringify(entity("assembler-b"));
    accumulator.push(`${first.slice(0, 23)}`);
    accumulator.push(`${first.slice(23)}\r\n\n${second.slice(0, 11)}`);
    const result = accumulator.finish(second.slice(11));
    assert.deepEqual(result.map(item => item.name), ["assembler-a", "assembler-b"]);
});

test("entity validation rejects inverted coordinates and control characters", () => {
    const inverted = entity();
    inverted.bounding_box.right_bottom.x = -2;
    assert.throws(() => validateEntity(inverted), /invalid record/);
    assert.throws(() => validateEntity(entity("bad\nname")), /invalid record/);
});

test("stream parsing rejects an oversized unterminated record", () => {
    const accumulator = createEntityAccumulator();
    assert.throws(() => accumulator.push("x".repeat(16385)), /oversized record/);
});
