import React, {useCallback, useEffect, useMemo, useState} from "react";
import {Link} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faArrowsRotate, faCloudArrowDown, faFileLines, faFloppyDisk, faGamepad,
    faHardDrive, faLayerGroup, faMap, faPuzzlePiece, faServer, faTerminal
} from "@fortawesome/free-solid-svg-icons";
import savesResource from "../../api/resources/saves";
import serverResource from "../../api/resources/server";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Alert from "../components/Alert";
import EmptyState from "../components/EmptyState";
import ScopeBadge from "../components/ScopeBadge";
import Button from "../components/Button";
import MapImageViewer, {MapImageLightbox} from "../components/MapImageViewer";
import {useProfiles} from "../context/ProfileContext";

const modeLabel = mode => mode === "space-age" ? "Space Age" : mode === "factorio" ? "Factorio" : "Custom";
const targetLabel = target => target === "latest" ? "Experimental / latest" : target === "stable" ? "Stable" : target || "Pinned";

const formatSize = bytes => {
    if (!Number.isFinite(bytes)) return "—";
    const megabytes = bytes / 1024 / 1024;
    return megabytes >= 100 ? `${megabytes.toFixed(0)} MB` : `${megabytes.toFixed(2)} MB`;
};

const formatDate = value => {
    if (!value) return "—";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString();
};

