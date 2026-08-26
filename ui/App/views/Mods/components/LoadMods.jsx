import React, {useEffect, useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faFileImport} from "@fortawesome/free-solid-svg-icons";
import savesResource from "../../../../api/resources/saves";
import modResource from "../../../../api/resources/mods";
import Select from "../../../components/Select";
import Label from "../../../components/Label";
import Button from "../../../components/Button";
import FactorioLogin from "./AddMod/components/FactorioLogin";
import ConfirmDialog from "../../../components/ConfirmDialog";
import Alert from "../../../components/Alert";
import saveModImportFeedbackHelpers from "./saveModImportFeedback.cjs";

const {saveModImportFeedback} = saveModImportFeedbackHelpers;

const LoadMods = ({refreshMods}) => {
    const [saves, setSaves] = useState([]);
    const {register, reset, handleSubmit} = useForm();
    const [isApplying, setIsApplying] = useState(false);
    const [isFactorioAuthenticated, setIsFactorioAuthenticated] = useState(false);
    const [loadModsData, setLoadModsData] = useState(undefined);

    useEffect(() => {
        let active = true;
        Promise.all([modResource.portal.status(), savesResource.list()])
            .then(([authenticated, availableSaves]) => {
                if (!active) return;
                setIsFactorioAuthenticated(authenticated);
                setSaves(availableSaves || []);
                reset({save: availableSaves?.[0]?.name || ""});
            })
            .catch(error => window.flash(error?.response?.data || "Save metadata could not be loaded.", "red"));
        return () => { active = false; };
    }, [reset]);

    const loadMods = async data => {
        setIsApplying(true);
        try {
            const result = await savesResource.importMods(data.save);
            await refreshMods();
            const feedback = saveModImportFeedback(data.save, result);
            window.flash(feedback.message, feedback.color);
        } catch (error) {
            window.flash(error?.response?.data || "Mods could not be loaded from this save.", "red");
        } finally {
            setIsApplying(false);
            setLoadModsData(undefined);
        }
    };

    if (!isFactorioAuthenticated) return <FactorioLogin setIsFactorioAuthenticated={setIsFactorioAuthenticated}/>;

    return <form onSubmit={handleSubmit(setLoadModsData)}>
        <Alert type="warning" className="mb-4">Importing replaces every currently installed mod with the enabled mods recorded in the selected save. Unavailable releases or mismatched Mod Portal archives are skipped and reported. Other preparation failures leave the current set active.</Alert>
        <div className="mb-4">
            <Label text="Source save" htmlFor="save"/>
            <Select register={register("save", {required: true})} disabled={saves.length === 0 || isApplying} options={saves.map(save => ({name: save.name, value: save.name}))}/>
        </div>
        <Button isSubmit isDisabled={saves.length === 0} isLoading={isApplying}><FontAwesomeIcon icon={faFileImport}/> Review import</Button>
        <ConfirmDialog
            title="Replace installed mods?"
            content={`Importing from "${loadModsData?.save || ""}" prepares an atomic replacement before activating it. Unavailable releases or mismatched archives will be skipped.`}
            isOpen={Boolean(loadModsData)}
            close={() => setLoadModsData(undefined)}
            onSuccess={() => loadMods(loadModsData)}
        />
    </form>;
};

export default LoadMods;
