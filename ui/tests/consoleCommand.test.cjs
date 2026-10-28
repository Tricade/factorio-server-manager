const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

test("console preserves a command when the socket rejects the send", async t => {
    const socket = {on() {}, off() {}, emit() {}, sendCommand: () => false};
    const Button = ({children, isDisabled, isSubmit, ...props}) => React.createElement("button", {
        ...props, disabled: isDisabled, type: isSubmit ? "submit" : "button"
    }, children);
    const Panel = ({content}) => React.createElement("section", null, content);
    const NullComponent = () => null;
    const {default: Console} = loadModule("App/views/Console.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faPaperPlane: {}, faTerminal: {}},
        "../../api/socket": socket,
        "../components/PageHeader": () => null,
        "../components/Panel": Panel,
        "../components/Button": Button,
        "../components/Alert": NullComponent,
        "../components/LogViewport": NullComponent,
        "../components/ScopeBadge": NullComponent
    }, {window: {}});

    let root;
    await act(async () => { root = create(React.createElement(Console, {
        serverStatus: {running: true, stopping: false}, socketState: "connected"
    })); });
    t.after(() => act(() => root.unmount()));

    const input = root.root.findByType("input");
    await act(async () => { input.props.onChange({target: {value: "/save"}}); });
    const form = root.root.findByType("form");
    await act(async () => { form.props.onSubmit({preventDefault() {}}); });

    assert.equal(root.root.findByType("input").props.value, "/save");
});
