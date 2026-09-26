import client from "../client";

const upload = async (path, file, extra = {}) => {
    const form = new FormData();
    form.append("backup", file);
    Object.entries(extra).forEach(([key, value]) => form.append(key, value));
    const response = await client.post(path, form, {headers: {"Content-Type": "multipart/form-data"}});
    return response.data;
};

export default {
    exportProfiles: async options => {
        const response = await client.post("/api/profiles/export", options, {responseType: "blob"});
        const url = URL.createObjectURL(response.data);
        const link = document.createElement("a");
        link.href = url;
        link.download = "factorio-profiles-backup.zip";
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.setTimeout(() => URL.revokeObjectURL(url), 60000);
    },
    previewProfiles: file => upload("/api/profiles/import/preview", file),
    importProfiles: (file, selection) => upload("/api/profiles/import", file, {selection: JSON.stringify(selection)}),
    previewMods: file => upload("/api/mods/backup/preview", file),
    restoreMods: (file, profileID) => upload("/api/mods/backup/restore", file, {profile_id: profileID})
};
