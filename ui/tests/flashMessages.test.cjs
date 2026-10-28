const {test} = require("node:test");
const assert = require("node:assert/strict");
const EventEmitter = require("node:events");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

test("the global flash renderer safely handles structured messages emitted on the bus", async t => {
    const bus = new EventEmitter();
    const {Flash} = loadModule("App/components/Flash.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faCheckCircle: {}, faCircleExclamation: {}, faCircleInfo: {}, faXmark: {}},
        "../../notifications": bus,
        "../../api/messageText.cjs": require("../api/messageText.cjs")
    }, {window: {setTimeout: () => 1, clearTimeout: () => {}}});

    let root;
    await act(async () => { root = create(React.createElement(Flash)); });
    t.after(() => act(() => root.unmount()));

    await act(async () => {
        bus.emit("flash", {message: {error: {details: "private backend data"}}, color: "red"});
    });
    assert.match(JSON.stringify(root.toJSON()), /The request could not be completed\./);
    assert.doesNotMatch(JSON.stringify(root.toJSON()), /private backend data/);
});
