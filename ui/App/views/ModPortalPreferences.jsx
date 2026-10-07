import React, {useCallback, useEffect, useState} from "react";
import {useForm} from "react-hook-form";
import modsResource from "../../api/resources/mods";
import Panel from "../components/Panel";
import Checkbox from "../components/Checkbox";
import Button from "../components/Button";
import Alert from "../components/Alert";
import ScopeBadge from "../components/ScopeBadge";

const ModPortalPreferences = ({canManage, onDirtyChange}) => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState("");
    const {register, reset, handleSubmit, formState: {isDirty}} = useForm({defaultValues: {preselect_optional: false}});
    const load = useCallback(async () => {
        setLoading(true);
        setError("");
        try { reset(await modsResource.portal.preferences()); }
        catch { setError("Mod Portal preferences could not be loaded."); }
        finally { setLoading(false); }
    }, [reset]);
    useEffect(() => { load(); }, [load]);
    useEffect(() => { onDirtyChange(isDirty); }, [isDirty, onDirtyChange]);
    const save = async values => {
        setSaving(true);
        try {
            reset(await modsResource.portal.setPreferences(Boolean(values.preselect_optional)));
            window.flash("Mod Portal preferences saved.", "green");
        } catch { window.flash("Mod Portal preferences could not be saved.", "red"); }
        finally { setSaving(false); }
    };
    return <form id="mod-portal-preferences-form" className="mb-5" onSubmit={handleSubmit(save)}>
        <Panel title="Mod Portal" headerAction={<ScopeBadge scope="manager"/>}
            help="Applies to new dependency reviews for all profiles. Existing mods and already-open reviews are not changed."
            content={<>
                {error && <Alert type="danger" className="mb-4"><div className="flex items-center gap-3"><span>{error}</span><Button type="secondary" size="sm" onClick={load}>Retry</Button></div></Alert>}
                <fieldset disabled={!canManage || loading || saving || Boolean(error)}>
                    <Checkbox text="Preselect optional and recommended dependencies"
                        help="Checks the available optional/recommended entries from the selected mod and its required dependencies. You can change the selection before downloading. Additional optional integrations remain unchecked."
                        register={register("preselect_optional")}/>
                </fieldset>
            </>}
            actions={canManage ? <Button isSubmit isLoading={saving} isDisabled={loading || Boolean(error) || !isDirty}>Save Mod Portal preferences</Button> : null}/>
    </form>;
};

export default ModPortalPreferences;
