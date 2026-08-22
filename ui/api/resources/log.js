import client from "../client";

export default {
    tail: async () => {
        const response = await client.get('/api/log/tail');
        if (!Array.isArray(response.data)) {
            throw new Error(typeof response.data === "string"
                ? response.data
                : "The log endpoint returned an unexpected response.");
        }
        return response.data;
    },
}
