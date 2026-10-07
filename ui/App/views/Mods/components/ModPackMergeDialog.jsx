import React, {useEffect, useState} from "react";
import modsResource from "../../../../api/resources/mods";
import Modal from "../../../components/Modal";
import Button from "../../../components/Button";
import Alert from "../../../components/Alert";
import helpers from "./modPackMerge.cjs";

const ModPackMergeDialog = ({name, isOpen, close, onSuccess, locked}) => {
    const [plan, setPlan] = useState(null);
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState("");
    const [reload, setReload] = useState(0);
    useEffect(() => {
        if (!isOpen) return;
        let active = true;
        setPlan(null);
        setError("");
        setLoading(true);
        modsResource.packs.previewMerge(name).then(result => {
            if (active) setPlan(result);
        }).catch(failure => {
            if (active) setError(failure?.response?.data?.error || "The pack could not be checked.");
        }).finally(() => { if (active) setLoading(false); });
        return () => { active = false; };
    }, [name, isOpen, reload]);

    const merge = async () => {
        setSaving(true);
        setError("");
        try {
            await modsResource.packs.merge(name, plan.revision);
        } catch (failure) {
            setPlan(null);
            setError(failure?.response?.data?.error || "The pack could not be added. Review it again before retrying.");
            setSaving(false);
            return;
        }
        try {
            await onSuccess();
            window.flash(`${name} added. Existing mods and settings were kept.`, "green");
        } catch {
            window.flash(`${name} was added, but the mod list could not be refreshed. Reload the page.`, "yellow");
        } finally { setSaving(false); close(); }
    };
    const list = (title, items) => <section>
        <h3 className="font-bold text-white mb-2">{title} · {items.length}</h3>
        <ul className="space-y-2">{items.map(item => <li className="ui-subcard p-3 flex justify-between gap-3" key={item.name}>
            <span className="break-all">{item.name} <span className="text-gray-light font-mono">{item.version}</span></span>
            <span className="text-gray-light">{item.enabled ? "Enabled" : "Disabled"}</span>
        </li>)}</ul>
    </section>;
    return <Modal isOpen={isOpen} title={`Add ${name} to profile`} close={close} dismissDisabled={saving}
        content={<div className="max-h-[65vh] overflow-y-auto space-y-4 pr-1">
            <p className="text-gray-light">Existing mods, game mode and settings are kept. Settings saved in this pack are not imported.</p>
            {locked && <Alert type="warning">Stop Factorio before adding a pack.</Alert>}
            {error && <Alert type="danger">{error}</Alert>}
            {loading && <p role="status">Checking mods and dependencies…</p>}
            {plan && <>
                {plan.conflicts?.length > 0 && <Alert type="danger"><ul className="space-y-2">{plan.conflicts.map((item, index) => <li key={`${item.name}-${index}`}><strong>{item.name}:</strong> {item.reason}</li>)}</ul></Alert>}
                {list("To add", plan.additions || [])}
                {plan.kept?.length > 0 && list("Already installed · kept", plan.kept)}
                {!plan.additions?.length && !plan.conflicts?.length && <p>All mods in this pack are already installed.</p>}
            </>}
        </div>}
        actions={<>
            <Button type="secondary" size="sm" onClick={close} isDisabled={saving}>Cancel</Button>
            <Button type="secondary" size="sm" onClick={() => setReload(value => value + 1)} isDisabled={saving || loading || locked}>Recheck</Button>
            <Button size="sm" onClick={merge} isLoading={saving} isDisabled={!helpers.canMergePack(plan, loading || saving, locked)}>Add mods</Button>
        </>}/>
};

export default ModPackMergeDialog;
