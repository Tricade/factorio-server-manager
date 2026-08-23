import React, {useCallback, useEffect, useMemo, useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faFloppyDisk, faLock, faMap, faNetworkWired, faPowerOff, faServer} from "@fortawesome/free-solid-svg-icons";
import settingsResource from "../../api/resources/settings";
import savesResource from "../../api/resources/saves";
import serverResource from "../../api/resources/server";
import profilesResource from "../../api/resources/profiles";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Input from "../components/Input";
import Label from "../components/Label";
import Select from "../components/Select";
import Error from "../components/Error";
import Checkbox from "../components/Checkbox";
import InputPassword from "../components/InputPassword";
import Button from "../components/Button";
import Alert from "../components/Alert";
import ScopeBadge from "../components/ScopeBadge";
import {useProfiles} from "../context/ProfileContext";

const humanize = key => key.replaceAll("_", " ").replace(/\b\w/g, letter => letter.toUpperCase());

const ServerSettings = ({serverStatus, canManage = false}) => {
    const {activeProfile, applyProfileState} = useProfiles();
    const [settings, setSettings] = useState(null);
    const [saves, setSaves] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    const [settingsLoadError, setSettingsLoadError] = useState("");
    const [isSaving, setIsSaving] = useState(false);
    const [isLoadingStartup, setIsLoadingStartup] = useState(true);
    const [isLoadingAutostart, setIsLoadingAutostart] = useState(true);
    const [isLoadingMapSnapshots, setIsLoadingMapSnapshots] = useState(true);
    const [startupLoadError, setStartupLoadError] = useState("");
    const [autostartLoadError, setAutostartLoadError] = useState("");
    const [mapSnapshotLoadError, setMapSnapshotLoadError] = useState("");
    const [isSavingStartup, setIsSavingStartup] = useState(false);
    const [isSavingAutostart, setIsSavingAutostart] = useState(false);
    const [isSavingMapSnapshots, setIsSavingMapSnapshots] = useState(false);
    const locked = Boolean(!canManage || serverStatus?.known === false || serverStatus?.running || serverStatus?.stopping);

    const settingsForm = useForm();
    const startupForm = useForm({defaultValues: {bind_ip: "0.0.0.0", port: 34197, selected_save: ""}});
    const autostartForm = useForm({defaultValues: {enabled: false}});
    const mapSnapshotForm = useForm({defaultValues: {interval_minutes: 60}});
    const {register, handleSubmit, reset, formState: {isDirty}} = settingsForm;
    const {
        register: registerStartup,
        handleSubmit: handleStartupSubmit,
        reset: resetStartup,
        formState: {isDirty: startupDirty, errors: startupErrors}
    } = startupForm;
    const {
        register: registerAutostart,
        handleSubmit: handleAutostartSubmit,
        reset: resetAutostart,
        watch: watchAutostart,
        formState: {isDirty: autostartDirty}
    } = autostartForm;
    const {
        register: registerMapSnapshot,
        handleSubmit: handleMapSnapshotSubmit,
        reset: resetMapSnapshot,
        watch: watchMapSnapshot,
        formState: {isDirty: mapSnapshotDirty, errors: mapSnapshotErrors}
    } = mapSnapshotForm;

    const fetchSettings = useCallback(async () => {
        setIsLoading(true);
        setSettingsLoadError("");
        try {
            const result = await settingsResource.server.list();
            setSettings(result);
            reset(result);
            return result;
        } catch (error) {
            setSettings(null);
            setSettingsLoadError("Multiplayer settings could not be loaded.");
        } finally {
            setIsLoading(false);
        }
    }, [reset]);

    const fetchStartup = useCallback(async () => {
        if (!activeProfile) return;
        setIsLoadingStartup(true);
        setStartupLoadError("");
        try {
            const availableSaves = await savesResource.list(false);
            const orderedSaves = [...(availableSaves || [])].sort((left, right) => new Date(right.last_mod) - new Date(left.last_mod));
            setSaves(orderedSaves);
            const preferredSave = orderedSaves.find(save => save.name === activeProfile.selected_save)?.name
                || orderedSaves[0]?.name
                || "";
            resetStartup({
                bind_ip: activeProfile.bind_ip || "0.0.0.0",
                port: activeProfile.port || 34197,
                selected_save: preferredSave
            });
        } catch (error) {
            setSaves([]);
            setStartupLoadError("Available saves could not be loaded, so startup configuration is locked.");
        } finally {
            setIsLoadingStartup(false);
        }
    }, [activeProfile, resetStartup]);

    const fetchAutostart = useCallback(async () => {
        setIsLoadingAutostart(true);
        setAutostartLoadError("");
        try {
            const result = await serverResource.autostart();
            resetAutostart({enabled: Boolean(result?.enabled)});
        } catch (error) {
            setAutostartLoadError("Autostart status could not be loaded.");
        } finally {
            setIsLoadingAutostart(false);
        }
    }, [resetAutostart]);

    const fetchMapSnapshots = useCallback(async () => {
        setIsLoadingMapSnapshots(true);
        setMapSnapshotLoadError("");
        try {
            const result = await serverResource.mapSnapshot();
            resetMapSnapshot({interval_minutes: Number(result?.settings?.interval_minutes ?? 60)});
        } catch (error) {
            setMapSnapshotLoadError("Map snapshot settings could not be loaded.");
        } finally {
            setIsLoadingMapSnapshots(false);
        }
    }, [resetMapSnapshot]);

    useEffect(() => { fetchSettings(); }, [fetchSettings]);
    useEffect(() => { fetchStartup(); }, [fetchStartup]);
    useEffect(() => { fetchAutostart(); }, [fetchAutostart]);
    useEffect(() => { fetchMapSnapshots(); }, [fetchMapSnapshots]);

    const saveStartup = async values => {
        setIsSavingStartup(true);
        try {
            const state = await profilesResource.updateStartup(activeProfile.id, {
                bind_ip: values.bind_ip,
                port: Number(values.port),
                selected_save: values.selected_save || ""
            });
            applyProfileState(state);
            const refreshed = state.profiles.find(profile => profile.id === state.active_profile_id);
            resetStartup({
                bind_ip: refreshed?.bind_ip || values.bind_ip,
                port: refreshed?.port || Number(values.port),
                selected_save: refreshed?.selected_save || ""
            });
            window.flash("Startup configuration saved.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Startup configuration could not be saved.", "red");
        } finally {
            setIsSavingStartup(false);
        }
    };

    const saveAutostart = async values => {
        setIsSavingAutostart(true);
        try {
            const result = await serverResource.setAutostart(Boolean(values.enabled));
            resetAutostart({enabled: Boolean(result?.enabled)});
            window.flash(`Factorio autostart ${result?.enabled ? "enabled" : "disabled"}.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "Autostart setting could not be saved.", "red");
        } finally {
            setIsSavingAutostart(false);
        }
    };

    const saveMapSnapshots = async values => {
        setIsSavingMapSnapshots(true);
        try {
            const result = await serverResource.setMapSnapshotSettings(Number(values.interval_minutes));
            resetMapSnapshot({interval_minutes: Number(result?.interval_minutes ?? 60)});
            window.flash(result?.interval_minutes === 0 ? "Automatic map snapshots disabled." : `Map snapshot interval set to ${result.interval_minutes} minutes.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "Map snapshot interval could not be saved.", "red");
        } finally {
            setIsSavingMapSnapshots(false);
        }
    };

    const saveServerSettings = async formData => {
        const data = {...formData};
        Object.keys(settings || {}).forEach(key => {
            if (key.startsWith("_comment")) data[key] = settings[key];
            else if (Array.isArray(settings[key])) {
                data[key] = String(data[key] || "").split(",").map(value => value.trim()).filter(Boolean);
            } else if (typeof settings[key] === "object" && settings[key] !== null && data[key] === undefined) {
                data[key] = settings[key];
            }
        });

        setIsSaving(true);
        try {
            await settingsResource.server.update(data);
            await fetchSettings();
            window.flash("Server settings saved.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Settings could not be saved.", "red");
        } finally {
            setIsSaving(false);
        }
    };

    const fields = useMemo(() => settings ? Object.keys(settings).filter(key => !key.startsWith("_comment_")) : [], [settings]);

    const field = (name, value) => {
        const label = humanize(name);
        const comment = settings[`_comment_${name}`];
        let control;

        if (typeof value === "boolean") {
            control = <Checkbox checked={value} text={label} help={comment} register={register(name)}/>;
        } else if (typeof value === "number") {
            control = <><Label htmlFor={name} text={label} help={comment}/><Input type="number" register={register(name, {valueAsNumber: true})}/></>;
        } else if (typeof value === "string") {
            control = <><Label htmlFor={name} text={label} help={comment}/>{name.includes("password")
                ? <InputPassword register={register(name)}/>
                : <Input register={register(name)}/>}</>;
        } else if (Array.isArray(value)) {
            control = <><Label htmlFor={name} text={label} help={comment}/><Input register={register(name)} defaultValue={value.join(", ")}/></>;
        } else if (value && typeof value === "object" && name.includes("visibility")) {
            control = <><Label text="Visibility" help={comment}/><div className="flex flex-wrap gap-4 py-2">
                {Object.keys(value).map(key => <Checkbox key={key} checked={value[key]} register={register(`${name}.${key}`)} text={humanize(key)}/>) }
            </div></>;
        } else {
            return null;
        }

        return <div className="ui-subcard p-4" key={name}>{control}</div>;
    };

    const hasUnsavedChanges = isDirty || startupDirty || autostartDirty || mapSnapshotDirty;
    const autostartEnabled = Boolean(watchAutostart("enabled"));
    const mapSnapshotInterval = Number(watchMapSnapshot("interval_minutes") || 0);
    const autostartStatusClass = !isLoadingAutostart && !autostartLoadError
        ? autostartEnabled ? "ui-status-badge--running" : "ui-status-badge--stopped"
        : "";
    const autostartStatusLabel = isLoadingAutostart ? "Loading…" : autostartLoadError ? "Unavailable" : autostartEnabled ? "Enabled" : "Disabled";
    const mapSnapshotStatusClass = !isLoadingMapSnapshots && !mapSnapshotLoadError
        ? mapSnapshotInterval > 0 ? "ui-status-badge--running" : "ui-status-badge--stopped"
        : "";
    const mapSnapshotStatusLabel = isLoadingMapSnapshots ? "Loading…" : mapSnapshotLoadError ? "Unavailable" : mapSnapshotInterval > 0 ? `Every ${mapSnapshotInterval} min` : "Manual only";

    return <>
        <PageHeader
            title="Server settings"
            actions={hasUnsavedChanges ? <span className="ui-status-badge ui-status-badge--warning">Unsaved changes</span> : null}
        />
        {locked && <Alert type={canManage ? "warning" : "info"} className="mb-5"><FontAwesomeIcon icon={faLock}/> {!canManage
            ? "Viewer access is read-only. Profile and manager-wide settings remain visible."
            : serverStatus?.known === false
            ? "Profile startup and multiplayer settings are locked until the Factorio process status is confirmed. Manager-wide autostart and map scheduling remain configurable."
            : serverStatus?.stopping
                ? "Profile startup and multiplayer settings remain locked while Factorio is shutting down. Manager-wide autostart and map scheduling remain configurable."
                : "Stop Factorio to edit profile startup, network and multiplayer settings. Manager-wide autostart and map scheduling remain configurable."}</Alert>}

        <div className="ui-settings-overview-grid mb-5">
            <form id="startup-settings-form" onSubmit={handleStartupSubmit(saveStartup)}>
                <Panel
                    className="h-full"
                    title="Startup & network"
                    help="The bind address selects local interfaces. 0.0.0.0 is normally correct in Docker. The host port mapping is configured outside this manager."
                    headerAction={<ScopeBadge/>}
                    content={<>
                        {startupLoadError && <Alert type="danger" className="mb-4">
                            <div className="flex flex-wrap items-center gap-3"><span>{startupLoadError}</span><Button type="secondary" size="sm" onClick={fetchStartup}>Retry</Button></div>
                        </Alert>}
                    <fieldset className="ui-settings-fields" disabled={locked || isLoadingStartup || Boolean(startupLoadError)}>
                        <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                            <div className="ui-subcard p-4">
                                <Label text="Bind address" htmlFor="bind_ip" help="Use 0.0.0.0 to listen on every interface inside the container."/>
                                <Input register={registerStartup("bind_ip", {
                                    required: true,
                                    pattern: /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/
                                })}/>
                                <Error error={startupErrors.bind_ip} message="Enter a valid IPv4 address."/>
                            </div>
                            <div className="ui-subcard p-4">
                                <Label text="Game port" htmlFor="port" help="Factorio uses UDP; 34197 is the default container port."/>
                                <Input type="number" min={1} max={65535} register={registerStartup("port", {required: true, min: 1, max: 65535, valueAsNumber: true})}/>
                                <Error error={startupErrors.port} message="Port must be between 1 and 65535."/>
                            </div>
                            <div className="ui-subcard p-4 xl:col-span-2">
                                <Label text="Save to start" htmlFor="selected_save" help="The top-bar Start button loads this save. Change it here while Factorio is stopped."/>
                                <Select
                                    register={registerStartup("selected_save", {required: saves.length > 0})}
                                    disabled={saves.length === 0}
                                    options={saves.length > 0
                                        ? saves.map(save => ({value: save.name, name: save.name}))
                                        : [{value: "", name: "No save available"}]}
                                />
                                <Error error={startupErrors.selected_save} message="Choose a save file."/>
                            </div>
                        </div>
                    </fieldset></>}
                    actions={canManage ? <Button form="startup-settings-form" isSubmit isLoading={isSavingStartup} isDisabled={locked || isLoadingStartup || Boolean(startupLoadError) || !startupDirty}>
                        <FontAwesomeIcon icon={faNetworkWired}/> Save startup configuration
                    </Button> : null}
                />
            </form>

            <form id="autostart-settings-form" onSubmit={handleAutostartSubmit(saveAutostart)}>
                <Panel
                    className="h-full"
                    title="Autostart"
                    help="Starts the active profile with its configured bind address, port and selected save when the manager container starts. It does not start or stop Factorio immediately."
                    headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge scope="manager"/><span className={`ui-status-badge ${autostartStatusClass}`}>{autostartStatusLabel}</span></div>}
                    content={<>
                        {autostartLoadError && <Alert type="danger" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{autostartLoadError}</span><Button type="secondary" size="sm" onClick={fetchAutostart}>Retry</Button></div></Alert>}
                        <fieldset disabled={!canManage || isLoadingAutostart || Boolean(autostartLoadError)}><div className="ui-startup-setting">
                        <div className="ui-startup-setting__icon"><FontAwesomeIcon icon={faPowerOff}/></div>
                        <Checkbox text="Start Factorio with this manager" register={registerAutostart("enabled")}/>
                    </div></fieldset></>}
                    actions={canManage ? <Button form="autostart-settings-form" isSubmit isLoading={isSavingAutostart} isDisabled={isLoadingAutostart || Boolean(autostartLoadError) || !autostartDirty}>
                        <FontAwesomeIcon icon={faPowerOff}/> Save autostart
                    </Button> : null}
                />
            </form>
        </div>

        <form id="map-snapshot-settings-form" className="mb-5" onSubmit={handleMapSnapshotSubmit(saveMapSnapshots)}>
            <Panel
                title="Map snapshots"
                help="Creates a dashboard image from a temporary save copy with an isolated exporter. Set 0 to keep manual generation only."
                headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge scope="manager"/><span className={`ui-status-badge ${mapSnapshotStatusClass}`}>{mapSnapshotStatusLabel}</span></div>}
                content={<>
                    {mapSnapshotLoadError && <Alert type="danger" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{mapSnapshotLoadError}</span><Button type="secondary" size="sm" onClick={fetchMapSnapshots}>Retry</Button></div></Alert>}
                    <fieldset className="ui-settings-fields" disabled={!canManage || isLoadingMapSnapshots || Boolean(mapSnapshotLoadError)}>
                    <div className="ui-subcard p-4">
                        <Label text="Automatic snapshot interval" htmlFor="interval_minutes" help="Minutes between completed map images. 0 disables scheduled generation; the Overview button remains available."/>
                        <Input
                            type="number"
                            min={0}
                            max={10080}
                            step={5}
                            register={registerMapSnapshot("interval_minutes", {required: true, min: 0, max: 10080, valueAsNumber: true})}
                        />
                        <Error error={mapSnapshotErrors.interval_minutes} message="Use a value from 0 to 10080 minutes."/>
                    </div>
                </fieldset></>}
                actions={canManage ? <Button form="map-snapshot-settings-form" isSubmit isLoading={isSavingMapSnapshots} isDisabled={isLoadingMapSnapshots || Boolean(mapSnapshotLoadError) || !mapSnapshotDirty}>
                    <FontAwesomeIcon icon={faMap}/> Save map interval
                </Button> : null}
            />
        </form>

        <form id="server-settings-form" onSubmit={handleSubmit(saveServerSettings)}>
            <Panel
                title="Multiplayer configuration"
                help="Factorio server-settings.json for the active profile, including visibility, passwords, player limits and autosaves."
                headerAction={<ScopeBadge/>}
                content={isLoading
                    ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faServer} spin/><p className="mt-3">Loading settings…</p></div></div>
                    : settingsLoadError
                        ? <Alert type="danger"><div className="flex flex-wrap items-center gap-3"><span>{settingsLoadError}</span><Button type="secondary" size="sm" onClick={fetchSettings}>Retry</Button></div></Alert>
                    : <fieldset className="ui-settings-fields" disabled={locked}>
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">{fields.map(key => field(key, settings[key]))}</div>
                    </fieldset>}
            />
            {isDirty && settings && !settingsLoadError && <aside className="ui-unsaved-bar" role="status" aria-live="polite">
                <div><strong>Unsaved multiplayer changes</strong></div>
                <Button type="secondary" isDisabled={isSaving} onClick={() => reset(settings)}>Discard changes</Button>
                <Button form="server-settings-form" isSubmit isLoading={isSaving} isDisabled={isLoading || locked}>
                    <FontAwesomeIcon icon={faFloppyDisk}/> Save changes
                </Button>
            </aside>}
        </form>
    </>;
};

export default ServerSettings;
