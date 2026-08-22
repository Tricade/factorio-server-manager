import EventEmitter from "events";

const bus = new EventEmitter();
const desiredRooms = new Set();
let socket = null;
let reconnectTimer = null;
let shouldReconnect = false;

const socketURL = () => `${window.location.protocol === "https:" ? "wss" : "ws"}://${window.location.host}/ws`;

const sendControl = (type, value) => {
    if (!socket || socket.readyState !== WebSocket.OPEN) return false;
    socket.send(JSON.stringify({room_name: "", controls: {type, value}}));
    return true;
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

    bus.emit("connection_state", "connecting");
    socket = new WebSocket(socketURL());

    socket.onopen = () => {
        bus.emit("connection_state", "connected");
        desiredRooms.forEach(room => sendControl("subscribe", room));
    };

    socket.onmessage = event => {
        try {
            const {room_name: roomName, message} = JSON.parse(event.data);
            if (roomName) bus.emit(roomName, message);
        } catch (error) {
            bus.emit("protocol_error", error);
        }
    };

    socket.onerror = () => socket?.close();
    socket.onclose = () => {
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
        socket.onclose = null;
        socket.close();
        socket = null;
    }
    bus.emit("connection_state", "disconnected");
};

bus.on("log subscribe", () => subscribe("gamelog"));
bus.on("log unsubscribe", () => unsubscribe("gamelog"));
bus.on("server status subscribe", () => subscribe("server_status"));
bus.on("command send", command => {
    if (!sendControl("command", command)) {
        bus.emit("command_error", "Console connection is not ready yet.");
    }
});

bus.connect = connect;
bus.disconnect = disconnect;

export default bus;
