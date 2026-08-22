import client from "../client";

const profiles = {
    list: async () => {
        const response = await client.get("/api/profiles");
        return response.data;
    },
    create: async data => {
        const response = await client.post("/api/profiles", data);
        return response.data;
    },
    update: async (id, data) => {
        const response = await client.patch("/api/profiles/" + encodeURIComponent(id), data);
        return response.data;
    },
    updateStartup: async (id, data) => {
        const response = await client.put("/api/profiles/" + encodeURIComponent(id) + "/startup", data);
        return response.data;
    },
    remove: async id => {
        const response = await client.delete("/api/profiles/" + encodeURIComponent(id));
        return response.data;
    },
    activate: async id => {
        const response = await client.post("/api/profiles/" + encodeURIComponent(id) + "/activate");
        return response.data;
    }
};

export default profiles;
