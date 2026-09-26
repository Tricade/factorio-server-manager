import React, {useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faUpload} from "@fortawesome/free-solid-svg-icons";
import backups from "../../api/resources/backups";
import Button from "./Button";
import Modal from "./Modal";
import Alert from "./Alert";
import {importSelection, validImportSelection} from "./backupSelection.cjs";

const BackupRestore = ({kind = "profiles", disabled, activeProfile, onComplete}) => {
    const [open, setOpen] = useState(false);
    const [file, setFile] = useState(null);
    const [preview, setPreview] = useState(null);
    const [selection, setSelection] = useState([]);
    const [busy, setBusy] = useState(false);
    const [confirmed, setConfirmed] = useState(false);
    const [error, setError] = useState("");
    const [targetID, setTargetID] = useState(null);
    const profiles = kind === "profiles";
    const reset = () => { setFile(null); setPreview(null); setSelection([]); setConfirmed(false); setError(""); setTargetID(activeProfile?.id); };
    const showError = error => setError(typeof error?.response?.data === "string" ? error.response.data : "The backup could not be processed. Try again.");
    const inspect = async () => {
        setBusy(true); setError(""); setPreview(null); setConfirmed(false);
        try {
            const result = await (profiles ? backups.previewProfiles(file) : backups.previewMods(file));
            setPreview(result);
            setSelection((result.profiles || []).map(profile => ({...profile, selected: true})));
            setTargetID(activeProfile?.id);
        } catch (error) { showError(error); } finally { setBusy(false); }
    };
    const restore = async () => {
        setBusy(true); setError("");
        try {
            const result = await (profiles ? backups.importProfiles(file, importSelection(selection)) : backups.restoreMods(file, targetID));
            // A refresh failure must not invite a second, already committed import.
            setOpen(false);
            window.flash(profiles ? "Backup imported as new inactive profiles." : "Mod backup restored.", "green");
            Promise.resolve(onComplete(result)).catch(() => window.flash("Restore completed. Refresh this page to reload the list.", "yellow"));
        } catch (error) { showError(error); } finally { setBusy(false); }
    };
    const targetChanged = !profiles && targetID !== activeProfile?.id;
    return <>
        <Button type="secondary" isDisabled={disabled} onClick={() => { reset(); setOpen(true); }}>
            <FontAwesomeIcon icon={faUpload}/> {profiles ? "Import backup" : "Restore mod backup"}
        </Button>
        <Modal isOpen={open} title={profiles ? "Import profiles" : "Restore mod backup"} close={() => setOpen(false)} dismissDisabled={busy}
            content={<div className="space-y-4">
                <label className="block font-bold" htmlFor={`backup-file-${kind}`}>Backup ZIP</label>
                <label className="ui-file-input px-4" htmlFor={`backup-file-${kind}`}>
                    <FontAwesomeIcon className="text-orange" icon={faUpload}/>
                    <span className="truncate">{file?.name || "Choose a backup ZIP…"}</span>
                    <input id={`backup-file-${kind}`} className="absolute inset-0 opacity-0 cursor-pointer" type="file" accept=".zip" disabled={busy || disabled}
                        onChange={event => { setFile(event.target.files?.[0] || null); setPreview(null); setConfirmed(false); setError(""); }}/>
                </label>
                {!profiles && <p className="text-sm text-gray-light">Choose a ZIP created by Mods → Download all. Single mods use Upload archive.</p>}
                {error && <Alert type="danger">{error}</Alert>}
                {(disabled || targetChanged) && <Alert type="warning">Restore is locked. Stop Factorio and reopen this dialog for the active profile.</Alert>}
                {preview && profiles && <>
                    <Alert type="info">New inactive profiles only. Existing profiles, game version and autostart stay unchanged. Passwords and account credentials are not transferred; imported servers start with public and LAN listing off.</Alert>
                    <div className="space-y-3 max-h-80 overflow-y-auto">
                        {selection.map((profile, index) => <div key={profile.id} className="rounded-xl border border-gray-dark p-3">
                            <label className="ui-checkbox"><input type="checkbox" checked={profile.selected} disabled={busy}
                                onChange={event => setSelection(current => current.map((item, i) => i === index ? {...item, selected: event.target.checked} : item))}/>
                                <span>{profile.name}</span></label>
                            <label className="block text-sm mt-2" htmlFor={`import-name-${profile.id}`}>New profile name</label>
                            <input id={`import-name-${profile.id}`} className="ui-input mt-1" value={profile.name} maxLength={64} disabled={busy || !profile.selected}
                                onChange={event => setSelection(current => current.map((item, i) => i === index ? {...item, name: event.target.value} : item))}/>
                            <p className="text-sm text-gray-light mt-2">{profile.installed_version} · {profile.game_mode} · {profile.save_count} saves · {profile.mod_count} mods · {profile.checkpoints?.length || 0} checkpoints</p>
                        </div>)}
                    </div>
                    {!validImportSelection(selection) && <p role="status" className="text-sm text-yellow">Select at least one profile and use unique, non-empty names.</p>}
                </>}
                {preview && !profiles && <>
                    <Alert type="warning">Replaces all mods and mod settings in {activeProfile?.name || "the active profile"}. Saves and the installed Factorio version stay unchanged. Download the current mods first if you need to undo this.</Alert>
                    <p className="text-sm">{preview.mods.length} mod archives · {preview.game_mode} · Startup settings: {preview.has_settings ? "included" : "not included"}</p>
                    <ul className="max-h-64 overflow-y-auto space-y-1 text-sm">{preview.mods.map(mod => <li key={`${mod.name}-${mod.version}`}>
                        {mod.name} {mod.version} · Factorio {mod.factorio_version} · {mod.enabled ? "enabled" : "disabled"}
                    </li>)}</ul>
                    <label className="ui-checkbox"><input type="checkbox" checked={confirmed} disabled={busy} onChange={event => setConfirmed(event.target.checked)}/> Replace this profile’s current mods and settings</label>
                </>}
            </div>}
            actions={<>
                <Button type="ghost" isDisabled={busy} onClick={() => setOpen(false)}>Cancel</Button>
                {!preview ? <Button isDisabled={!file || disabled} isLoading={busy} onClick={inspect}>Preview backup</Button>
                    : <Button isDisabled={disabled || targetChanged || (profiles ? !validImportSelection(selection) : !confirmed)} isLoading={busy} onClick={restore}>
                        {profiles ? "Import selected profiles" : "Restore mods"}
                    </Button>}
            </>}/>
    </>;
};

export default BackupRestore;
