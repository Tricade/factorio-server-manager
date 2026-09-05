const cloneSettingValue = value => {
    if (value === null || typeof value !== "object") return value;
    if (Array.isArray(value)) return value.map(cloneSettingValue);
    return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, cloneSettingValue(entry)]));
};

const settingValuesEqual = (left, right) => {
    if (Object.is(left, right)) return true;
    if (!left || !right || typeof left !== "object" || typeof right !== "object") return false;
    const leftKeys = Object.keys(left).sort();
    const rightKeys = Object.keys(right).sort();
    if (leftKeys.length !== rightKeys.length || leftKeys.some((key, index) => key !== rightKeys[index])) return false;
    return leftKeys.every(key => settingValuesEqual(left[key], right[key]));
};

const normalizeSettingDraft = (setting, value) => {
    if (setting.type === "int-setting") {
        if (value === "" || !Number.isInteger(Number(value))) throw new Error(`${setting.display_name} requires a whole number.`);
        return Number(value);
    }
    if (setting.type === "double-setting") {
        if (value === "" || !Number.isFinite(Number(value))) throw new Error(`${setting.display_name} requires a number.`);
        return Number(value);
    }
    return cloneSettingValue(value);
};

const formatSettingValue = value => {
    if (typeof value === "boolean") return value ? "On" : "Off";
    if (value && typeof value === "object" && ["r", "g", "b"].every(component => component in value)) {
        return `${colorToHex(value)} · ${Math.round((value.a ?? 1) * 100)}% alpha`;
    }
    return String(value ?? "");
};

const clampColorComponent = value => Math.max(0, Math.min(1, Number(value) || 0));
const componentToHex = value => Math.round(clampColorComponent(value) * 255).toString(16).padStart(2, "0");
const colorToHex = color => `#${componentToHex(color?.r)}${componentToHex(color?.g)}${componentToHex(color?.b)}`;

const hexToColor = (hex, alpha = 1) => {
    const normalized = /^#[0-9a-f]{6}$/i.test(hex) ? hex.slice(1) : "000000";
    return {
        r: parseInt(normalized.slice(0, 2), 16) / 255,
        g: parseInt(normalized.slice(2, 4), 16) / 255,
        b: parseInt(normalized.slice(4, 6), 16) / 255,
        a: clampColorComponent(alpha)
    };
};

module.exports = {
    cloneSettingValue,
    settingValuesEqual,
    normalizeSettingDraft,
    formatSettingValue,
    colorToHex,
    hexToColor,
    clampColorComponent
};
