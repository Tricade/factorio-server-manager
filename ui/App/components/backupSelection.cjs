function importSelection(profiles) {
    return profiles.filter(profile => profile.selected).map(profile => ({id: profile.id, name: profile.name.trim()}));
}

function validImportSelection(profiles) {
    const selected = importSelection(profiles);
    const names = selected.map(profile => profile.name.toLowerCase());
    return selected.length > 0 && selected.every(profile => profile.name && [...profile.name].length <= 64 && !/[\x00-\x1f\x7f]/.test(profile.name))
        && new Set(names).size === names.length;
}

module.exports = {importSelection, validImportSelection};