const Controls = ({serverStatus}) => {
    const {activeProfile} = useProfiles();
    const [saves, setSaves] = useState([]);
    const [checkpointState, setCheckpointState] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [mapState, setMapState] = useState(null);
    const [selectedSurface, setSelectedSurface] = useState("");
    const [isLoadingMap, setIsLoadingMap] = useState(true);
    const [isMapLightboxOpen, setIsMapLightboxOpen] = useState(false);
    const [mapView, setMapView] = useState({zoom: 1, x: 0, y: 0});

    const loadMapSnapshot = useCallback(async (showError = false) => {
        try {
            const state = await serverResource.mapSnapshot();
            setMapState(state);
            return state;
        } catch (error) {
            if (showError) window.flash(error?.response?.data || "Map snapshot could not be loaded.", "red");
            return null;
        } finally {
            setIsLoadingMap(false);
        }
    }, []);

    useEffect(() => {
        let active = true;
        setIsLoading(true);
        Promise.allSettled([
            savesResource.list(false),
            savesResource.checkpoints.list()
        ]).then(([saveResult, checkpointResult]) => {
            if (!active) return;
            setSaves(saveResult.status === "fulfilled" ? saveResult.value || [] : []);
            setCheckpointState(checkpointResult.status === "fulfilled" ? checkpointResult.value : null);
        }).finally(() => active && setIsLoading(false));
        return () => { active = false; };
    }, [activeProfile?.id, activeProfile?.selected_save, serverStatus?.savefile]);

    useEffect(() => {
        setIsLoadingMap(true);
        setMapState(null);
        setSelectedSurface("");
        loadMapSnapshot();
    }, [activeProfile?.id, loadMapSnapshot]);

    useEffect(() => {
        if (!mapState?.running) return undefined;
        const poll = window.setInterval(() => loadMapSnapshot(), 3000);
        return () => window.clearInterval(poll);
    }, [loadMapSnapshot, mapState?.running]);

    useEffect(() => {
        const surfaces = mapState?.snapshot?.surfaces || [];
        if (!surfaces.some(surface => surface.id === selectedSurface)) {
            setSelectedSurface(surfaces[0]?.id || "");
        }
    }, [mapState?.snapshot?.generated_at, selectedSurface]);

    useEffect(() => {
        setMapView({zoom: 1, x: 0, y: 0});
        setIsMapLightboxOpen(false);
    }, [mapState?.snapshot?.generated_at, selectedSurface]);

    const sortedSaves = useMemo(() => [...saves].sort((left, right) => new Date(right.last_mod) - new Date(left.last_mod)), [saves]);
    const selectedName = serverStatus?.running ? serverStatus?.savefile : activeProfile?.selected_save;
    const activeSave = sortedSaves.find(save => save.name === selectedName)
        || (selectedName?.startsWith("Load Latest") ? sortedSaves[0] : null)
        || sortedSaves.find(save => save.name === activeProfile?.selected_save)
        || sortedSaves[0]
        || null;
    const checkpoints = checkpointState?.checkpoints || [];
    const latestCheckpoint = [...checkpoints].sort((left, right) => new Date(right.created_at) - new Date(left.created_at))[0] || null;
    const mapSnapshot = mapState?.snapshot || null;
    const mapSurfaces = mapSnapshot?.surfaces || [];
    const activeSurface = mapSurfaces.find(surface => surface.id === selectedSurface) || mapSurfaces[0] || null;
    const mapImageURL = activeSurface && mapSnapshot
        ? `/api/map-snapshot/surfaces/${encodeURIComponent(activeSurface.id)}?v=${encodeURIComponent(mapSnapshot.generated_at)}`
        : "";
    const mapImageAlt = activeSurface && mapSnapshot ? `${activeSurface.name} map from ${mapSnapshot.save_name}` : "Factory map";

    const refreshMapSnapshot = async () => {
        try {
            const state = await serverResource.refreshMapSnapshot();
            setMapState(state);
            window.flash("Map snapshot generation started.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Map snapshot generation could not be started.", "red");
        }
    };

    return <>
        <PageHeader title="Overview"/>

        {serverStatus?.stopping && <Alert type="warning" className="mb-5">
            Factorio is saving the active world and shutting down.
        </Alert>}

        <div className="ui-dashboard-grid">
            <Panel
                className="ui-world-snapshot"
                title="Current world"
                help="This is the save configured for the next start. While Factorio is running, it follows the loaded save."
                headerAction={<ScopeBadge/>}
                content={isLoading
                    ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faHardDrive} spin/><p className="mt-3">Reading save data…</p></div></div>
                    : activeSave
                        ? <div className="ui-world-snapshot__content">
                            <div className="ui-world-snapshot__identity">
                                <div className="ui-world-snapshot__icon"><FontAwesomeIcon icon={faFloppyDisk}/></div>
                                <div>
                                    <span>Selected save</span>
                                    <h2 title={activeSave.name}>{activeSave.name}</h2>
                                    <p>Last saved {formatDate(activeSave.last_mod)}</p>
                                </div>
                            </div>
                            <div className="ui-fact-grid">
                                <div className="ui-fact"><span>File size</span><strong>{formatSize(activeSave.size)}</strong></div>
                                <div className="ui-fact"><span>Stored saves</span><strong>{activeProfile?.save_count ?? sortedSaves.length}</strong></div>
                                <div className="ui-fact"><span>Installed mods</span><strong>{activeProfile?.mod_count ?? "—"}</strong></div>
                                <div className="ui-fact"><span>Fixed checkpoints</span><strong>{checkpoints.length}</strong></div>
                            </div>
                        </div>
                        : <EmptyState icon={faHardDrive} title="No world in this profile"/>}
                actions={<>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/saves"><FontAwesomeIcon icon={faFloppyDisk}/> Saves & checkpoints</Link>
                    {activeSave && <a className="ui-button ui-button--secondary ui-button--sm" href={`/api/saves/dl/${encodeURIComponent(activeSave.name)}`}><FontAwesomeIcon icon={faHardDrive}/> Download save</a>}
                </>}
            />

            <Panel
                title="Profile runtime"
                headerAction={<ScopeBadge/>}
                content={<div className="ui-kv-list">
                    <div><span>Profile</span><strong>{activeProfile?.name || "Unavailable"}</strong></div>
                    <div><span>Factorio</span><strong>{activeProfile?.installed_version || serverStatus?.fac_version || "Unknown"}</strong></div>
                    <div><span>Release target</span><strong>{targetLabel(activeProfile?.release_target)}</strong></div>
                    <div><span>Game mode</span><strong>{modeLabel(activeProfile?.game_mode)}</strong></div>
                    <div><span>Game endpoint</span><strong>{activeProfile?.bind_ip || "0.0.0.0"}:{activeProfile?.port || 34197} / UDP</strong></div>
                    <div><span>Latest checkpoint</span><strong>{latestCheckpoint ? formatDate(latestCheckpoint.created_at) : "None"}</strong></div>
                </div>}
                actions={<>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/server-settings"><FontAwesomeIcon icon={faServer}/> Server configuration</Link>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/releases"><FontAwesomeIcon icon={faCloudArrowDown}/> Version & mode</Link>
                </>}
            />
        </div>

        <Panel
            className="ui-map-snapshot mt-5"
            title="Factory map"
            help="Generated from a temporary copy of the latest completed save. The exporter is never added to the active profile or written back to the save."
            headerAction={<div className="flex flex-wrap items-center gap-2">
                <ScopeBadge/>
                <span className={`ui-status-badge ${mapState?.running ? "ui-status-badge--warning" : mapSnapshot ? "ui-status-badge--running" : ""}`}>
                    {mapState?.running ? "Generating" : mapSnapshot ? "Ready" : "No snapshot"}
                </span>
            </div>}
            content={isLoadingMap
                ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faMap} spin/><p className="mt-3">Loading map…</p></div></div>
                : <>
                    {mapState?.last_error && <Alert type="warning" className="mb-4">{mapState.last_error}</Alert>}
                    {mapSnapshot && activeSurface
                        ? <div className="ui-map-snapshot__content">
                            <div className="ui-map-snapshot__toolbar">
                                <div>
                                    <span>Surface</span>
                                    <select className="ui-select" value={activeSurface.id} onChange={event => setSelectedSurface(event.target.value)}>
                                        {mapSurfaces.map(surface => <option value={surface.id} key={surface.id}>{surface.name}</option>)}
                                    </select>
                                </div>
                                <div className="ui-map-snapshot__facts">
                                    <span><small>Snapshot</small><strong>{formatDate(mapSnapshot.generated_at)}</strong></span>
                                    <span><small>Source save</small><strong title={mapSnapshot.save_name}>{mapSnapshot.save_name}</strong></span>
                                    <span><small>Charted chunks</small><strong>{activeSurface.chunk_count.toLocaleString()}</strong></span>
                                </div>
                            </div>
                            <MapImageViewer
                                src={mapImageURL}
                                alt={mapImageAlt}
                                view={mapView}
                                setView={setMapView}
                                isPixelated={activeSurface.kind === "platform"}
                                onFullscreen={() => setIsMapLightboxOpen(true)}
                            />
                            <MapImageLightbox
                                src={mapImageURL}
                                alt={mapImageAlt}
                                title={activeSurface.name}
                                isOpen={isMapLightboxOpen}
                                close={() => setIsMapLightboxOpen(false)}
                                view={mapView}
                                setView={setMapView}
                                isPixelated={activeSurface.kind === "platform"}
                            />
                        </div>
                        : <EmptyState
                            icon={faMap}
                            title={mapState?.running ? "Generating first map snapshot" : activeSave ? "No map snapshot yet" : "No world to map"}
                        />}
                </>}
            actions={<Button
                type="secondary"
                size="sm"
                isLoading={Boolean(mapState?.running)}
                isDisabled={!activeSave || Boolean(mapState?.running)}
                onClick={refreshMapSnapshot}
            ><FontAwesomeIcon icon={faArrowsRotate}/> Generate now</Button>}
        />

        <Panel
            className="mt-5"
            title="Operations"
            content={<div className="ui-operation-links">
                <Link to="/mods"><FontAwesomeIcon icon={faPuzzlePiece}/><span><strong>Mods</strong><small>{activeProfile?.mod_count ?? "—"} installed</small></span></Link>
                <Link to="/game-settings"><FontAwesomeIcon icon={faGamepad}/><span><strong>Game settings</strong><small>Runtime configuration</small></span></Link>
                <Link to="/console"><FontAwesomeIcon icon={faTerminal}/><span><strong>Console</strong><small>Commands and live output</small></span></Link>
                <Link to="/logs"><FontAwesomeIcon icon={faFileLines}/><span><strong>Logs</strong><small>Recent Factorio output</small></span></Link>
                <Link to="/profiles"><FontAwesomeIcon icon={faLayerGroup}/><span><strong>Profiles</strong><small>Switch saved setups</small></span></Link>
            </div>}
        />
    </>;
};

export default Controls;
