const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

const buttonText = button => button.children.filter(child => typeof child === "string").join("");

test("release install is not reported as failed when only status refresh fails", async t => {
    const flashes = [];
    const server = {
        releaseStatus: async () => ({installed_target: "stable", installed_channel: "stable", installed_version: "2.1.14", stable_version: "2.1.14", available_versions: []}),
        gameMode: async () => ({mode: "factorio", space_age_available: true, features: []}),
        installRelease: async () => ({message: "Factorio version installed", installed_version: "2.1.14"})
    };
    const Button = ({children, isDisabled, isLoading, onClick, ...props}) => React.createElement("button", {
        ...props, disabled: Boolean(isDisabled || isLoading), onClick
    }, children);
    const Panel = ({content, actions}) => React.createElement("section", null, content, actions);
    const NullComponent = () => null;
    const {default: Releases} = loadModule("App/views/Releases.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faCloudArrowDown: {}, faFlask: {}, faIndustry: {}, faRocket: {}, faShieldHalved: {}, faTag: {}},
        "../../api/resources/server": server,
        "../components/PageHeader": NullComponent,
        "../components/Panel": Panel,
        "../components/Button": Button,
        "../components/Alert": NullComponent,
        "../components/HelpTip": NullComponent,
        "../components/Label": NullComponent,
        "../components/Error": NullComponent,
        "../components/ScopeBadge": NullComponent,
        "../context/ProfileContext": {useProfiles: () => ({
            activeProfile: {id: "profile-1", release_target: "stable"},
            refreshProfiles: async () => {}
        })}
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(Releases, {
        canManage: true,
        serverStatus: {known: true, running: false, stopping: false},
        refreshServerStatus: async () => { throw new Error("status service unavailable"); }
    })); });
    t.after(() => act(() => root.unmount()));
    await act(async () => { await Promise.resolve(); });

    const installButton = root.root.findAllByType("button").find(button => buttonText(button).includes("Reinstall stable"));
    await act(async () => { await installButton.props.onClick(); });
    assert.deepEqual(flashes, [{
        message: "Factorio 2.1.14 was installed, but process status could not be refreshed.",
        color: "yellow"
    }]);
});
