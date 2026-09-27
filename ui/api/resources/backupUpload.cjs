const backupUploadInterruptedMessage = "Backup upload interrupted. Check the connection, reverse proxy upload limit and available temporary disk space, then retry.";

function formatBackupSize(bytes) {
    const unit = bytes > 0 ? Math.min(3, Math.floor(Math.log(bytes) / Math.log(1024))) : 0;
    return `${Number((bytes / (1024 ** unit)).toFixed(2))} ${["B", "KiB", "MiB", "GiB"][unit]}`;
}

function validBackupLimits(limits) {
    return limits && Number.isSafeInteger(limits.max_upload_bytes) && limits.max_upload_bytes > 0
        && Number.isSafeInteger(limits.max_file_bytes) && limits.max_file_bytes >= 0
        && limits.max_file_bytes < limits.max_upload_bytes;
}

function backupFileError(file, limits, kind = "profiles") {
    if (!file) return "Choose a backup ZIP.";
    if (!Number.isSafeInteger(file.size) || file.size <= 0) return "The backup ZIP is empty or its size could not be read.";
    if (!validBackupLimits(limits)) return "The backup upload limit could not be loaded. Retry before uploading.";
    if (file.size <= limits.max_file_bytes) return "";
    const advice = kind === "profiles"
        ? "Export one profile at a time instead of all profiles. If one profile is still too large, omit saves/checkpoints or increase FSM_MAX_UPLOAD on the destination."
        : "Use a smaller mod backup or increase FSM_MAX_UPLOAD on the destination.";
    return `This backup (${formatBackupSize(file.size)}) does not fit the ${formatBackupSize(limits.max_upload_bytes)} upload limit, including upload metadata. ${advice}`;
}

function validationError(message, limits) {
    const error = new Error(message);
    error.userMessage = message;
    error.backupLimits = limits;
    return error;
}

function backupErrorMessage(error) {
    return error?.userMessage || (typeof error?.response?.data === "string" ? error.response.data : "The backup could not be processed. Try again.");
}

function createBackupUploader(client) {
    const getLimits = async () => {
        const {data} = await client.get("/api/backups/limits");
        if (!validBackupLimits(data)) throw validationError("The backup upload limit could not be loaded. Retry before uploading.");
        return data;
    };
    const upload = async (path, file, extra = {}) => {
        // Recheck for both preview and confirmation: a container restart or a
        // config change must not turn a stale dialog into a multi-GB upload.
        const limits = await getLimits();
        const message = backupFileError(file, limits, path.startsWith("/api/profiles/") ? "profiles" : "mods");
        if (message) throw validationError(message, limits);
        const form = new FormData();
        // Keep multipart headers bounded; the source filename is not needed by
        // the backup parser. Profile/save/mod names remain inside the archive.
        form.append("backup", file, "backup.zip");
        Object.entries(extra).forEach(([key, value]) => form.append(key, value));
        const response = await client.post(path, form, {
            headers: {"Content-Type": "multipart/form-data"},
            networkErrorMessage: backupUploadInterruptedMessage
        });
        return response.data;
    };
    return {getLimits, upload};
}

module.exports = {createBackupUploader, backupFileError, backupErrorMessage, formatBackupSize};
