import React, {useCallback, useEffect, useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faDownload, faFloppyDisk, faHardDrive, faRotate, faTrashAlt} from "@fortawesome/free-solid-svg-icons";
import savesResource from "../../../../api/resources/saves";
import Panel from "../../../components/Panel";
import Alert from "../../../components/Alert";
import Button from "../../../components/Button";
import Checkbox from "../../../components/Checkbox";
import Input from "../../../components/Input";
import Label from "../../../components/Label";
import Error from "../../../components/Error";
import IconButton from "../../../components/IconButton";
import EmptyState from "../../../components/EmptyState";
import ConfirmDialog from "../../../components/ConfirmDialog";
import ScopeBadge from "../../../components/ScopeBadge";

const defaultSettings = {
    interval_enabled: false,
    interval_minutes: 30,
    last_player_enabled: false,
    clean_stop_enabled: false,
    retention_count: 10
};

const triggerLabels = {
    manual: "Manual",
    interval: "Timed interval",
    "last-player": "Last player left",
    "clean-stop": "Clean shutdown"
};

const formatSize = bytes => {
    if (!Number.isFinite(bytes)) return "—";
    const megabytes = bytes / 1024 / 1024;
    return megabytes >= 100 ? `${megabytes.toFixed(0)} MB` : `${megabytes.toFixed(2)} MB`;
};

