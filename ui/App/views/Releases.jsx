import React, {useCallback, useEffect, useId, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCloudArrowDown, faFlask, faIndustry, faRocket, faShieldHalved, faTag} from "@fortawesome/free-solid-svg-icons";
import server from "../../api/resources/server";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Button from "../components/Button";
import Alert from "../components/Alert";
import HelpTip from "../components/HelpTip";
import Label from "../components/Label";
import Error from "../components/Error";
import ScopeBadge from "../components/ScopeBadge";
import {useProfiles} from "../context/ProfileContext";

const channels = [
    {id: "stable", title: "Stable", description: "Recommended for long-running worlds and modded servers.", icon: faShieldHalved, tone: "green"},
    {id: "latest", title: "Experimental / latest", description: "Newest headless release from Factorio's experimental branch.", icon: faFlask, tone: "orange"},
    {id: "exact", title: "Exact version", description: "Install one specific archived headless release and keep it pinned.", icon: faTag, tone: "blue-light"}
];

const modes = [
    {id: "factorio", title: "Factorio", description: "Base game without Space Age, Quality or Elevated Rails.", icon: faIndustry, tone: "green"},
    {id: "space-age", title: "Space Age", description: "Enables Space Age together with Quality and Elevated Rails. Joining players also need access to the expansion.", icon: faRocket, tone: "orange"}
];

const ChoiceCard = ({option, selected, installed, disabled, onSelect, detail}) => {
    const id = useId();
    return <div className={"ui-choice-card" + (selected ? " is-selected" : "") + (disabled ? " is-disabled" : "")}>
        <input id={id} className="sr-only" type="radio" checked={selected} onChange={onSelect} disabled={disabled}/>
        <label className="ui-choice-card__select" htmlFor={id}>
            <span className="ui-choice-card__radio" aria-hidden="true"/>
            <div className="ui-choice-card__body">
                <div className={"ui-choice-card__icon ui-choice-card__icon--" + option.tone}>
                    <FontAwesomeIcon icon={option.icon}/>
                </div>
                <div className="ui-choice-card__copy">
                    <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-bold text-white">{option.title}</h3>
                        {installed && <span className="ui-status-badge ui-status-badge--running">Installed</span>}
                    </div>
                    {detail && <strong>{detail}</strong>}
                </div>
            </div>
        </label>
        <span className="ui-choice-card__help"><HelpTip content={option.description} label={`${option.title} help`}/></span>
    </div>;
};

