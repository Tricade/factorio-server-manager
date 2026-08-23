import client from "../client";
import loadMapEntities from "./mapEntities";

export default {
    factorioVersion: async () => {
        const response = await client.get('/api/server/facVersion');
        return response.data;
    },
    installRelease: async (target) => {
        const response = await client.post('/api/server/release/install', {target});
        return response.data;
    },
    releaseStatus: async () => {
        const response = await client.get('/api/server/release/status');
        return response.data;
    },
    gameMode: async () => {
        const response = await client.get('/api/server/game-mode');
        return response.data;
    },
    setGameMode: async (mode) => {
        const response = await client.post('/api/server/game-mode', {mode});
        return response.data;
    },
    status: async () => {
        const response = await client.get('/api/server/status');
        return response.data;
    },
    players: async () => {
        const response = await client.get('/api/server/players');
        return response.data;
    },
    autostart: async () => {
        const response = await client.get('/api/server/autostart');
        return response.data;
    },
    setAutostart: async (enabled) => {
        const response = await client.put('/api/server/autostart', {enabled});
        return response.data;
    },
    mapSnapshot: async () => {
        const response = await client.get('/api/map-snapshot');
        return response.data;
    },
    mapSnapshotEntities: loadMapEntities,
    refreshMapSnapshot: async () => {
        const response = await client.post('/api/map-snapshot/refresh');
        return response.data;
    },
    setMapSnapshotSettings: async (intervalMinutes) => {
        const response = await client.put('/api/map-snapshot/settings', {interval_minutes: intervalMinutes});
        return response.data;
    },
    stop: async () => {
        const response = await client.post('/api/server/stop');
        return response.data;
    },
    start: async (ip, port, savefile) => {
        const response = await client.post('/api/server/start', {
            bindip: ip,
            savefile,
            port
        });
        return response.data;
    },
    kill: async () => {
        const response = await client.post('/api/server/kill');
        return response.data;
    }
}
