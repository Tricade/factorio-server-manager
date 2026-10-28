const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

const deferred = () => {
    let reject;
    const promise = new Promise((resolve, no) => { reject = no; });
    return {promise, reject};
};

test("save metadata failures do not toast after the import tab has unmounted", async () => {
    const statusRequest = deferred();
    const savesRequest = deferred();
    const flashes = [];
    const NullComponent = () => null;
    const {default: LoadMods} = loadModule("App/views/Mods/components/LoadMods.jsx", {
        react: React,
        "react-hook-form": require("react-hook-form"),
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faFileImport: {}},
        "../../../../api/resources/saves": {list: () => savesRequest.promise, importMods: async () => ({})},
        "../../../../api/resources/mods": {portal: {status: () => statusRequest.promise}},
        "../../../components/Select": NullComponent,
        "../../../components/Label": NullComponent,
        "../../../components/Button": NullComponent,
        "./AddMod/components/FactorioLogin": NullComponent,
        "../../../components/ConfirmDialog": NullComponent,
        "../../../components/Alert": NullComponent,
        "./saveModImportFeedback.cjs": {saveModImportFeedback: () => ({message: "", color: "green"})}
    }, {window: {flash: message => flashes.push(message)}});

    let root;
    await act(async () => { root = create(React.createElement(LoadMods, {refreshMods: async () => {}})); });
    await act(async () => { root.unmount(); });
    await act(async () => {
        statusRequest.reject(new Error("late failure"));
        await Promise.resolve();
    });
    assert.deepEqual(flashes, []);
});
