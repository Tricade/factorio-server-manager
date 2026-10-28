const {test} = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const {transformSync} = require("@babel/core");

const code = transformSync(fs.readFileSync(path.join(__dirname, "../api/client.js"), "utf8"), {
    presets: [["@babel/preset-env", {targets: {node: "current"}, modules: "commonjs"}]]
}).code;

function loadClient() {
    const flashes = [], events = [];
    let reject;
    const axios = {create: () => ({interceptors: {response: {use: (_, callback) => { reject = callback; }}}})};
    const module = {exports: {}};
    const window = {flash: message => flashes.push(message), dispatchEvent: event => events.push(event.type)};
    new Function("require", "module", "exports", "window", "Event", code)(name => {
        if (name === "axios") return axios;
        if (name === "./messageText.cjs") return require("../api/messageText.cjs");
        throw new Error(`Unexpected dependency: ${name}`);
    }, module, module.exports, window, Event);
    return {reject, flashes, events};
}

test("backup transport errors use their actionable message, without manager-offline toast", async () => {
    const client = loadClient();
    const error = {config: {networkErrorMessage: "Backup upload interrupted. Check proxy limits."}};
    await assert.rejects(client.reject(error), value => value === error);
    assert.deepEqual(client.flashes, [error.config.networkErrorMessage]);
    assert.equal(error.userMessage, error.config.networkErrorMessage);
});

test("normal network, size-limit and authentication handling remains unchanged", async () => {
    for (const [error, flash] of [
        [{}, "The control service is not reachable. Check the connection."],
        [{response: {status: 413, data: "Backup exceeds 512 MiB"}}, "Backup exceeds 512 MiB"],
        [{response: {status: 502}}, "Service not available"]
    ]) {
        const client = loadClient();
        await assert.rejects(client.reject(error), value => value === error);
        assert.deepEqual(client.flashes, [flash]);
    }
    const client = loadClient();
    const error = {response: {status: 401}, config: {networkErrorMessage: "not an authentication message"}};
    await assert.rejects(client.reject(error), value => value === error);
    assert.deepEqual(client.flashes, []);
    assert.deepEqual(client.events, ["fsm:authentication-required"]);
});

test("cancelled requests do not report the control service as offline", async () => {
    const client = loadClient();
    const error = {code: "ERR_CANCELED"};
    await assert.rejects(client.reject(error), value => value === error);
    assert.deepEqual(client.flashes, []);
    assert.deepEqual(client.events, []);
});

test("unexpected structured API errors never reach React as objects", async () => {
    for (const data of [{error: {details: "internal"}}, {message: ["invalid"]}, null]) {
        const client = loadClient();
        const error = {response: {status: 500, data}};
        await assert.rejects(client.reject(error), value => value === error);
        assert.deepEqual(client.flashes, ["The request could not be completed."]);
    }
});
