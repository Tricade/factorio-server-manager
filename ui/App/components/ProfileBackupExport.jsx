import React, {useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faDownload} from "@fortawesome/free-solid-svg-icons";
import backups from "../../api/resources/backups";
import Button from "./Button";
import Modal from "./Modal";
import Alert from "./Alert";

const ProfileBackupExport = ({profiles, disabled}) => {
    const [open, setOpen] = useState(false);
    const [ids, setIDs] = useState([]);
    const [saves, setSaves] = useState(true);
    const [checkpoints, setCheckpoints] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const download = async () => {
        setBusy(true); setError("");
        try {
            await backups.exportProfiles({profile_ids: ids, include_saves: saves, include_checkpoints: checkpoints});
            setOpen(false);
        } catch (error) {
            const response = error?.response?.data;
            setError(response instanceof Blob ? await response.text() : "Backup could not be created.");
        } finally { setBusy(false); }
    };
    return <>
        <Button type="secondary" isDisabled={disabled} onClick={() => { setIDs(profiles.map(profile => profile.id)); setError(""); setOpen(true); }}>
            <FontAwesomeIcon icon={faDownload}/> Export profiles
        </Button>
        <Modal isOpen={open} title="Export profiles" close={() => setOpen(false)} dismissDisabled={busy}
            content={<div className="space-y-4">
                <Alert type="info">Includes exact versions, mods, startup settings and server settings. Manager accounts, passwords, tokens and engine files are excluded. Saves and mod settings may contain player or custom mod data; keep backups private.</Alert>
                {error && <Alert type="danger">{error}</Alert>}
                {disabled && <Alert type="warning">Save and stop Factorio before exporting.</Alert>}
                <fieldset disabled={busy} className="space-y-3">
                    <legend className="font-bold mb-2">Profiles</legend>
                    <label className="ui-checkbox"><input type="checkbox" checked={ids.length === profiles.length} onChange={event => setIDs(event.target.checked ? profiles.map(profile => profile.id) : [])}/> Select all</label>
                    <div className="max-h-60 overflow-y-auto space-y-2">{profiles.map(profile => <label key={profile.id} className="ui-checkbox">
                        <input type="checkbox" checked={ids.includes(profile.id)} onChange={event => setIDs(current => event.target.checked ? [...current, profile.id] : current.filter(id => id !== profile.id))}/>{profile.name}
                    </label>)}</div>
                    <label className="ui-checkbox pt-3 border-t border-gray-dark"><input type="checkbox" checked={saves} onChange={event => setSaves(event.target.checked)}/> Include saves</label>
                    <label className="ui-checkbox"><input type="checkbox" checked={checkpoints} onChange={event => setCheckpoints(event.target.checked)}/> Include checkpoint files</label>
                </fieldset>
                <p className="text-sm text-gray-light">Checkpoint schedules are always included. Large backups need enough temporary disk space and an adequate FSM_MAX_UPLOAD limit on the destination.</p>
            </div>}
            actions={<><Button type="ghost" isDisabled={busy} onClick={() => setOpen(false)}>Cancel</Button><Button isDisabled={disabled || !ids.length} isLoading={busy} onClick={download}>Download backup</Button></>}/>
    </>;
};

export default ProfileBackupExport;
