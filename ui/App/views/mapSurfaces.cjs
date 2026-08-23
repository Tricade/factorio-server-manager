const groupDefinitions = [
    {kind: "planet", label: "Planets"},
    {kind: "platform", label: "Space platforms"},
    {kind: "surface", label: "Other surfaces"}
];

const technicalPlatformName = /^platform-\d+$/i;

const mapSurfaceLabel = surface => {
    const name = typeof surface?.name === "string" ? surface.name.trim() : "";
    const technicalName = typeof surface?.surface_name === "string" ? surface.surface_name.trim() : "";
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

module.exports = {groupMapSurfaces, mapSurfaceKind, mapSurfaceLabel};
