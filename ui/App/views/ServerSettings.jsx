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

const ServerSettings = ({serverStatus}) => {
    const {activeProfile, applyProfileState} = useProfiles();
    const [settings, setSettings] = useState(null);
    const [saves, setSaves] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isLoadingStartup, setIsLoadingStartup] = useState(true);
    const [isSavingStartup, setIsSavingStartup] = useState(false);
    const [isSavingAutostart, setIsSavingAutostart] = useState(false);
    const [isSavingMapSnapshots, setIsSavingMapSnapshots] = useState(false);
    const locked = Boolean(serverStatus?.running || serverStatus?.stopping);

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
        try {
            const result = await settingsResource.server.list();
            setSettings(result);
            reset(result);
            return result;
        } catch (error) {
            window.flash(error?.response?.data || "Settings could not be loaded.", "red");
        } finally {
            setIsLoading(false);
        }
    }, [reset]);

    const fetchStartup = useCallback(async () => {
        if (!activeProfile) return;
        setIsLoadingStartup(true);
        try {
            const [availableSaves, autostart, mapSnapshot] = await Promise.all([
                savesResource.list(false),
                serverResource.autostart(),
                serverResource.mapSnapshot()
            ]);
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
            resetAutostart({enabled: Boolean(autostart?.enabled)});
            resetMapSnapshot({interval_minutes: Number(mapSnapshot?.settings?.interval_minutes ?? 60)});
        } catch (error) {
            window.flash(error?.response?.data || "Startup settings could not be loaded.", "red");
        } finally {
            setIsLoadingStartup(false);
        }
    }, [activeProfile, resetAutostart, resetMapSnapshot, resetStartup]);

    useEffect(() => { fetchSettings(); }, [fetchSettings]);
    useEffect(() => { fetchStartup(); }, [fetchStartup]);

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

    return <>
        <PageHeader
            title="Server settings"
            actions={hasUnsavedChanges ? <span className="ui-status-badge ui-status-badge--warning">Unsaved changes</span> : null}
        />
        {locked && <Alert type="warning" className="mb-5"><FontAwesomeIcon icon={faLock}/> Stop Factorio to edit profile startup, network and multiplayer settings. Autostart remains configurable.</Alert>}

        <div className="ui-settings-overview-grid mb-5">
            <form id="startup-settings-form" onSubmit={handleStartupSubmit(saveStartup)}>
                <Panel
                    className="h-full"
                    title="Startup & network"
                    help="The bind address selects local interfaces. 0.0.0.0 is normally correct in Docker. The host port mapping is configured outside this manager."
                    headerAction={<ScopeBadge/>}
                    content={<fieldset className="ui-settings-fields" disabled={locked || isLoadingStartup}>
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
                    </fieldset>}
                    actions={<Button form="startup-settings-form" isSubmit isLoading={isSavingStartup} isDisabled={locked || isLoadingStartup || !startupDirty}>
                        <FontAwesomeIcon icon={faNetworkWired}/> Save startup configuration
                    </Button>}
                />
            </form>

            <form id="autostart-settings-form" onSubmit={handleAutostartSubmit(saveAutostart)}>
                <Panel
                    className="h-full"
                    title="Autostart"
                    help="Starts the active profile with its configured bind address, port and selected save when the manager container starts. It does not start or stop Factorio immediately."
                    headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge scope="manager"/><span className={`ui-status-badge ${autostartEnabled ? "ui-status-badge--running" : "ui-status-badge--stopped"}`}>{autostartEnabled ? "Enabled" : "Disabled"}</span></div>}
                    content={<div className="ui-startup-setting">
                        <div className="ui-startup-setting__icon"><FontAwesomeIcon icon={faPowerOff}/></div>
                        <Checkbox text="Start Factorio with this manager" register={registerAutostart("enabled")}/>
                    </div>}
                    actions={<Button form="autostart-settings-form" isSubmit isLoading={isSavingAutostart} isDisabled={isLoadingStartup || !autostartDirty}>
                        <FontAwesomeIcon icon={faPowerOff}/> Save autostart
                    </Button>}
                />
            </form>
        </div>

        <form id="map-snapshot-settings-form" className="mb-5" onSubmit={handleMapSnapshotSubmit(saveMapSnapshots)}>
            <Panel
                title="Map snapshots"
                help="Creates a dashboard image from a temporary save copy with an isolated exporter. Set 0 to keep manual generation only."
                headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge scope="manager"/><span className={`ui-status-badge ${mapSnapshotInterval > 0 ? "ui-status-badge--running" : "ui-status-badge--stopped"}`}>{mapSnapshotInterval > 0 ? `Every ${mapSnapshotInterval} min` : "Manual only"}</span></div>}
                content={<div className="ui-settings-fields">
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
                </div>}
                actions={<Button form="map-snapshot-settings-form" isSubmit isLoading={isSavingMapSnapshots} isDisabled={isLoadingStartup || !mapSnapshotDirty}>
                    <FontAwesomeIcon icon={faMap}/> Save map interval
                </Button>}
            />
        </form>

        <form id="server-settings-form" onSubmit={handleSubmit(saveServerSettings)}>
            <Panel
                title="Multiplayer configuration"
                help="Factorio server-settings.json for the active profile, including visibility, passwords, player limits and autosaves."
                headerAction={<ScopeBadge/>}
                content={isLoading
                    ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faServer} spin/><p className="mt-3">Loading settings…</p></div></div>
                    : <fieldset className="ui-settings-fields" disabled={locked}>
                        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">{fields.map(key => field(key, settings[key]))}</div>
                    </fieldset>}
            />
            {isDirty && <aside className="ui-unsaved-bar" role="status" aria-live="polite">
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