const Releases = ({serverStatus, refreshServerStatus, canManage = false}) => {
    const {activeProfile, refreshProfiles} = useProfiles();
    const [targetType, setTargetType] = useState(null);
    const [exactVersion, setExactVersion] = useState("");
    const [releaseStatus, setReleaseStatus] = useState(null);
    const [mode, setMode] = useState(null);
    const [modeStatus, setModeStatus] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [releaseLoadError, setReleaseLoadError] = useState("");
    const [modeLoadError, setModeLoadError] = useState("");
    const [installing, setInstalling] = useState(false);
    const [savingMode, setSavingMode] = useState(false);
    const locked = Boolean(!canManage || serverStatus?.known === false || serverStatus?.running || serverStatus?.stopping);

    const loadRuntime = useCallback(async () => {
        setIsLoading(true);
        setReleaseLoadError("");
        setModeLoadError("");
        const [releaseResult, modeResult] = await Promise.allSettled([server.releaseStatus(), server.gameMode()]);
        if (releaseResult.status === "fulfilled") {
            const status = releaseResult.value;
            setReleaseStatus(status);
            const persistedTarget = status.installed_target;
            let installedType = "exact";
            if (["stable", "latest"].includes(persistedTarget)) {
                installedType = persistedTarget;
            } else if (!persistedTarget && ["stable", "latest"].includes(status.installed_channel)) {
                installedType = status.installed_channel;
            }
            setTargetType(installedType);
            setExactVersion(installedType === "exact" && persistedTarget ? persistedTarget : status.installed_version || "");
        } else {
            setReleaseStatus(null);
            setTargetType(null);
            setExactVersion("");
            setReleaseLoadError("Factorio release status could not be loaded.");
        }
        if (modeResult.status === "fulfilled") {
            setModeStatus(modeResult.value);
            setMode(["factorio", "space-age"].includes(modeResult.value.mode) ? modeResult.value.mode : null);
        } else {
            setModeStatus(null);
            setMode(null);
            setModeLoadError("Game-mode status could not be loaded.");
        }
        setIsLoading(false);
    }, []);

    useEffect(() => { loadRuntime(); }, [loadRuntime]);

    const install = async () => {
        const target = targetType === "exact" ? exactVersion.trim() : targetType;
        if (!target || (targetType === "exact" && !/^\d+\.\d+\.\d+$/.test(target))) return;
        setInstalling(true);
        try {
            const result = await server.installRelease(target);
            await Promise.all([refreshServerStatus(), refreshProfiles().catch(() => undefined)]);
            await loadRuntime();
            window.flash(`${result.message}: ${result.installed_version || target}`, "green");
        } catch (error) {
            window.flash(error?.response?.data || `Factorio ${target} could not be installed.`, "red");
        } finally {
            setInstalling(false);
        }
    };

    const saveGameMode = async () => {
        if (!mode) return;
        setSavingMode(true);
        try {
            const result = await server.setGameMode(mode);
            setModeStatus(result);
            setMode(result.mode);
            await refreshProfiles().catch(() => undefined);
            window.flash(result.mode === "space-age" ? "Space Age mode enabled." : "Base Factorio mode enabled.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Game mode could not be changed.", "red");
        } finally {
            setSavingMode(false);
        }
    };

    const persistedTarget = releaseStatus?.installed_target;
    const hasPinnedTarget = Boolean(persistedTarget && !["stable", "latest"].includes(persistedTarget));
    const selectedTarget = targetType === "exact" ? exactVersion.trim() : targetType;
    const exactVersionIsValid = /^\d+\.\d+\.\d+$/.test(exactVersion.trim());
    const selectionIsValid = targetType === "exact" ? exactVersionIsValid : Boolean(selectedTarget);
    const selectionIsInstalled = targetType === "exact"
        ? releaseStatus?.installed_version === exactVersion.trim() && (hasPinnedTarget || releaseStatus?.installed_channel === "custom")
        : releaseStatus?.installed_channel === targetType;

    return <>
        <PageHeader title="Version & mode"/>

        {locked && <Alert type={canManage ? "warning" : "info"} className="mb-5">
            {!canManage
                ? "Viewer access is read-only. Installed version, update target, and game mode remain visible."
                : serverStatus?.known === false
                ? "Version and mode changes are locked until the Factorio process status is confirmed."
                : serverStatus?.stopping
                    ? "Version and mode changes remain locked while Factorio is shutting down."
                    : "Stop Factorio before replacing program files or changing the game mode."}
        </Alert>}

        <div className="ui-release-layout">
        <Panel
            title="Factorio version"
            help="The selected channel or pinned version is stored with this profile and survives manager updates. Installed marks the executable currently present."
            headerAction={<ScopeBadge/>}
            content={<>
                {releaseLoadError && <Alert type="danger" className="mb-4">
                    <div className="flex flex-wrap items-center gap-3">
                        <span>{releaseLoadError}</span>
                        <Button type="secondary" size="sm" onClick={loadRuntime}>Retry</Button>
                    </div>
                </Alert>}
                {releaseStatus?.metadata_error && <Alert type="warning" className="mb-4">
                    Official release metadata is temporarily unavailable. The installed version remains unchanged and can still be read locally.
                </Alert>}
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                    {channels.map(option => <ChoiceCard
                        key={option.id}
                        option={option}
                        selected={targetType === option.id}
                        installed={option.id === "exact"
                            ? hasPinnedTarget || (!persistedTarget && releaseStatus?.installed_channel === "custom")
                            : !hasPinnedTarget && releaseStatus?.installed_channel === option.id}
                        disabled={locked || installing || isLoading || Boolean(releaseLoadError)}
                        onSelect={() => setTargetType(option.id)}
                        detail={option.id === "stable"
                            ? releaseStatus?.stable_version
                            : option.id === "latest" ? releaseStatus?.latest_version : releaseStatus?.installed_version}
                    />)}
                </div>
                {targetType === "exact" && <div className="ui-subcard mt-4 p-4">
                    <Label text="Exact Factorio version" htmlFor="exact-factorio-version" help="Choose a listed version or enter an official three-part archive version."/>
                    <input
                        id="exact-factorio-version"
                        className="ui-input font-mono"
                        list="factorio-available-versions"
                        value={exactVersion}
                        disabled={locked || installing || isLoading || Boolean(releaseLoadError)}
                        onChange={event => setExactVersion(event.target.value)}
                        placeholder="2.1.14"
                        pattern="[0-9]+\.[0-9]+\.[0-9]+"
                    />
                    <Error
                        error={Boolean(exactVersion.trim()) && !exactVersionIsValid}
                        message="Use a three-part version such as 2.1.14."
                    />
                    <datalist id="factorio-available-versions">
                        {(releaseStatus?.available_versions || []).map(version => <option value={version} key={version}/>) }
                    </datalist>
                </div>}
                <div className="ui-release-facts mt-4">
                    <div><span>Installed executable</span><strong>{releaseStatus?.installed_version || serverStatus?.fac_version || "Unknown"}</strong></div>
                    <div><span>Saved update target</span><strong>{activeProfile?.release_target === "latest" ? "Experimental / latest" : activeProfile?.release_target === "stable" ? "Stable" : activeProfile?.release_target || "Unknown"}</strong></div>
                </div>
                {targetType === "latest" && <Alert type="warning" className="mt-4">
                    Experimental updates can move to a new Factorio compatibility line. Mod search follows the installed version after the switch.
                </Alert>}
            </>}
            actions={canManage ? <Button type="success" onClick={install} isLoading={installing} isDisabled={locked || isLoading || Boolean(releaseLoadError) || !selectionIsValid}>
                <FontAwesomeIcon icon={faCloudArrowDown}/> {selectionIsInstalled ? "Reinstall" : "Install"} {selectedTarget || "version"}
            </Button> : null}
        />

        <Panel
            title="Factorio or Space Age"
            help="Space Age also controls the built-in Quality and Elevated Rails mods. This selection survives manager and game updates."
            headerAction={<ScopeBadge/>}
            content={<>
                {modeLoadError && <Alert type="danger" className="mb-4">
                    <div className="flex flex-wrap items-center gap-3">
                        <span>{modeLoadError}</span>
                        <Button type="secondary" size="sm" onClick={loadRuntime}>Retry</Button>
                    </div>
                </Alert>}
                {modeStatus?.mode === "custom" && <Alert type="warning" className="mb-4">
                    The built-in expansion mods currently form a custom mix. Choose a mode below to make the set consistent.
                </Alert>}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {modes.map(option => <ChoiceCard
                        key={option.id}
                        option={option}
                        selected={mode === option.id}
                        installed={modeStatus?.mode === option.id}
                        disabled={locked || savingMode || isLoading || Boolean(modeLoadError) || (option.id === "space-age" && !modeStatus?.space_age_available)}
                        onSelect={() => setMode(option.id)}
                        detail={option.id === "space-age" && !modeStatus?.space_age_available ? "Not included in this Factorio installation" : null}
                    />)}
                </div>
                {modeStatus?.features?.length > 0 && <div className="ui-feature-state mt-4">
                    {modeStatus.features.map(feature => <span className={`ui-status-badge ${feature.enabled ? "ui-status-badge--running" : "ui-status-badge--stopped"}`} key={feature.name}>
                        {feature.name}: {feature.enabled ? "on" : "off"}
                    </span>)}
                </div>}
            </>}
            actions={canManage ? <Button type="success" onClick={saveGameMode} isLoading={savingMode} isDisabled={locked || isLoading || Boolean(modeLoadError) || !mode || modeStatus?.mode === mode}>
                <FontAwesomeIcon icon={mode === "space-age" ? faRocket : faIndustry}/> Apply {mode === "space-age" ? "Space Age" : "Factorio"}
            </Button> : null}
        />
        </div>
    </>;
};

export default Releases;
