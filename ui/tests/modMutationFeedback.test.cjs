const {test} = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const {act, create} = require("react-test-renderer");
const loadModule = require("./helpers/loadModule.cjs");

const feedback = require("../App/views/Mods/components/modMutationFeedback.cjs");
const buttonText = button => button.children.filter(child => typeof child === "string").join("");
const Button = ({children, isDisabled, isLoading, isSubmit, onClick, ...props}) => React.createElement("button", {
    ...props, disabled: Boolean(isDisabled || isLoading), type: isSubmit ? "submit" : "button", onClick
}, children);
const NullComponent = () => null;
const makeForm = ({name = "new-pack", file = {name: "sample.zip"}, onReset = () => {}} = {}) => ({
    register: () => ({}),
    reset: onReset,
    formState: {errors: {}},
    handleSubmit: callback => event => {
        event?.preventDefault?.();
        return callback({name, mod_file: [file]});
    }
});

test("create mod pack closes and warns when the pack was created but refresh failed", async t => {
    const flashes = [], resets = [];
    const {default: CreateModPack} = loadModule("App/views/Mods/components/CreateModPack.jsx", {
        react: React,
        "react-hook-form": {useForm: () => makeForm({onReset: () => resets.push(true)})},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faPlus: {}},
        "../../../../api/resources/mods": {packs: {create: async () => {}}},
        "../../../components/Button": Button,
        "../../../components/Modal": ({isOpen, content, actions}) => isOpen ? React.createElement("div", null, content, actions) : null,
        "../../../components/Label": NullComponent,
        "../../../components/Input": NullComponent,
        "../../../components/Error": NullComponent,
        "./modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(CreateModPack, {onSuccess: async () => { throw new Error("list unavailable"); }})); });
    t.after(() => act(() => root.unmount()));
    act(() => root.root.findAllByType("button")[0].props.onClick());
    await act(async () => { await root.root.findByType("form").props.onSubmit({preventDefault() {}}); });

    assert.equal(resets.length, 1);
    assert.equal(root.root.findAllByType("form").length, 0);
    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});

test("mod upload clears the committed file selection and warns when refresh failed", async t => {
    const flashes = [], resets = [];
    const {default: UploadMod} = loadModule("App/views/Mods/components/UploadMod.jsx", {
        react: React,
        "react-hook-form": {useForm: () => makeForm({name: "", onReset: () => resets.push(true)})},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faCloudArrowUp: {}},
        "../../../../api/resources/mods": {upload: async () => {}},
        "../../../components/Button": Button,
        "../../../components/Label": NullComponent,
        "../../../components/Error": NullComponent,
        "./modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(UploadMod, {refetchInstalledMods: async () => { throw new Error("list unavailable"); }})); });
    t.after(() => act(() => root.unmount()));
    await act(async () => { await root.root.findByType("form").props.onSubmit({preventDefault() {}}); });

    assert.equal(resets.length, 1);
    assert.match(JSON.stringify(root.toJSON()), /Choose a mod archive/);
    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});

test("portal installation closes dependency review and warns instead of reporting install failure", async t => {
    const flashes = [];
    let selectVersionProps, dependencyProps;
    const release = {version: "1.0.0", compatibility: true};
    const modResult = {item: {title: "Sample Mod", name: "sample-mod"}};
    const form = {
        register: () => ({}),
        watch: () => "Sample Mod",
        setValue() {},
        handleSubmit: callback => event => { event?.preventDefault?.(); return callback(); },
        formState: {errors: {}}
    };
    const {default: AddModForm} = loadModule("App/views/Mods/components/AddMod/components/AddModForm.jsx", {
        react: React,
        "react-hook-form": {useForm: () => form},
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faArrowRightFromBracket: {}, faExternalLinkAlt: {}, faMagnifyingGlass: {}, faSpinner: {}},
        "../../../../../../api/resources/mods": {portal: {
            info: async () => ({releases: [release]}),
            planInstall: async () => ({optional: [], conflicts: []}),
            installResolved: async () => {}
        }},
        "../../../../../components/Button": Button,
        "../../../../../components/ButtonLink": NullComponent,
        "../../../../../components/Label": NullComponent,
        "../../../../../components/Input": NullComponent,
        "../../../../../components/Error": NullComponent,
        "./SelectVersionForm": props => {
            selectVersionProps = props;
            return props.isOpen ? React.createElement("button", {onClick: () => props.review(release).then(props.close)}, "Review version") : null;
        },
        "./DependencyInstallForm": props => {
            dependencyProps = props;
            return props.isOpen ? React.createElement("button", {onClick: props.onInstall}, "Install reviewed set") : null;
        },
        "../../modPackMerge.cjs": require("../App/views/Mods/components/modPackMerge.cjs"),
        "../../modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color}), setTimeout: callback => { callback(); return 1; }, clearTimeout() {}}});

    const fuse = {search: () => [modResult]};
    let root;
    await act(async () => { root = create(React.createElement(AddModForm, {
        setIsFactorioAuthenticated() {}, fuse, factorioVersion: "2.1", refetchInstalledMods: async () => { throw new Error("list unavailable"); }
    })); });
    t.after(() => act(() => root.unmount()));
    const resultButton = root.root.findByProps({className: "ui-search-result is-active"});
    await act(async () => { resultButton.props.onClick(); });
    await act(async () => { await root.root.findByType("form").props.onSubmit({preventDefault() {}}); });
    await act(async () => { await selectVersionProps.review(release); });
    assert.equal(dependencyProps.isOpen, true);
    await act(async () => { await dependencyProps.onInstall(); });

    assert.equal(dependencyProps.isOpen, false);
    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});

test("replacing mods from a pack resolves the confirmation after commit if refresh fails", async t => {
    const flashes = [];
    let dialogProps;
    const {default: ModPack} = loadModule("App/views/Mods/components/ModPack.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faDownload: {}, faPlay: {}, faTrashAlt: {}},
        "../../../../api/resources/mods": {packs: {load: async () => {}}},
        "./ModList": NullComponent,
        "../../../components/ConfirmDialog": props => { dialogProps = props; return null; },
        "../../../components/Button": Button,
        "../../../components/ButtonLink": NullComponent,
        "./ModPackMergeDialog": NullComponent,
        "./modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(ModPack, {
        modPack: {name: "pack-a", mods: {mods: []}},
        reloadModPacks: async () => {},
        reloadMods: async () => { throw new Error("installed list unavailable"); }
    })); });
    t.after(() => act(() => root.unmount()));
    const replaceButton = root.root.findAllByType("button").find(button => buttonText(button).includes("Replace mods"));
    act(() => replaceButton.props.onClick());
    let completed = false;
    await act(async () => { await dialogProps.onSuccess(); completed = true; });

    assert.equal(completed, true);
    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});

