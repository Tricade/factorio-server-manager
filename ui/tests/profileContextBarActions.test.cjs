const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

test("force-stop success closes cleanly when only the follow-up status refresh fails", async t => {
    const flashes = [];
    let dialogProps;
    const Button = ({children, onClick, isDisabled, ...props}) => React.createElement("button", {
        ...props, onClick, disabled: isDisabled
    }, children);
    const ConfirmDialog = props => { dialogProps = props; return null; };
    const {default: ProfileContextBar} = loadModule("App/components/ProfileContextBar.jsx", {
        react: React,
        "react-router-dom": {useLocation: () => ({pathname: "/"})},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {
            faCloudArrowDown: {}, faFloppyDisk: {}, faGamepad: {}, faLayerGroup: {}, faPlay: {},
            faRotate: {}, faSkullCrossbones: {}, faStop: {}
        },
        "../../api/resources/server": {kill: async () => {}},
        "./Button": Button,
        "./ConfirmDialog": ConfirmDialog,
        "../context/ProfileContext": {useProfiles: () => ({
            activeProfile: {id: "profile-1", name: "Test", game_mode: "factorio", installed_version: "2.1", release_target: "stable", save_count: 1, selected_save: "world.zip"},
            isLoading: false,
            error: "",
            refreshProfiles: async () => {}
        })}
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(ProfileContextBar, {
        canManage: true,
        serverStatus: {known: true, running: true, stopping: false},
        refreshServerStatus: async () => { throw new Error("status service unavailable"); }
    })); });
    t.after(() => act(() => root.unmount()));

    let result;
    await act(async () => { result = dialogProps.onSuccess(); await result; });
    assert.equal(await result, undefined);
    assert.deepEqual(flashes, [{
        message: "Force stop completed, but Factorio's current status could not be refreshed.",
        color: "yellow"
    }]);
});