const Checkpoints = ({serverStatus, onRestore, canManage = false}) => {
    const [state, setState] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");
    const [isSavingSettings, setIsSavingSettings] = useState(false);
    const [isCreating, setIsCreating] = useState(false);
    const [restoringID, setRestoringID] = useState("");
    const [checkpointToDelete, setCheckpointToDelete] = useState(null);
    const {register, handleSubmit, reset, formState: {errors}} = useForm({defaultValues: defaultSettings});

    const applyState = useCallback((result, syncSettings = false) => {
        setState(result);
        if (syncSettings && result?.settings) reset(result.settings);
    }, [reset]);

    const load = useCallback(async () => {
        setIsLoading(true);
        setLoadError("");
        try {
            applyState(await savesResource.checkpoints.list(), true);
        } catch (error) {
            setLoadError("Checkpoint data and schedule settings could not be loaded.");
        } finally {
            setIsLoading(false);
        }
    }, [applyState]);

    useEffect(() => { load(); }, [load]);

    useEffect(() => {
        if (!state) return undefined;
        const timer = window.setInterval(() => {
            savesResource.checkpoints.list().then(setState).catch(() => undefined);
        }, 30000);
        return () => window.clearInterval(timer);
    }, [Boolean(state)]);

    const saveSettings = async values => {
        setIsSavingSettings(true);
        try {
            const settings = {
                ...values,
                interval_minutes: Number(values.interval_minutes),
                retention_count: Number(values.retention_count)
            };
            applyState(await savesResource.checkpoints.settings(settings), true);
            window.flash("Checkpoint schedule saved.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Checkpoint settings could not be saved.", "red");
        } finally {
            setIsSavingSettings(false);
        }
    };

    const create = async () => {
        setIsCreating(true);
        try {
            applyState(await savesResource.checkpoints.create());
            window.flash("Fixed checkpoint created and verified.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Checkpoint could not be created.", "red");
        } finally {
            setIsCreating(false);
        }
    };

    const restore = async checkpoint => {
        setRestoringID(checkpoint.id);
        try {
            const save = await savesResource.checkpoints.restore(checkpoint);
            await Promise.resolve(onRestore?.());
            window.flash(`${save?.name || "Checkpoint"} restored as a new save.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "Checkpoint could not be restored.", "red");
        } finally {
            setRestoringID("");
        }
    };

    const remove = async checkpoint => {
        applyState(await savesResource.checkpoints.delete(checkpoint));
        window.flash("Checkpoint deleted.", "green");
    };

    const checkpoints = state?.checkpoints || [];
    const statusUnknown = serverStatus?.known === false;
    const busy = !canManage || statusUnknown || isLoading || Boolean(loadError) || !state || isCreating || isSavingSettings || Boolean(restoringID) || serverStatus?.stopping;

    return <><Panel
        className="mb-5"
        title="Fixed checkpoints"
        description="Keep verified, profile-specific world snapshots outside Factorio's rotating autosaves. Existing checkpoints are never overwritten."
        headerAction={<div className="flex flex-wrap items-center justify-end gap-2">
            <ScopeBadge/>
            <span className="ui-status-badge">{state ? checkpoints.length : "—"} stored</span>
        </div>}
        content={<>
            {loadError && <Alert type="danger" className="mb-5">
                <div className="flex flex-wrap items-center gap-3">
                    <span>{loadError} Existing settings are locked until they can be read.</span>
                    <Button type="secondary" size="sm" onClick={load} isLoading={isLoading}>Retry</Button>
                </div>
            </Alert>}
            {state?.last_error && <Alert type="danger" className="mb-5">
                <strong>Last background checkpoint failed.</strong><br/>{state.last_error}
            </Alert>}

            {canManage && <form id="checkpoint-settings" onSubmit={handleSubmit(saveSettings)}>
                <fieldset disabled={busy}>
                <div className="grid grid-cols-1 xl:grid-cols-2 gap-4 mb-5">
                    <div className="ui-subcard p-4">
                        <Checkbox text="Create a checkpoint on a running-time interval" register={register("interval_enabled")}/>
                        <div className="mt-3 max-w-xs">
                            <Label text="Interval in minutes" htmlFor="interval_minutes"/>
                            <Input type="number" min={5} max={10080} disabled={isSavingSettings} register={register("interval_minutes", {
                                required: true,
                                valueAsNumber: true,
                                min: 5,
                                max: 10080
                            })}/>
                            <Error error={errors.interval_minutes} message="Choose an interval between 5 minutes and 7 days."/>
                        </div>
                    </div>
                    <div className="ui-subcard p-4 space-y-3">
                        <Checkbox text="Create a checkpoint when the last confirmed player leaves" help="Uses Factorio's live connected-player count." register={register("last_player_enabled")}/>
                        <Checkbox text="Create a checkpoint before a clean server shutdown" help="A force stop cannot create a safe checkpoint." register={register("clean_stop_enabled")}/>
                    </div>
                </div>
                <div className="flex flex-col md:flex-row md:items-end gap-4 pb-5 border-b border-white border-opacity-5">
                    <div className="w-full md:max-w-xs">
                        <Label text="Maximum stored checkpoints" htmlFor="retention_count" help="0 keeps every checkpoint. A reduced limit is applied only after a replacement snapshot is verified."/>
                        <Input type="number" min={0} max={1000} disabled={isSavingSettings} register={register("retention_count", {
                            required: true,
                            valueAsNumber: true,
                            min: 0,
                            max: 1000
                        })}/>
                        <Error error={errors.retention_count} message="Use 0 to keep all, or choose up to 1000."/>
                    </div>
                    <Button isSubmit form="checkpoint-settings" isLoading={isSavingSettings} isDisabled={busy}>Save schedule</Button>
                    <Button type="success" onClick={create} isLoading={isCreating} isDisabled={busy}>
                        <FontAwesomeIcon icon={faFloppyDisk}/> Create checkpoint now
                    </Button>
                </div>
                </fieldset>
            </form>}

            <div className="mt-5">
                {isLoading
                    ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faHardDrive} spin/><p className="mt-3">Loading checkpoints…</p></div></div>
                    : loadError
                        ? null
                    : checkpoints.length === 0
                        ? <EmptyState icon={faFloppyDisk} title="No fixed checkpoints yet"/>
                        : <div className="ui-table-wrap">
                            <table className="ui-table">
                                <thead><tr><th>Created</th><th>Trigger</th><th>Source save</th><th>Size</th><th className="text-right">Actions</th></tr></thead>
                                <tbody>{checkpoints.map(checkpoint => <tr key={checkpoint.id}>
                                    <td>{new Date(checkpoint.created_at).toLocaleString()}</td>
                                    <td><span className="ui-status-badge">{triggerLabels[checkpoint.trigger] || checkpoint.trigger}</span></td>
                                    <td className="max-w-xs truncate" title={checkpoint.source_save}>{checkpoint.source_save || "—"}</td>
                                    <td>{formatSize(checkpoint.size)}</td>
                                    <td><div className="flex justify-end gap-2">
                                        <a className="ui-icon-button" href={`/api/checkpoints/${encodeURIComponent(checkpoint.id)}/download`} aria-label="Download checkpoint" title="Download checkpoint">
                                            <FontAwesomeIcon icon={faDownload}/>
                                        </a>
                                        {canManage && <IconButton
                                            label="Restore as a new save"
                                            icon={faRotate}
                                            spin={restoringID === checkpoint.id}
                                            disabled={busy || serverStatus?.running}
                                            onClick={() => restore(checkpoint)}
                                        />}
                                        {canManage && <IconButton
                                            type="danger"
                                            label="Delete checkpoint"
                                            icon={faTrashAlt}
                                            disabled={busy}
                                            onClick={() => setCheckpointToDelete(checkpoint)}
                                        />}
                                    </div></td>
                                </tr>)}</tbody>
                            </table>
                        </div>}
            </div>
        </>}
    />
        {canManage && <ConfirmDialog
            title="Delete fixed checkpoint?"
            content="This removes the selected checkpoint permanently. Normal saves and other checkpoints are not affected."
            isOpen={Boolean(checkpointToDelete)}
            close={() => setCheckpointToDelete(null)}
            onSuccess={() => remove(checkpointToDelete)}
        />}
    </>;
};

export default Checkpoints;
