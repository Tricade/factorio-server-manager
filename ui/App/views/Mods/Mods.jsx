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
import ModStartupSettings from "./components/ModStartupSettings";
import {useProfiles} from "../../context/ProfileContext";
import ScopeBadge from "../../components/ScopeBadge";

const Mods = ({serverStatus, canManage = false}) => {
    const {activeProfile, refreshProfiles} = useProfiles();
    const [installedMods, setInstalledMods] = useState([]);
    const [modPacks, setModPacks] = useState([]);
    const [factorioVersion, setFactorioVersion] = useState(null);
    const [portalFactorioLine, setPortalFactorioLine] = useState(null);
    const [fuse, setFuse] = useState(undefined);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");
    const [portalLoadError, setPortalLoadError] = useState("");
    const [reloadToken, setReloadToken] = useState(0);
    const [startupSettingsRefreshKey, setStartupSettingsRefreshKey] = useState(0);
    const [isDeletingAllMods, setIsDeletingAllMods] = useState(false);
    const [isUpdatingAllMods, setIsUpdatingAllMods] = useState(false);
    const [isDeleteAllDialogOpen, setIsDeleteAllDialogOpen] = useState(false);
    const [updatableMods, setUpdatableMods] = useState([]);
    const statusLocked = Boolean(serverStatus?.known === false || serverStatus?.running || serverStatus?.stopping);
    const disabled = Boolean(!canManage || statusLocked || loadError);

    const addUpdatableMod = useCallback(mod => {
        setUpdatableMods(current => [...current.filter(existing => existing.modName !== mod.modName), mod]);
    }, []);

    const fetchInstalledMods = useCallback(async () => {
        const result = await modsResource.installed();
        setUpdatableMods([]);
        setInstalledMods(result || []);
        setStartupSettingsRefreshKey(token => token + 1);
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
        setIsLoading(true);
        setLoadError("");
        setPortalLoadError("");
        (async () => {
            let currentFactorioVersion = null;
            try {
                const version = await server.factorioVersion();
                if (!active) return;
                currentFactorioVersion = version.base_mod_version;
                setFactorioVersion(currentFactorioVersion);
                await Promise.all([fetchInstalledMods(), fetchModPacks()]);
            } catch (error) {
                if (active) setLoadError("Installed mods, mod packs, and compatibility data could not be loaded.");
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
                if (active) {
                    setFuse(undefined);
                    setPortalLoadError("The Factorio mod portal index is currently unavailable.");
                }
            }
        })();
        return () => { active = false; };
    }, [fetchInstalledMods, fetchModPacks, reloadToken]);

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
        />

        {!canManage && <Alert type="info" className="mb-5">Viewer access is read-only. Installed mods and mod packs can be inspected and downloaded.</Alert>}

        {statusLocked && <Alert type="warning" className="mb-5">
            Installed-mod changes are locked until Factorio is confirmed stopped. Manager-wide mod-pack organization and downloads remain available.
        </Alert>}

        {loadError && <Alert type="danger" className="mb-5">
            <div className="flex flex-wrap items-center gap-3">
                <span>{loadError}</span>
                <Button size="sm" type="secondary" onClick={() => setReloadToken(token => token + 1)}>Retry</Button>
            </div>
        </Alert>}

        {canManage && !disabled && <TabControl>
            <Tab title="Mod portal"><AddMod
                refetchInstalledMods={fetchInstalledMods}
                fuse={fuse}
                factorioVersion={portalFactorioLine}
                portalError={portalLoadError}
                retryPortal={() => setReloadToken(token => token + 1)}
            /></Tab>
            <Tab title="Upload archive"><UploadMod refetchInstalledMods={fetchInstalledMods}/></Tab>
            <Tab title="Import from save"><LoadMods refreshMods={fetchInstalledMods}/></Tab>
        </TabControl>}

        <Panel
            title="Installed mods"
            description={factorioVersion ? `Compatibility is evaluated against Factorio ${factorioVersion}.` : "Loading Factorio compatibility information…"}
            className="mb-5"
            headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge/><span className="ui-status-badge">{installedMods.length} installed</span></div>}
            content={isLoading
                ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faPuzzlePiece} spin/><p className="mt-3">Reading installed mods…</p></div></div>
                : loadError
                    ? <Alert type="danger">Installed mod data is unavailable.</Alert>
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

        {canManage && !isLoading && !loadError && <ModStartupSettings
            profileId={activeProfile?.id}
            statusLocked={statusLocked}
            refreshKey={startupSettingsRefreshKey}
        />}

        <Panel
            title="Mod packs"
            description="Manager-wide snapshots of mod combinations and their startup settings. Loading one replaces the installed mod set and stored mod settings of the active profile only."
            headerAction={<ScopeBadge scope="manager"/>}
            content={isLoading
                ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faPuzzlePiece} spin/><p className="mt-3">Reading mod packs…</p></div></div>
                : loadError
                    ? <Alert type="danger">Mod-pack data is unavailable.</Alert>
                : modPacks.length === 0
                ? <EmptyState icon={faPuzzlePiece} title="No mod packs"/>
                : <div className="space-y-4">{modPacks.map(pack => <ModPack
                    factorioVersion={factorioVersion}
                    key={pack.name}
                    modPack={pack}
                    reloadMods={fetchInstalledMods}
                    reloadModPacks={fetchModPacks}
                    profileLocked={disabled}
                    readOnly={!canManage}
                />)}</div>}
            actions={canManage ? <CreateModPack onSuccess={fetchModPacks}/> : null}
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