test("deleting a mod pack resolves confirmation when only the pack-list refresh fails", async t => {
    const flashes = [];
    let dialogProps;
    const {default: ModPack} = loadModule("App/views/Mods/components/ModPack.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faDownload: {}, faPlay: {}, faTrashAlt: {}},
        "../../../../api/resources/mods": {packs: {
            delete: async () => {},
            load: async () => {}
        }},
        "./ModList": NullComponent,
        "../../../components/ConfirmDialog": props => { dialogProps = props; return null; },
        "../../../components/Button": Button,
        "../../../components/ButtonLink": NullComponent,
        "./ModPackMergeDialog": NullComponent,
        "./modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(ModPack, {
        modPack: {name: "pack-a", mods: {mods: []}},
        reloadModPacks: async () => { throw new Error("pack list unavailable"); },
        reloadMods: async () => {}
    })); });
    t.after(() => act(() => root.unmount()));
    const deleteButton = root.root.findAllByType("button").find(button => buttonText(button).includes("Delete"));
    act(() => deleteButton.props.onClick());
    let completed = false;
    await act(async () => { await dialogProps.onSuccess(); completed = true; });

    assert.equal(completed, true);
    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});

test("inline mod-pack actions warn when the pack mutation succeeded but refresh failed", async t => {
    const flashes = [];
    const IconButton = ({label, onClick}) => React.createElement("button", {onClick, "aria-label": label});
    const {default: Mod} = loadModule("App/views/Mods/components/Mod.jsx", {
        react: React,
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faArrowUp: {}, faCheck: {}, faSpinner: {}, faTimes: {}, faToggleOff: {}, faToggleOn: {}, faTrashAlt: {}},
        "semver": require("semver"),
        "../../../../api/resources/mods": {portal: {info: async () => ({releases: []})}},
        "../../../components/IconButton": IconButton,
        "./modMutationFeedback.cjs": feedback
    }, {window: {flash: (message, color) => flashes.push({message, color})}});

    let root;
    await act(async () => { root = create(React.createElement(Mod, {
        mod: {name: "sample-mod", title: "Sample Mod", enabled: false, version: "1.0.0", factorio_version: "2.1"},
        factorioVersion: "2.1.0",
        toggleMod: async () => ({refreshed: false}),
        deleteMod: async () => ({refreshed: false}),
        updateMod: async () => ({refreshed: false}),
        addUpdatableMod() {}
    })); });
    t.after(() => act(() => root.unmount()));
    const toggle = root.root.findByProps({"aria-label": "Enable Sample Mod"});
    await act(async () => { await toggle.props.onClick(); });

    assert.deepEqual(flashes, [{message: feedback.refreshFailureMessage, color: "yellow"}]);
});
