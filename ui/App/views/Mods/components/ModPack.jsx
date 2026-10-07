import React, {useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faDownload, faPlay, faTrashAlt} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../api/resources/mods";
import ModList from "./ModList";
import ConfirmDialog from "../../../components/ConfirmDialog";
import Button from "../../../components/Button";
import ButtonLink from "../../../components/ButtonLink";
import ModPackMergeDialog from "./ModPackMergeDialog";

const ModPack = ({modPack, reloadModPacks, factorioVersion, reloadMods, profileLocked = false, readOnly = false}) => {
    const [dialog, setDialog] = useState(null);
    const [isLoading, setIsLoading] = useState(false);

    const deleteModPack = async () => {
        await modsResource.packs.delete(modPack.name);
        await reloadModPacks();
        window.flash(`${modPack.name} deleted.`, "green");
    };
    const toggleMod = modName => modsResource.packs.mods.toggle(modPack.name, modName).then(reloadModPacks);
    const updateMod = version => modsResource.packs.mods.update(modPack.name, version).then(reloadModPacks);
    const deleteMod = modName => modsResource.packs.mods.delete(modPack.name, modName).then(reloadModPacks);
    const loadModPack = async () => {
        setIsLoading(true);
        try {
            await modsResource.packs.load(modPack.name);
            await reloadMods();
            window.flash(`${modPack.name} loaded.`, "green");
        } finally {
            setIsLoading(false);
        }
    };

    return <div className="ui-subcard overflow-hidden">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 px-4 py-3 border-b border-white border-opacity-5">
            <div><h3 className="font-bold text-white">{modPack.name}</h3><p className="text-xs text-gray-light">{modPack.mods?.mods?.length || 0} mods</p></div>
            <div className="flex flex-wrap gap-2">
                <ButtonLink size="sm" type="ghost" href={`/api/mods/packs/${encodeURIComponent(modPack.name)}/download`}>
                    <FontAwesomeIcon icon={faDownload}/> Download
                </ButtonLink>
                {!readOnly && <Button size="sm" type="secondary" isDisabled={profileLocked || isLoading} title={profileLocked ? "Stop Factorio before adding mods" : undefined} onClick={() => setDialog("merge")}>Add to profile</Button>}
                {!readOnly && <Button size="sm" type="secondary" isLoading={isLoading} isDisabled={profileLocked} title={profileLocked ? "Stop Factorio before replacing the active profile's mods" : undefined} onClick={() => setDialog("load")}><FontAwesomeIcon icon={faPlay}/> Replace mods</Button>}
                {!readOnly && <Button size="sm" type="danger" onClick={() => setDialog("delete")}><FontAwesomeIcon icon={faTrashAlt}/> Delete</Button>}
            </div>
        </div>
        <div className="p-3"><ModList
            mods={modPack.mods?.mods || []}
            factorioVersion={factorioVersion}
            toggleMod={toggleMod}
            updateMod={updateMod}
            deleteMod={deleteMod}
            disabled={readOnly}
        /></div>
        {!readOnly && <ConfirmDialog
            title={dialog === "delete" ? "Delete mod pack?" : "Replace profile mods?"}
            content={dialog === "delete"
                ? `${modPack.name} will be removed. Installed mods are not changed.`
                : `Loading ${modPack.name} replaces the active profile's entire mod set and stored mod settings.`}
            isOpen={dialog === "delete" || dialog === "load"}
            close={() => setDialog(null)}
            onSuccess={dialog === "delete" ? deleteModPack : loadModPack}
        />}
        {!readOnly && <ModPackMergeDialog name={modPack.name} isOpen={dialog === "merge"} close={() => setDialog(null)} onSuccess={reloadMods} locked={profileLocked}/>}
    </div>;
};

export default ModPack;
