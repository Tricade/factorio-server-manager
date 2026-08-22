import React, {useCallback, useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faDownload, faPuzzlePiece, faRotate, faTrashAlt} from "@fortawesome/free-solid-svg-icons";
import Fuse from "fuse.js";
import modsResource from "../../../api/resources/mods";
import server from "../../../api/resources/server";
import PageHeader from "../../components/PageHeader";
import Panel from "../../components/Panel";
import Button from "../../components/Button";
import ButtonLink from "../../components/ButtonLink";
import Alert from "../../components/Alert";
import EmptyState from "../../components/EmptyState";
import ConfirmDialog from "../../components/ConfirmDialog";
import TabControl from "../../components/Tabs/TabControl";
import Tab from "../../components/Tabs/Tab";
import AddMod from "./components/AddMod/AddMod";
import UploadMod from "./components/UploadMod";
import LoadMods from "./components/LoadMods";
import CreateModPack from "./components/CreateModPack";
import ModPack from "./components/ModPack";
import ModList from "./components/ModList";
import {useProfiles} from "../../context/ProfileContext";

const Mods = ({serverStatus}) => {
    const {activeProfile, refreshProfiles} = useProfiles();
    const [installedMods, setInstalledMods] = useState([]);
    const [modPacks, setModPacks] = useState([]);
    const [factorioVersion, setFactorioVersion] = useState(null);
    const [portalFactorioLine, setPortalFactorioLine] = useState(null);
    const [fuse, setFuse] = useState(undefined);
    const [isLoading, setIsLoading] = useState(true);
    const [isDeletingAllMods, setIsDeletingAllMods] = useState(false);
    const [isUpdatingAllMods, setIsUpdatingAllMods] = useState(false);
    const [isDeleteAllDialogOpen, setIsDeleteAllDialogOpen] = useState(false);
    const [updatableMods, setUpdatableMods] = useState([]);
    const disabled = Boolean(serverStatus?.running || serverStatus?.stopping);

    const addUpdatableMod = useCallback(mod => {
        setUpdatableMods(current => [...current.filter(existing => existing.modName !== mod.modName), mod]);
    }, []);

    const fetchInstalledMods = useCallback(async () => {
        const result = await modsResource.installed();
        setUpdatableMods([]);
        setInstalledMods(result || []);
        refreshProfiles().catch(() => undefined);
        return result;
    }, [refreshProfiles]);

    const fetchModPacks = useCallback(async () => {
        const result = await modsResource.packs.list();
        setModPacks(result || []);
        return result;
    }, []);

    useEffect(() => {
        let active = true;
        (async () => {
            let currentFactorioVersion = null;
            try {
                const version = await server.factorioVersion();
                if (!active) return;
                currentFactorioVersion = version.base_mod_version;
                setFactorioVersion(currentFactorioVersion);
                await Promise.all([fetchInstalledMods(), fetchModPacks()]);
            } catch (error) {
                window.flash(error?.response?.data || "Mods could not be loaded.", "red");
            } finally {
                if (active) setIsLoading(false);
            }

            try {
                const portal = await modsResource.portal.list();
                const expectedLine = (portal.factorio_version || currentFactorioVersion || "")
                    .split(".").slice(0, 2).join(".");
                const compatibleResults = (portal.results || []).filter(mod => {
                    const releaseLine = mod.latest_release?.info_json?.factorio_version;
                    return releaseLine === expectedLine;
                });
                if (active) {
                    setPortalFactorioLine(expectedLine);
                    setFuse(new Fuse(compatibleResults, {
                    keys: [{name: "name", weight: 2}, {name: "title", weight: 1}],
                    minMatchCharLength: 2,
                    threshold: 0.35
                    }));
                }
            } catch (error) {
                if (active) window.flash("The Factorio mod portal index is currently unavailable.", "red");
            }
        })();
        return () => { active = false; };
    }, [fetchInstalledMods, fetchModPacks]);

    const deleteAllMods = async () => {
        setIsDeletingAllMods(true);
        try {
            await modsResource.deleteAll();
            await fetchInstalledMods();
            window.flash("All installed mods were removed.", "green");
        } finally {
            setIsDeletingAllMods(false);
        }
    };

    const updateAllMods = async () => {
        setIsUpdatingAllMods(true);
        try {
            await Promise.all(updatableMods.map(modsResource.update));
            await fetchInstalledMods();
            window.flash("Compatible mod updates installed.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Not all mods could be updated.", "red");
        } finally {
            setIsUpdatingAllMods(false);
        }
    };

    const toggleMod = modName => modsResource.toggle(modName).then(fetchInstalledMods);
    const deleteMod = modName => modsResource.delete(modName).then(fetchInstalledMods);
    const updateMod = version => modsResource.update(version).then(fetchInstalledMods);

    return <>
        <PageHeader
            title="Mods & mod packs"
            help={portalFactorioLine ? `Portal results and dependency resolution are filtered to Factorio ${portalFactorioLine}. Mod packs can be reused across profiles.` : "Portal results are filtered to the installed Factorio version. Mod packs can be reused across profiles."}
            actions={<div className="ui-status-badge">{installedMods.length} installed</div>}
        />

        {disabled && <Alert type="warning" className="mb-5">
            Mod changes are locked while Factorio is running or stopping. Downloads and inspection remain available.
        </Alert>}

        {!disabled && <TabControl>
            <Tab title="Mod portal"><AddMod refetchInstalledMods={fetchInstalledMods} fuse={fuse} factorioVersion={portalFactorioLine}/></Tab>
            <Tab title="Upload archive"><UploadMod refetchInstalledMods={fetchInstalledMods}/></Tab>
            <Tab title="Import from save"><LoadMods refreshMods={fetchInstalledMods}/></Tab>
        </TabControl>}

        <Panel
            title="Installed mods"
            description={factorioVersion ? `Compatibility is evaluated against Factorio ${factorioVersion}.` : "Loading Factorio compatibility information…"}
            className="mb-5"
            content={isLoading
                ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faPuzzlePiece} spin/><p className="mt-3">Reading installed mods…</p></div></div>
                : <ModList
                    addUpdatableMod={addUpdatableMod}
                    toggleMod={toggleMod}
                    updateMod={updateMod}
                    deleteMod={deleteMod}
                    mods={installedMods}
                    factorioVersion={factorioVersion}
                    disabled={disabled}
                />}
            actions={<>
                {!disabled && <>
                    <Button size="sm" type="danger" isLoading={isDeletingAllMods} isDisabled={installedMods.length === 0} onClick={() => setIsDeleteAllDialogOpen(true)}>
                        <FontAwesomeIcon icon={faTrashAlt}/> Delete all
                    </Button>
                    <Button size="sm" type="secondary" isLoading={isUpdatingAllMods} isDisabled={updatableMods.length === 0} onClick={updateAllMods}>
                        <FontAwesomeIcon icon={faRotate}/> Update {updatableMods.length || "all"}
                    </Button>
                </>}
                <ButtonLink size="sm" type="secondary" href={modsResource.downloadAllURL}>
                    <FontAwesomeIcon icon={faDownload}/> Download all
                </ButtonLink>
            </>}
        />

        <Panel
            title="Mod packs"
            description="Manager-wide snapshots of mod combinations. Loading one replaces the installed mod set of the active profile only."
            content={modPacks.length === 0
                ? <EmptyState icon={faPuzzlePiece} title="No mod packs"/>
                : <div className="space-y-4">{modPacks.map(pack => <ModPack
                    factorioVersion={factorioVersion}
                    key={pack.name}
                    modPack={pack}
                    reloadMods={fetchInstalledMods}
                    reloadModPacks={fetchModPacks}
                    disabled={disabled}
                />)}</div>}
            actions={!disabled && <CreateModPack onSuccess={fetchModPacks}/>}
        />

        <ConfirmDialog
            title="Delete all installed mods?"
            content={`Every installed mod archive and the active mod list will be removed from ${activeProfile?.name || "the active profile"}. Manager-wide mod packs and other profiles are kept.`}
            isOpen={isDeleteAllDialogOpen}
            close={() => setIsDeleteAllDialogOpen(false)}
            onSuccess={deleteAllMods}
        />
    </>;
};

export default Mods;
