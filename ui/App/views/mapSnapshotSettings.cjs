const defaultAutomaticInterval = 60;

const positiveInterval = value => {
    const interval = Number(value);
    return Number.isInteger(interval) && interval >= 1 && interval <= 10080
        ? interval
        : defaultAutomaticInterval;
};

const mapSnapshotFormValues = settings => {
    const storedInterval = Number(settings?.interval_minutes ?? defaultAutomaticInterval);
    return {
        mode: settings?.enabled === false ? "disabled" : storedInterval > 0 ? "automatic" : "manual",
        automatic_interval_minutes: storedInterval > 0 ? positiveInterval(storedInterval) : defaultAutomaticInterval,
        automatic_only_when_no_players: Boolean(settings?.automatic_only_when_no_players),
        include_space_platforms: settings?.include_space_platforms !== false
    };
};

const mapSnapshotRequestValues = values => {
    const mode = ["automatic", "manual", "disabled"].includes(values?.mode) ? values.mode : "automatic";
    const interval = positiveInterval(values?.automatic_interval_minutes);
    return {
        enabled: mode !== "disabled",
        intervalMinutes: mode === "manual" ? 0 : interval,
        automaticOnlyWhenNoPlayers: Boolean(values?.automatic_only_when_no_players),
        includeSpacePlatforms: values?.include_space_platforms !== false
    };
};

module.exports = {mapSnapshotFormValues, mapSnapshotRequestValues};
