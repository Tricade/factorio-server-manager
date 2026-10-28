const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

const deferred = () => {
    let resolve;
    const promise = new Promise(yes => { resolve = yes; });
    return {promise, resolve};
};

const buttonText = button => button.children.filter(child => typeof child === "string").join("");

test("individual startup-setting reset controls stay disabled while a save is pending", async t => {
    const saveRequest = deferred();
    const settings = {
        revision: "rev-1",
        groups: [{mod: "sample-mod", title: "Sample mod", settings: [{
            name: "sample-enabled",
            display_name: "Sample enabled",
            type: "bool-setting",
            value: false,
            default_value: false
        }]}]
    };
const Button = ({children, isDisabled, isLoading, onClick, ...props}) => React.createElement("button", {
        ...props, disabled: Boolean(isDisabled || isLoading), onClick
    }, children);
    const Panel = ({content, actions}) => React.createElement("section", null, content, actions);
    const NullComponent = () => null;
    const {default: ModStartupSettings} = loadModule("App/views/Mods/components/ModStartupSettings.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faFloppyDisk: {}, faMagnifyingGlass: {}, faRotateLeft: {}, faSliders: {}},
        "../../../../api/resources/mods": {startupSettings: {
            get: async () => settings,
            update: () => saveRequest.promise
        }},
        "../../../components/Panel": Panel,
        "../../../components/Alert": NullComponent,
        "../../../components/Button": Button,
        "../../../components/EmptyState": NullComponent,
        "../../../components/HelpTip": NullComponent,
        "../../../components/ScopeBadge": NullComponent,
        "../modStartupSettings.cjs": require("../App/views/Mods/modStartupSettings.cjs")
    }, {window: {flash: () => {}}});

    let root;
    await act(async () => { root = create(React.createElement(ModStartupSettings, {profileId: "profile-1", statusLocked: false})); });
    t.after(() => act(() => root.unmount()));
    await act(async () => { await Promise.resolve(); });

    const settingControl = root.root.findByType("input");
    await act(async () => { settingControl.props.onChange({target: {checked: true}}); });
    const buttons = root.root.findAllByType("button");
    const saveButton = buttons.find(button => buttonText(button).includes("Save settings"));
    let savePromise;
    act(() => { savePromise = saveButton.props.onClick(); });

    const individualReset = root.root.findAllByType("button").find(button => button.props.title === "Reset Sample enabled to its default");
    assert.equal(individualReset.props.disabled, true);

    await act(async () => {
        saveRequest.resolve({...settings, revision: "rev-2", groups: [{...settings.groups[0], settings: [{...settings.groups[0].settings[0], value: true}]}]});
        await savePromise;
    });
});
