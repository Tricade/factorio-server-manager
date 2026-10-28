const {test} = require("node:test");
const assert = require("node:assert/strict");
const EventEmitter = require("node:events");
const loadModule = require("./helpers/loadModule.cjs");

function setup(protocol = "https:") {
    const connections = [], timers = new Map();
    let nextTimer = 1;
    class WebSocket {
        static CONNECTING = 0;
        static OPEN = 1;
        static CLOSING = 2;
        static CLOSED = 3;
        constructor(url) {
            this.url = url;
            this.readyState = WebSocket.CONNECTING;
            this.sent = [];
            connections.push(this);
        }
        open() { this.readyState = WebSocket.OPEN; this.onopen?.(); }
        send(data) {
            if (this.failSend) throw new Error("Transport closed during send");
            this.sent.push(JSON.parse(data));
        }
        close() { this.readyState = WebSocket.CLOSING; }
        finishClose() { this.readyState = WebSocket.CLOSED; this.onclose?.(); }
    }
    const window = {
        location: {protocol, host: "manager.example.test:8080"},
        setTimeout: callback => { const id = nextTimer++; timers.set(id, callback); return id; },
        clearTimeout: id => timers.delete(id)
    };
    const bus = loadModule("api/socket.js", {events: EventEmitter}, {window, WebSocket}).default;
    return {bus, connections, timers, WebSocket, runTimer() {
        const [id, callback] = timers.entries().next().value;
        timers.delete(id);
        callback();
    }};
}

test("websocket reconnect restores current rooms once, never queued commands", () => {
    const {bus, connections, timers, runTimer} = setup();
    bus.emit("server status subscribe");
    bus.emit("log subscribe");
    bus.connect();
    bus.connect();
    assert.equal(connections.length, 1);
    assert.equal(connections[0].url, "wss://manager.example.test:8080/ws");
    connections[0].open();
    bus.emit("command send", "hello");
    connections[0].finishClose();
    bus.emit("log unsubscribe");
    assert.equal(timers.size, 1);
    runTimer();
    connections[1].open();
    assert.deepEqual(connections[1].sent, [{room_name: "", controls: {type: "subscribe", value: "server_status"}}]);
    bus.disconnect();
    assert.equal(timers.size, 0);
});

test("callbacks from an obsolete connection cannot close or overwrite its replacement", () => {
    const {bus, connections, WebSocket} = setup();
    const states = [], logs = [];
    bus.on("connection_state", state => states.push(state));
    bus.on("gamelog", line => logs.push(line));
    bus.connect();
    const old = connections[0];
    const late = {error: old.onerror, close: old.onclose, open: old.onopen, message: old.onmessage};
    old.close();
    bus.connect();
    const current = connections[1];
    current.open();
    states.length = 0;
    late.error();
    late.close();
    late.open();
    late.message({data: JSON.stringify({room_name: "gamelog", message: "old session"})});
    assert.equal(current.readyState, WebSocket.OPEN);
    assert.deepEqual(states, []);
    assert.deepEqual(logs, []);
    bus.emit("command send", "status");
    assert.equal(current.sent.at(-1).controls.value, "status");
});

test("logout ignores late messages and cancels reconnects", () => {
    const {bus, connections, timers} = setup("http:");
    const logs = [], states = [];
    bus.on("gamelog", line => logs.push(line));
    bus.on("connection_state", state => states.push(state));
    bus.connect();
    assert.equal(connections[0].url, "ws://manager.example.test:8080/ws");
    const {onmessage, onopen} = connections[0];
    bus.disconnect();
    states.length = 0;
    onmessage({data: JSON.stringify({room_name: "gamelog", message: "after logout"})});
    onopen();
    assert.deepEqual(logs, []);
    assert.deepEqual(states, []);
    assert.equal(timers.size, 0);
});

test("a failed websocket send reports the command failure without throwing or replaying it", () => {
    const {bus, connections} = setup();
    const failures = [];
    bus.on("command_error", value => failures.push(value));
    bus.connect();
    connections[0].open();
    connections[0].failSend = true;
    assert.doesNotThrow(() => bus.emit("command send", "save"));
    assert.equal(failures.length, 1);
    assert.equal(bus.sendCommand("save"), false);
    assert.equal(failures.length, 2);
});

test("a successful command send reports that the command was accepted by the socket", () => {
    const {bus, connections} = setup();
    bus.connect();
    connections[0].open();
    assert.equal(bus.sendCommand("/help"), true);
    assert.deepEqual(connections[0].sent.at(-1).controls, {type: "command", value: "/help"});
});

test("malformed websocket JSON is reported without preventing the next valid message", () => {
    const {bus, connections} = setup();
    const logs = [], errors = [];
    bus.on("protocol_error", error => errors.push(error));
    bus.on("gamelog", line => logs.push(line));
    bus.connect();
    connections[0].open();
    connections[0].onmessage({data: "{"});
    connections[0].onmessage({data: JSON.stringify({room_name: "gamelog", message: "ok"})});
    assert.equal(errors.length, 1);
    assert.deepEqual(logs, ["ok"]);
});
