const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

test("structured game-settings errors render a useful string instead of crashing the route", async t => {
    const flashes = [];
    const settingsResource = {game: {list: async () => {
        throw {response: {data: {error: {details: "internal"}}}};
    }}};
    const passThrough = ({children}) => React.createElement("div", null, children);
    const {default: GameSettings} = loadModule("App/views/GameSettings.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faGamepad: {}, faRotate: {}},
        "../../api/resources/settings": settingsResource,
        "../../api/messageText.cjs": require("../api/messageText.cjs"),
        "../components/PageHeader": () => null,
        "../components/Panel": ({content}) => React.createElement("section", null, content),
        "../components/EmptyState": () => null,
        "../components/Alert": passThrough,
        "../components/Button": ({children}) => React.createElement("button", null, children),
        "../components/ScopeBadge": () => null
    }, {window: {flash: message => flashes.push(message)}});

    let root;
    await act(async () => { root = create(React.createElement(GameSettings)); });
    t.after(() => act(() => root.unmount()));

    assert.match(JSON.stringify(root.toJSON()), /Game configuration could not be loaded\./);
    assert.deepEqual(flashes, ["Game configuration could not be loaded."]);
});
