import React, {useCallback, useEffect, useState} from "react";
import {Link} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faDownload, faHardDrive, faPlus, faRotate, faTrashAlt} from "@fortawesome/free-solid-svg-icons";
import savesResource from "../../../api/resources/saves";
import PageHeader from "../../components/PageHeader";
import Panel from "../../components/Panel";
import NewWorldForm from "./components/NewWorldForm";
import UploadSaveForm from "./components/UploadSaveForm";
import IconButton from "../../components/IconButton";
import Alert from "../../components/Alert";
import ConfirmDialog from "../../components/ConfirmDialog";
import Checkpoints from "./components/Checkpoints";
import {useProfiles} from "../../context/ProfileContext";

const formatSize = bytes => {
    if (!Number.isFinite(bytes)) return "—";
    const megabytes = bytes / 1024 / 1024;
    return megabytes >= 100 ? `${megabytes.toFixed(0)} MB` : `${megabytes.toFixed(2)} MB`;
};

const Saves = ({serverStatus}) => {
    const {activeProfile, refreshProfiles} = useProfiles();
    const [saves, setSaves] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");
    const [saveToDelete, setSaveToDelete] = useState(null);

    const updateList = useCallback(async () => {
        setIsLoading(true);
        setLoadError("");
        try {
            const result = await savesResource.list();
            setSaves(result || []);
            refreshProfiles().catch(() => undefined);
        } catch (error) {
            const message = error?.response?.data || "Saves could not be loaded.";
            setLoadError(typeof message === "string" ? message : "Saves could not be loaded.");
            window.flash(message, "red");
        } finally {
            setIsLoading(false);
        }
    }, [refreshProfiles]);

    useEffect(() => { updateList(); }, [updateList]);

    const deleteSave = async save => {
        await savesResource.delete(save);
        await updateList();
        window.flash(`${save.name} deleted.`, "green");
    };

    const saveTable = <div className="ui-table-wrap">
        <table className="ui-table">
            <thead><tr><th>Name</th><th>Last modified</th><th>Size</th><th className="text-right">Actions</th></tr></thead>
            <tbody>{saves.map(save => {
                const selected = save.name === activeProfile?.selected_save;
                return <tr key={save.name}>
                    <td><div className="flex flex-wrap items-center gap-2">
                        <span className="font-bold text-white">{save.name}</span>
                        {selected && <span className="ui-status-badge ui-status-badge--running">Selected</span>}
                    </div></td>
                    <td>{new Date(save.last_mod).toLocaleString()}</td>
                    <td>{formatSize(save.size)}</td>
                    <td><div className="flex justify-end gap-2">
                        <a className="ui-icon-button" href={`/api/saves/dl/${encodeURIComponent(save.name)}`} aria-label={`Download ${save.name}`} title="Download save">
                            <FontAwesomeIcon icon={faDownload}/>
                        </a>
                        <IconButton
                            type="danger"
                            label={`Delete ${save.name}`}
                            icon={faTrashAlt}
                            disabled={Boolean(serverStatus?.running || serverStatus?.stopping)}
                            onClick={() => setSaveToDelete(save)}
                        />
                    </div></td>
                </tr>;
            })}</tbody>
        </table>
    </div>;

    return <>
        <PageHeader
            title="Saves & checkpoints"
            actions={<div className="flex flex-wrap gap-2">
                <div className="ui-status-badge">{saves.length} {saves.length === 1 ? "save" : "saves"}</div>
                {!isLoading && !loadError && saves.length > 0 && <Link className="ui-button ui-button--success ui-button--sm" to="/profiles?new=fresh">
                    <FontAwesomeIcon icon={faPlus}/> New profile
                </Link>}
            </div>}
        />

        {serverStatus?.running && <Alert type="info" className="mb-5">
            Uploads and downloads stay available while Factorio is running. Creating or deleting worlds unlocks after the server stops.
        </Alert>}

        {isLoading && <Panel className="mb-5" content={<div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faHardDrive} spin/><p className="mt-3">Loading profile saves…</p></div></div>}/>}

        {!isLoading && loadError && <Panel className="mb-5" content={<div>
            <Alert type="danger" className="mb-4">{loadError}</Alert>
            <button className="ui-button ui-button--secondary" type="button" onClick={updateList}><FontAwesomeIcon icon={faRotate}/> Try again</button>
        </div>}/>}

        {!isLoading && !loadError && saves.length === 0 && <>
            <Panel
                className="mb-5"
                title="Create world"
                content={serverStatus?.running
                    ? <Alert type="warning">Stop Factorio before creating a new world.</Alert>
                    : <NewWorldForm onSuccess={updateList}/>}
            />
            <Panel
                className="mb-5"
                title="Upload existing save"
                content={<UploadSaveForm onSuccess={updateList}/>}
            />
        </>}

        {!isLoading && !loadError && saves.length > 0 && <>
            <Panel
                className="mb-5"
                title="Saves"
                content={saveTable}
            />

            <Checkpoints serverStatus={serverStatus} onRestore={updateList}/>

            <Panel
                className="mb-5"
                title="Upload save"
                content={<UploadSaveForm onSuccess={updateList}/>}
            />
        </>}

        <ConfirmDialog
            title="Delete save?"
            content={saveToDelete ? `${saveToDelete.name} will be permanently removed from ${activeProfile?.name || "the active profile"}. Other profiles are not changed.` : ""}
            isOpen={Boolean(saveToDelete)}
            close={() => setSaveToDelete(null)}
            onSuccess={() => deleteSave(saveToDelete)}
        />
    </>;
};

export default Saves;
