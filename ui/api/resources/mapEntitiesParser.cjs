const maximumEntityCount = 100000;
const maximumEntityLineLength = 16384;
const controlCharacterPattern = /[\u0000-\u001f\u007f]/;

const validateEntity = value => {
    const box = value?.bounding_box;
    const coordinates = [box?.left_top?.x, box?.left_top?.y, box?.right_bottom?.x, box?.right_bottom?.y];
    if (
        !value || typeof value.name !== "string" || typeof value.type !== "string"
        || value.name.length === 0 || value.name.length > 200 || controlCharacterPattern.test(value.name)
        || value.type.length === 0 || value.type.length > 200 || controlCharacterPattern.test(value.type)
        || coordinates.some(coordinate => !Number.isFinite(coordinate) || Math.abs(coordinate) > 10000000)
        || box.left_top.x > box.right_bottom.x || box.left_top.y > box.right_bottom.y
    ) {
        throw new Error("The map entity endpoint returned an invalid record.");
    }
    return value;
};

const parseLine = (line, entities) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    if (trimmed.length > maximumEntityLineLength) throw new Error("The map entity endpoint returned an oversized record.");
    if (entities.length >= maximumEntityCount) throw new Error("The map entity endpoint returned too many records.");
    entities.push(validateEntity(JSON.parse(trimmed)));
};

const createEntityAccumulator = () => {
    const entities = [];
    let pending = "";
    return {
        push(chunk) {
            pending += chunk;
            const lines = pending.split(/\r?\n/);
            pending = lines.pop() || "";
            if (pending.length > maximumEntityLineLength) throw new Error("The map entity endpoint returned an oversized record.");
            lines.forEach(line => parseLine(line, entities));
        },
        finish(chunk = "") {
            pending += chunk;
            parseLine(pending, entities);
            pending = "";
            return entities;
        }
    };
};

module.exports = {createEntityAccumulator, validateEntity};
