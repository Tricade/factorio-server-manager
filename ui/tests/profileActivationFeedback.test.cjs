const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

const buttonText = button => button.children.filter(child => typeof child === "string").join("");

test("profile activation is not reported as failed when only status refresh fails", async t => {
    const flashes = [];
    const profiles = [
        {id: "profile-a", name: "A", active: true, game_mode: "factorio", save_count: 1, mod_count: 0},
        {id: "profile-b", name: "B", active: false, game_mode: "factorio", save_count: 1, mod_count: 0}
    ];
    const profileState = {active_profile_id: "profile-a", profiles};
    let activateProfile = async () => profileState;
    const Button = ({children, isDisabled, isLoading, isSubmit, onClick, ...props}) => React.createElement("button", {
        ...props, disabled: Boolean(isDisabled || isLoading), type: isSubmit ? "submit" : "button", onClick
    }, children);
    const Panel = ({content, actions}) => React.createElement("section", null, content, actions);
    const NullComponent = () => null;
    const refreshProfiles = async () => profileState;
    const {default: Profiles} = loadModule("App/views/Profiles.jsx", {
        react: React,
        "react-hook-form": require("react-hook-form"),
        "react-router-dom": {useNavigate: () => () => {}, useSearchParams: () => [new URLSearchParams(), () => {}]},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {
            faBoxArchive: {}, faCheck: {}, faClone: {}, faFloppyDisk: {}, faLayerGroup: {},
            faPen: {}, faPlay: {}, faPlus: {}, faPuzzlePiece: {}, faTrash: {}, faXmark: {}
        },
        "../../api/resources/profiles": {activate: (...args) => activateProfile(...args)},
        "../../api/messageText.cjs": require("../api/messageText.cjs"),
        "../components/PageHeader": NullComponent,
        "../components/Panel": Panel,
        "../components/Button": Button,
        "../components/Input": NullComponent,
        "../components/Select": NullComponent,
        "../components/Label": NullComponent,
        "../components/Error": NullComponent,
        "../components/Alert": NullComponent,
        "../components/EmptyState": NullComponent,
        "../components/ConfirmDialog": NullComponent,
        "../context/ProfileContext": {useProfiles: () => ({
            state: profileState,
            activeProfile: profiles[0],
            isLoading: false,
            applyProfileState() {},
            refreshProfiles
        })},
        "../components/ProfileBackupExport": NullComponent,
        "../components/BackupRestore": NullComponent
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(Profiles, {
        canManage: true,
        serverStatus: {known: true, running: false, stopping: false},
        refreshServerStatus: async () => { throw new Error("status service unavailable"); }
    })); });
    t.after(() => act(() => root.unmount()));

    const activateButton = root.root.findAllByType("button").find(button => buttonText(button).includes("Activate"));
    await act(async () => { await activateButton.props.onClick(); });
    assert.deepEqual(flashes, [{
        message: "B is active, but Factorio's current status could not be refreshed.",
        color: "yellow"
    }]);

    activateProfile = async () => { throw {response: {data: {error: {details: "private backend data"}}}}; };
    await act(async () => { await activateButton.props.onClick(); });
    assert.deepEqual(flashes[1], {message: "B could not be activated.", color: "red"});
});
