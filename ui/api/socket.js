import EventEmitter from "events";

const bus = new EventEmitter();
const desiredRooms = new Set();
let socket = null;
let reconnectTimer = null;
let shouldReconnect = false;

const socketURL = () => `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;

const sendControl = (type, value) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return false;
    try {
        socket.send(JSON.stringify({room_name: "", controls: {type, value}}));
        return true;
    } catch {
        // A connection can fail between the ready-state check and send. Never
        // queue commands: replaying one after reconnect could repeat a mutation.
        socket.close();
        return false;
    }
};

const subscribe = room => {
    desiredRooms.add(room);
    sendControl("subscribe", room);
};

const unsubscribe = room => {
    desiredRooms.delete(room);
    sendControl("unsubscribe", room);
};

const scheduleReconnect = () => {
    if (!shouldReconnect || reconnectTimer) return;
    reconnectTimer = window.setTimeout(() => {
        reconnectTimer = null;
        connect();
    }, 2500);
};

const connect = () => {
    shouldReconnect = true;
    if (socket && [WebSocket.CONNECTING, WebSocket.OPEN].includes(socket.readyState)) return;
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
    reconnectTimer = null;

    bus.emit("connection_state", "connecting");
    let connection;
    try {
        connection = new WebSocket(socketURL());
    } catch {
        socket = null;
        bus.emit("connection_state", "disconnected");
        scheduleReconnect();
        return;
    }
    socket = connection;
    const isCurrent = () => shouldReconnect && socket === connection;

    connection.onopen = () => {
        if (!isCurrent()) return;
        bus.emit("connection_state", "connected");
        desiredRooms.forEach(room => sendControl("subscribe", room));
    };

    connection.onmessage = event => {
        if (!isCurrent()) return;
        try {
            const {room_name: roomName, message} = JSON.parse(event.data);
            if (roomName) bus.emit(roomName, message);
        } catch (error) {
            bus.emit("protocol_error", error);
        }
    };

    connection.onerror = () => {
        if (isCurrent()) connection.close();
    };
    connection.onclose = () => {
        if (!isCurrent()) return;
        socket = null;
        bus.emit("connection_state", "disconnected");
        scheduleReconnect();
    };
};

const disconnect = () => {
    shouldReconnect = false;
    desiredRooms.clear();
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
    if (socket) {
        const connection = socket;
        socket = null;
        connection.onopen = connection.onmessage = connection.onerror = connection.onclose = null;
        connection.close();
    }
    bus.emit("connection_state", "disconnected");
};

bus.on("log subscribe", () => subscribe("gamelog"));
bus.on("log unsubscribe", () => unsubscribe("gamelog"));
bus.on("server status subscribe", () => subscribe("server_status"));
const sendCommand = command => {
    if (!sendControl("command", command)) {
        bus.emit("command_error", "Console connection is not ready yet.");
        return false;
    }
    return true;
};
bus.on("command send", sendCommand);

bus.connect = connect;
bus.disconnect = disconnect;
bus.sendCommand = sendCommand;

export default bus;
