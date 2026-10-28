const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

test("mod-pack preview errors render safe text instead of a structured API object", async t => {
    const modsResource = {packs: {previewMerge: async () => {
        throw {response: {data: {error: {details: "private backend data"}}}};
    }}};
    const Alert = ({children, type}) => React.createElement("div", {role: type === "danger" ? "alert" : undefined}, children);
    const Modal = ({content, actions}) => React.createElement("section", null, content, actions);
    const Button = ({children}) => React.createElement("button", null, children);
    const {default: ModPackMergeDialog} = loadModule("App/views/Mods/components/ModPackMergeDialog.jsx", {
        react: React,
        "../../../../api/resources/mods": modsResource,
        "../../../components/Modal": Modal,
        "../../../components/Button": Button,
        "../../../components/Alert": Alert,
        "./modPackMerge.cjs": require("../App/views/Mods/components/modPackMerge.cjs"),
        "../../../../api/messageText.cjs": require("../api/messageText.cjs")
    }, {window: {flash: () => {}}});

    let root;
    await act(async () => { root = create(React.createElement(ModPackMergeDialog, {
        name: "test-pack", isOpen: true, close: () => {}, onSuccess: async () => {}, locked: false
    })); });
    t.after(() => act(() => root.unmount()));

    const rendered = JSON.stringify(root.toJSON());
    assert.match(rendered, /The pack could not be checked\./);
    assert.doesNotMatch(rendered, /private backend data/);
});

test("save-list errors keep a plain-text alert and notification", async t => {
    const flashes = [];
    const refreshProfiles = async () => {};
    const NullComponent = () => null;
    const Alert = ({children}) => React.createElement("div", {role: "alert"}, children);
    const PageHeader = ({actions}) => React.createElement("header", null, actions);
    const Panel = ({content}) => React.createElement("section", null, content);
    const {default: Saves} = loadModule("App/views/Saves/Saves.jsx", {
        react: React,
        "react-router-dom": {Link: ({children}) => React.createElement("a", null, children)},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faDownload: {}, faHardDrive: {}, faPlus: {}, faRotate: {}, faTrashAlt: {}},
        "../../../api/resources/saves": {list: async () => {
            throw {response: {data: {error: {details: "private backend data"}}}};
        }},
        "../../components/PageHeader": PageHeader,
        "../../components/Panel": Panel,
        "./components/NewWorldForm": NullComponent,
        "./components/UploadSaveForm": NullComponent,
        "../../components/IconButton": NullComponent,
        "../../components/Alert": Alert,
        "../../components/ConfirmDialog": NullComponent,
        "./components/Checkpoints": NullComponent,
        "../../context/ProfileContext": {useProfiles: () => ({activeProfile: null, refreshProfiles})},
        "../../components/ScopeBadge": NullComponent,
        "../../components/EmptyState": NullComponent,
        "../../../api/messageText.cjs": require("../api/messageText.cjs")
    }, {window: {flash: message => flashes.push(message)}});

    let root;
    await act(async () => { root = create(React.createElement(Saves)); });
    t.after(() => act(() => root.unmount()));

    const rendered = JSON.stringify(root.toJSON());
    assert.match(rendered, /Saves could not be loaded\./);
    assert.doesNotMatch(rendered, /private backend data/);
    assert.deepEqual(flashes, ["Saves could not be loaded."]);
});
