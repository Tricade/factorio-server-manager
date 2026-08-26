const maximumListedSkippedMods = 3;

const skipReasonLabel = reason => {
    switch (reason) {
        case "release-unavailable":
            return "release unavailable";
        case "archive-identity-mismatch":
            return "archive metadata mismatch";
        default:
            return "could not be imported";
    }
};

const displayValue = (value, fallback) => typeof value === "string" && value.trim()
    ? value.trim()
    : fallback;

const saveModImportFeedback = (saveName, result) => {
    const save = displayValue(saveName, "the selected save");
    const skipped = Array.isArray(result?.skipped) ? result.skipped : [];
    if (skipped.length === 0) {
        return {message: `Mods imported from ${save}.`, color: "green"};
    }

    const listed = skipped.slice(0, maximumListedSkippedMods).map(mod => {
        const name = displayValue(mod?.name, "Unknown mod");
        const version = displayValue(mod?.version, "unknown version");
        return `${name} ${version} (${skipReasonLabel(mod?.reason)})`;
    });
    const remainder = skipped.length - listed.length;
    const more = remainder > 0 ? `; and ${remainder} more` : "";
    const noun = skipped.length === 1 ? "mod was" : "mods were";

    return {
        message: `Not all mods could be imported from ${save}. Available mods were installed; ${skipped.length} ${noun} skipped: ${listed.join("; ")}${more}.`,
        color: "gray-light"
    };
};

module.exports = {saveModImportFeedback, skipReasonLabel};
