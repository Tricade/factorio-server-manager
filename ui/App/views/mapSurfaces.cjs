const groupDefinitions = [
    {kind: "planet", label: "Planets"},
    {kind: "platform", label: "Space platforms"},
    {kind: "surface", label: "Other surfaces"}
];

const technicalPlatformName = /^platform-\d+$/i;

// Browser-native option elements cannot reliably render Factorio sprites.
// Keep player-visible names readable by removing known inline icon tags while
// preserving unknown bracketed text and escaped "[[" sequences.
const factorioIconTags = new Set([
    "achievement", "armor", "entity", "fluid", "gps", "icon", "img",
    "item", "item-group", "planet", "player", "quality", "recipe",
    "shortcut", "space-age", "space-location", "space-platform",
    "special-item", "surface", "technology", "tile", "tip", "train",
    "train-stop", "virtual-signal"
]);
const factorioTextModifierTags = new Set(["color", "font"]);

const stripFactorioRichText = value => {
    if (typeof value !== "string") return "";

    let plainText = "";
    for (let index = 0; index < value.length;) {
        if (value[index] !== "[") {
            plainText += value[index];
            index += 1;
            continue;
        }
        if (value[index + 1] === "[") {
            plainText += "[[";
            index += 2;
            continue;
        }

        const closingBracket = value.indexOf("]", index + 1);
        if (closingBracket === -1) {
            plainText += value.slice(index);
            break;
        }
        const contents = value.slice(index + 1, closingBracket);
        if (contents.includes("\n") || contents.includes("\r")) {
            plainText += value[index];
            index += 1;
            continue;
        }

        const normalized = contents.trim().toLowerCase();
        const isClosingModifier = (normalized.startsWith("/") || normalized.startsWith("."))
            && factorioTextModifierTags.has(normalized.slice(1));
        const separator = normalized.indexOf("=");
        const tagName = (separator === -1 ? normalized : normalized.slice(0, separator)).trim();
        const isOpeningModifier = separator !== -1 && factorioTextModifierTags.has(tagName);
        const isIcon = factorioIconTags.has(tagName) && (separator !== -1 || tagName === "space-age");
        if (isClosingModifier || isOpeningModifier || isIcon) {
            index = closingBracket + 1;
            continue;
        }

        plainText += value.slice(index, closingBracket + 1);
        index = closingBracket + 1;
    }
    return plainText.replace(/\s+/g, " ").trim();
};

const mapSurfaceLabel = surface => {
    const name = stripFactorioRichText(surface?.name);
    const technicalName = stripFactorioRichText(surface?.surface_name);
    return name || technicalName || `Surface ${surface?.id || surface?.index || "?"}`;
};

const mapSurfaceKind = surface => {
    const kind = typeof surface?.kind === "string" ? surface.kind.trim().toLowerCase() : "";
    if (groupDefinitions.some(group => group.kind === kind)) return kind;
    const technicalName = typeof surface?.surface_name === "string" && surface.surface_name.trim()
        ? surface.surface_name.trim()
        : mapSurfaceLabel(surface);
    if (technicalPlatformName.test(technicalName)) return "platform";
    // Image-only snapshots predate explicit surface kinds. Their non-platform
    // entries were overwhelmingly planets, so keep those useful under Planets.
    return "planet";
};

const groupMapSurfaces = surfaces => groupDefinitions
    .map(group => ({
        ...group,
        surfaces: (Array.isArray(surfaces) ? surfaces : []).filter(surface => mapSurfaceKind(surface) === group.kind)
    }))
    .filter(group => group.surfaces.length > 0);

module.exports = {groupMapSurfaces, mapSurfaceKind, mapSurfaceLabel, stripFactorioRichText};
