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
import MapImageViewer, {entityDetailZoom, MapImageLightbox} from "../components/MapImageViewer";
import PlayerOverviewPanel from "../components/PlayerOverviewPanel";
import {useProfiles} from "../context/ProfileContext";
import mapSurfaceHelpers from "./mapSurfaces.cjs";

const {groupMapSurfaces, mapSurfaceKind, mapSurfaceLabel} = mapSurfaceHelpers;

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

const Controls = ({serverStatus, canManage = false}) => {
    const {activeProfile} = useProfiles();
    const [saves, setSaves] = useState([]);
    const [checkpointState, setCheckpointState] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [saveLoadError, setSaveLoadError] = useState("");
    const [checkpointLoadError, setCheckpointLoadError] = useState("");
    const [overviewReloadToken, setOverviewReloadToken] = useState(0);
    const [mapState, setMapState] = useState(null);
    const [mapLoadError, setMapLoadError] = useState("");
    const [selectedSurface, setSelectedSurface] = useState("");
    const [isLoadingMap, setIsLoadingMap] = useState(true);
    const [isMapLightboxOpen, setIsMapLightboxOpen] = useState(false);
    const [mapView, setMapView] = useState({zoom: 1, x: 0, y: 0});
    const [mapEntities, setMapEntities] = useState(null);
    const [isLoadingMapEntities, setIsLoadingMapEntities] = useState(false);
    const [mapEntityError, setMapEntityError] = useState("");
    const [mapEntityReloadToken, setMapEntityReloadToken] = useState(0);

    const loadMapSnapshot = useCallback(async (showError = false) => {
        try {
            const state = await serverResource.mapSnapshot();
            setMapState(state);
            setMapLoadError("");
            return state;
        } catch (error) {
            setMapLoadError("Map snapshot status could not be loaded.");
            if (showError) window.flash(error?.response?.data || "Map snapshot could not be loaded.", "red");
            return null;
        } finally {
            setIsLoadingMap(false);
        }
    }, []);

    useEffect(() => {
        let active = true;
        setIsLoading(true);
        setSaveLoadError("");
        setCheckpointLoadError("");
        Promise.allSettled([
            savesResource.list(false),
            savesResource.checkpoints.list()
        ]).then(([saveResult, checkpointResult]) => {
            if (!active) return;
            if (saveResult.status === "fulfilled") setSaves(saveResult.value || []);
            else {
                setSaves([]);
                setSaveLoadError("Save data could not be loaded.");
            }
            if (checkpointResult.status === "fulfilled") setCheckpointState(checkpointResult.value);
            else {
                setCheckpointState(null);
                setCheckpointLoadError("Checkpoint data could not be loaded.");
            }
        }).finally(() => active && setIsLoading(false));
        return () => { active = false; };
    }, [activeProfile?.id, activeProfile?.selected_save, serverStatus?.savefile, overviewReloadToken]);

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

    const mapSnapshot = mapState?.snapshot || null;
    const rawMapSurfaces = mapSnapshot?.surfaces || [];
    const mapSnapshotsEnabled = mapState?.settings?.enabled !== false;
    const includeSpacePlatforms = mapState?.settings?.include_space_platforms !== false;
    const mapSurfaces = useMemo(() => rawMapSurfaces.filter(surface => includeSpacePlatforms || mapSurfaceKind(surface) !== "platform"), [includeSpacePlatforms, rawMapSurfaces]);
    const mapSurfaceGroups = useMemo(() => groupMapSurfaces(mapSurfaces), [mapSurfaces]);

    useEffect(() => {
        if (!mapSurfaces.some(surface => surface.id === selectedSurface)) {
            setSelectedSurface(mapSurfaces[0]?.id || "");
        }
    }, [mapSurfaces, selectedSurface]);

    useEffect(() => {
        setMapView({zoom: 1, x: 0, y: 0});
        setIsMapLightboxOpen(false);
    }, [mapState?.snapshot?.generated_at, selectedSurface]);

    const activeSurface = mapSurfaces.find(surface => surface.id === selectedSurface) || mapSurfaces[0] || null;
    const activeSurfaceKind = activeSurface ? mapSurfaceKind(activeSurface) : "surface";
    const activeSurfaceDetailZoom = activeSurfaceKind === "platform" ? 1 : entityDetailZoom;
    const shouldLoadMapEntities = mapView.zoom >= activeSurfaceDetailZoom;

    useEffect(() => {
        setMapEntities(null);
        setMapEntityError("");
        setIsLoadingMapEntities(false);
    }, [activeSurface?.entities_available, activeSurface?.id, activeSurface?.view_bounds_available, mapEntityReloadToken, mapSnapshot?.generated_at]);

    useEffect(() => {
        if (!shouldLoadMapEntities || mapEntities !== null || mapEntityError) return undefined;
        if (!activeSurface?.entities_available || activeSurface?.view_bounds_available !== true || !mapSnapshot?.generated_at) {
            setIsLoadingMapEntities(false);
            return undefined;
        }
        const controller = new AbortController();
        let active = true;
        setIsLoadingMapEntities(true);
        serverResource.mapSnapshotEntities(activeSurface.id, mapSnapshot.generated_at, controller.signal)
            .then(entities => {
                if (!active) return;
                if (entities.length !== Number(activeSurface.entity_count)) {
                    throw new Error("The map entity endpoint returned an incomplete dataset.");
                }
                setMapEntities(entities);
            })
            .catch(error => {
                if (active && error?.name !== "AbortError") setMapEntityError("Building detail could not be loaded. The map image remains available.");
            })
            .finally(() => {
                if (active) setIsLoadingMapEntities(false);
            });
        return () => {
            active = false;
            controller.abort();
        };
    }, [activeSurface?.entities_available, activeSurface?.entity_count, activeSurface?.id, activeSurface?.view_bounds_available, mapEntities, mapEntityError, mapEntityReloadToken, mapSnapshot?.generated_at, shouldLoadMapEntities]);

    const sortedSaves = useMemo(() => [...saves].sort((left, right) => new Date(right.last_mod) - new Date(left.last_mod)), [saves]);
    const selectedName = serverStatus?.running ? serverStatus?.savefile : activeProfile?.selected_save;
    const activeSave = sortedSaves.find(save => save.name === selectedName)
        || (selectedName?.startsWith("Load Latest") ? sortedSaves[0] : null)
        || sortedSaves.find(save => save.name === activeProfile?.selected_save)
        || sortedSaves[0]
        || null;
    const checkpoints = checkpointState?.checkpoints || [];
    const latestCheckpoint = [...checkpoints].sort((left, right) => new Date(right.created_at) - new Date(left.created_at))[0] || null;
    const mapImageURL = activeSurface && mapSnapshot
        ? `/api/map-snapshot/surfaces/${encodeURIComponent(activeSurface.id)}?v=${encodeURIComponent(mapSnapshot.generated_at)}`
        : "";
    const activeSurfaceLabel = activeSurface ? mapSurfaceLabel(activeSurface) : "";
    const mapImageAlt = activeSurface && mapSnapshot ? `${activeSurfaceLabel} map from ${mapSnapshot.save_name}` : "Factory map";
    const mapEntityOverlay = activeSurface ? {
        entities: mapEntities,
        error: mapEntityError,
        isLoading: isLoadingMapEntities,
        surface: activeSurface
    } : null;

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
                    : saveLoadError
                        ? <Alert type="danger"><div className="flex flex-wrap items-center gap-3"><span>{saveLoadError}</span><Button type="secondary" size="sm" onClick={() => setOverviewReloadToken(token => token + 1)}>Retry</Button></div></Alert>
                    : activeSave
                        ? <div className="ui-world-snapshot__content">
                            {checkpointLoadError && <Alert type="warning" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{checkpointLoadError}</span><Button type="secondary" size="sm" onClick={() => setOverviewReloadToken(token => token + 1)}>Retry</Button></div></Alert>}
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
                                <div className="ui-fact"><span>Fixed checkpoints</span><strong>{checkpointLoadError ? "Unavailable" : checkpoints.length}</strong></div>
                            </div>
                        </div>
                        : <>
                            {checkpointLoadError && <Alert type="warning" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{checkpointLoadError}</span><Button type="secondary" size="sm" onClick={() => setOverviewReloadToken(token => token + 1)}>Retry</Button></div></Alert>}
                            <EmptyState icon={faHardDrive} title="No world in this profile"/>
                        </>}
                actions={<>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/saves"><FontAwesomeIcon icon={faFloppyDisk}/> Saves & checkpoints</Link>
                    {activeSave && <a className="ui-button ui-button--secondary ui-button--sm" href={`/api/saves/dl/${encodeURIComponent(activeSave.name)}`}><FontAwesomeIcon icon={faHardDrive}/> Download save</a>}
                </>}
            />

            <Panel
                title="Profile runtime"
                headerAction={<ScopeBadge/>}
                content={<div className="ui-kv-list">
                    <div><span>Release target</span><strong>{targetLabel(activeProfile?.release_target)}</strong></div>
                    <div><span>Game endpoint</span><strong>{activeProfile?.bind_ip || "0.0.0.0"}:{activeProfile?.port || 34197} / UDP</strong></div>
                    <div><span>Latest checkpoint</span><strong>{checkpointLoadError ? "Unavailable" : latestCheckpoint ? formatDate(latestCheckpoint.created_at) : "None"}</strong></div>
                </div>}
                actions={<>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/server-settings"><FontAwesomeIcon icon={faServer}/> Server configuration</Link>
                    <Link className="ui-button ui-button--secondary ui-button--sm" to="/releases"><FontAwesomeIcon icon={faCloudArrowDown}/> Version & mode</Link>
                </>}
            />
        </div>

        {activeProfile?.id && <PlayerOverviewPanel profileID={activeProfile.id} serverStatus={serverStatus}/>}

        <Panel
            className="ui-map-snapshot mt-5"
            title="Factory map"
            help="Generated from a temporary copy of the latest completed save. The exporter is never added to the active profile or written back to the save."
            headerAction={<div className="flex flex-wrap items-center gap-2">
                <ScopeBadge/>
                <span className={`ui-status-badge ${!mapSnapshotsEnabled ? "ui-status-badge--stopped" : mapState?.running ? "ui-status-badge--warning" : mapSnapshot ? "ui-status-badge--running" : ""}`}>
                    {!mapSnapshotsEnabled ? "Disabled" : mapState?.running ? "Generating" : mapSnapshot ? "Ready" : "No snapshot"}
                </span>
            </div>}
            content={isLoadingMap
                ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faMap} spin/><p className="mt-3">Loading map…</p></div></div>
                : <>
                    {mapLoadError && <Alert type="danger" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{mapLoadError}</span><Button type="secondary" size="sm" onClick={() => loadMapSnapshot(true)}>Retry</Button></div></Alert>}
                    {mapState?.last_error && <Alert type="warning" className="mb-4">{mapState.last_error}</Alert>}
                    {mapSnapshot && activeSurface
                        ? <div className="ui-map-snapshot__content">
                            <div className="ui-map-snapshot__toolbar">
                                <div>
                                    <span>Surface</span>
                                    <select className="ui-select" aria-label="Map surface" value={activeSurface.id} onChange={event => setSelectedSurface(event.target.value)}>
                                        {mapSurfaceGroups.map(group => <optgroup label={group.label} key={group.kind}>
                                            {group.surfaces.map(surface => <option value={surface.id} key={surface.id}>{mapSurfaceLabel(surface)}</option>)}
                                        </optgroup>)}
                                    </select>
                                </div>
                                <div className="ui-map-snapshot__facts">
                                    <span><small>Snapshot</small><strong>{formatDate(mapSnapshot.generated_at)}</strong></span>
                                    <span><small>Source save</small><strong title={mapSnapshot.save_name}>{mapSnapshot.save_name}</strong></span>
                                    <span><small>Charted chunks</small><strong>{activeSurface.chunk_count.toLocaleString()}</strong></span>
                                    <span><small>Building detail</small><strong>{activeSurface.view_bounds_available !== true || !activeSurface.entities_available
                                        ? "Not available"
                                        : isLoadingMapEntities ? "Loading…" : mapEntityError ? "Unavailable" : mapEntities === null ? "Zoom in to load" : `${mapEntities.length.toLocaleString()} footprints`}</strong></span>
                                </div>
                            </div>
                            {mapEntityError && <Alert type="warning" className="mb-4"><div className="flex flex-wrap items-center gap-3"><span>{mapEntityError}</span><Button type="secondary" size="sm" onClick={() => setMapEntityReloadToken(token => token + 1)}>Retry detail</Button></div></Alert>}
                            <MapImageViewer
                                src={mapImageURL}
                                alt={mapImageAlt}
                                view={mapView}
                                setView={setMapView}
                                isPixelated={activeSurfaceKind === "platform"}
                                entityOverlay={mapEntityOverlay}
                                detailZoom={activeSurfaceDetailZoom}
                                onFullscreen={() => setIsMapLightboxOpen(true)}
                            />
                            <MapImageLightbox
                                src={mapImageURL}
                                alt={mapImageAlt}
                                title={activeSurfaceLabel}
                                isOpen={isMapLightboxOpen}
                                close={() => setIsMapLightboxOpen(false)}
                                view={mapView}
                                setView={setMapView}
                                isPixelated={activeSurfaceKind === "platform"}
                                entityOverlay={mapEntityOverlay}
                                detailZoom={activeSurfaceDetailZoom}
                            />
                        </div>
                        : <EmptyState
                            icon={faMap}
                            title={mapState?.running ? "Generating first map snapshot" : activeSave ? "No map snapshot yet" : "No world to map"}
                        />}
                </>}
            actions={canManage ? <Button
                type="secondary"
                size="sm"
                isLoading={Boolean(mapState?.running)}
                isDisabled={!mapSnapshotsEnabled || !activeSave || Boolean(mapLoadError) || Boolean(mapState?.running)}
                title={!mapSnapshotsEnabled ? "Enable factory map snapshots under Server settings first." : undefined}
                onClick={refreshMapSnapshot}
            ><FontAwesomeIcon icon={faArrowsRotate}/> Generate now</Button> : null}
        />

        <Panel
            className="mt-5"
            title="Operations"
            content={<div className="ui-operation-links">
                <Link to="/mods"><FontAwesomeIcon icon={faPuzzlePiece}/><span><strong>Mods</strong><small>{activeProfile?.mod_count ?? "—"} installed</small></span></Link>
                <Link to="/game-settings"><FontAwesomeIcon icon={faGamepad}/><span><strong>Game settings</strong><small>Runtime configuration</small></span></Link>
                {canManage && <Link to="/console"><FontAwesomeIcon icon={faTerminal}/><span><strong>Console</strong><small>Commands and live output</small></span></Link>}
                <Link to="/logs"><FontAwesomeIcon icon={faFileLines}/><span><strong>Logs</strong><small>Recent Factorio output</small></span></Link>
                <Link to="/profiles"><FontAwesomeIcon icon={faLayerGroup}/><span><strong>Profiles</strong><small>Switch saved setups</small></span></Link>
            </div>}
        />
    </>;
};

export default Controls;
