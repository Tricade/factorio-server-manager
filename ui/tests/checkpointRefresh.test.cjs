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

const checkpointState = (sourceSave, settings = {}) => ({
    checkpoints: sourceSave ? [{id: sourceSave, source_save: sourceSave, created_at: "2026-01-01T00:00:00Z", size: 1, trigger: "manual"}] : [],
    settings
});

test("a late checkpoint poll cannot overwrite a completed checkpoint mutation", async t => {
    const listRequests = [];
    const createRequest = deferred();
    const intervals = [];
    const savesResource = {checkpoints: {
        list() {
            const request = deferred();
            listRequests.push(request);
            return request.promise;
        },
        create: () => createRequest.promise
    }};
    const Button = ({children, onClick, isSubmit, form, ...props}) => React.createElement("button", {...props, onClick, type: isSubmit ? "submit" : "button"}, children);
    const IconButton = ({onClick, label}) => React.createElement("button", {onClick, "aria-label": label});
    const Panel = ({content}) => React.createElement("section", null, content);
    const NullComponent = () => null;
    const {default: Checkpoints} = loadModule("App/views/Saves/components/Checkpoints.jsx", {
        react: React,
        "react-hook-form": require("react-hook-form"),
        "@fortawesome/react-fontawesome": {FontAwesomeIcon: () => null},
        "@fortawesome/free-solid-svg-icons": {faDownload: {}, faFloppyDisk: {}, faHardDrive: {}, faRotate: {}, faTrashAlt: {}},
        "../../../../api/resources/saves": savesResource,
        "../../../components/Panel": Panel,
        "../../../components/Alert": NullComponent,
        "../../../components/Button": Button,
        "../../../components/Checkbox": NullComponent,
        "../../../components/Input": NullComponent,
        "../../../components/Label": NullComponent,
        "../../../components/Error": NullComponent,
        "../../../components/IconButton": IconButton,
        "../../../components/EmptyState": NullComponent,
        "../../../components/ConfirmDialog": NullComponent,
        "../../../components/ScopeBadge": NullComponent
    }, {window: {
        setInterval(callback) { intervals.push(callback); return intervals.length; },
        clearInterval() {},
        flash() {}
    }});

    let root;
    await act(async () => { root = create(React.createElement(Checkpoints, {canManage: true, serverStatus: {known: true, running: false}})); });
    t.after(() => act(() => root.unmount()));
    assert.equal(listRequests.length, 1);
    await act(async () => { listRequests[0].resolve(checkpointState("before")); });
    assert.equal(intervals.length, 1);

    await act(async () => { intervals[0](); });
    assert.equal(listRequests.length, 2);
    let createPromise;
    await act(async () => {
        const createButton = root.root.findAllByType("button").find(button => button.children
            .filter(child => typeof child === "string")
            .join("")
            .includes("Create checkpoint now"));
        createPromise = createButton.props.onClick();
    });
    await act(async () => {
        createRequest.resolve(checkpointState("after"));
        await createPromise;
    });
    await act(async () => { listRequests[1].resolve(checkpointState("before")); });

    assert.match(JSON.stringify(root.toJSON()), /after/);
    assert.doesNotMatch(JSON.stringify(root.toJSON()), /before/);
});
