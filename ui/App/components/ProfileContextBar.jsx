import React, {useState} from "react";
import {useLocation} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faCloudArrowDown, faFloppyDisk, faGamepad,
    faLayerGroup, faPlay, faRotate, faSkullCrossbones, faStop
} from "@fortawesome/free-solid-svg-icons";
import server from "../../api/resources/server";
import Button from "./Button";
import ConfirmDialog from "./ConfirmDialog";
import {useProfiles} from "../context/ProfileContext";

const isManagerPath = pathname => ["/profiles", "/user-management"].some(path => pathname === path || pathname.startsWith(path + "/"));
const modeLabel = mode => mode === "space-age" ? "Space Age" : mode === "factorio" ? "Factorio" : "Custom";
const channelLabel = target => target === "latest" ? "Experimental" : target === "stable" ? "Stable" : target ? "Pinned" : "";

const ProfileContextBar = ({serverStatus, refreshServerStatus}) => {
    const location = useLocation();
    const {activeProfile, isLoading, error, refreshProfiles} = useProfiles();
    const [isStarting, setIsStarting] = useState(false);
    const [isStopping, setIsStopping] = useState(false);
    const [isKilling, setIsKilling] = useState(false);
    const [isKillDialogOpen, setIsKillDialogOpen] = useState(false);

    if (isManagerPath(location.pathname)) return null;

    if (isLoading && !activeProfile) return <section className="ui-context-strip ui-context-strip--loading" aria-label="Loading active profile">
        <div className="ui-context-loading"><FontAwesomeIcon icon={faLayerGroup} spin/> Loading active profile…</div>
    </section>;

    if (!activeProfile) return <section className="ui-context-strip ui-context-strip--error" aria-label="Active profile unavailable">
        <div className="ui-context-loading">
            <FontAwesomeIcon icon={faLayerGroup}/>
            <span><strong>Active profile unavailable</strong><small>{error || "Profile metadata could not be loaded."}</small></span>
        </div>
        <button className="ui-button ui-button--secondary ui-button--sm" type="button" onClick={() => refreshProfiles().catch(() => undefined)}>
            <FontAwesomeIcon icon={faRotate}/> Retry
        </button>
    </section>;

    const state = serverStatus?.stopping ? "stopping" : serverStatus?.running ? "running" : "stopped";
    const version = activeProfile.installed_version || serverStatus?.fac_version || "Unknown";
    const channel = channelLabel(activeProfile.release_target);
    const selectedSave = activeProfile.save_count === 0
        ? "No save yet"
        : activeProfile.selected_save || serverStatus?.savefile || `${activeProfile.save_count} stored saves`;

    const busy = isStarting || isStopping || isKilling || serverStatus?.stopping;

    const startServer = async () => {
        setIsStarting(true);
        try {
            await server.start(
                activeProfile.bind_ip || "0.0.0.0",
                Number(activeProfile.port || 34197),
                activeProfile.selected_save
            );
            await Promise.all([refreshServerStatus?.(), refreshProfiles().catch(() => undefined)]);
            window.flash("Factorio server started.", "green");
        } catch (startError) {
            window.flash(startError?.response?.data || "Factorio server could not be started.", "red");
        } finally {
            setIsStarting(false);
        }
    };

    const stopServer = async () => {
        setIsStopping(true);
        try {
            await server.stop();
            await refreshServerStatus?.();
            window.flash("Save and stop requested.", "green");
        } catch (stopError) {
            window.flash(stopError?.response?.data || "Factorio server could not be stopped.", "red");
        } finally {
            setIsStopping(false);
        }
    };

    const killServer = async () => {
        setIsKilling(true);
        try {
            await server.kill();
            await refreshServerStatus?.();
            window.flash("Factorio process was force-stopped.", "green");
        } finally {
            setIsKilling(false);
        }
    };

    return <section className="ui-context-strip" aria-label={`Active profile ${activeProfile.name}`}>
        <div className="ui-context-item ui-context-item--server">
            <span className="ui-context-label">Server</span>
            <span className="ui-context-value"><span className={`ui-status-orb ui-status-orb--${state}`}/><strong>{state}</strong></span>
        </div>
        <div className="ui-context-item ui-context-item--save" title={selectedSave}>
            <span className="ui-context-label">Active save</span>
            <span className="ui-context-value"><FontAwesomeIcon icon={faFloppyDisk}/><strong>{selectedSave}</strong></span>
        </div>
        <div className="ui-context-item ui-context-item--version">
            <span className="ui-context-label">Installed version</span>
            <span className="ui-context-value"><FontAwesomeIcon icon={faCloudArrowDown}/><strong>{version}</strong>{channel && <small>· {channel}</small>}</span>
        </div>
        <div className="ui-context-item ui-context-item--mode">
            <span className="ui-context-label">Game mode</span>
            <span className="ui-context-value"><FontAwesomeIcon icon={faGamepad}/><strong>{modeLabel(activeProfile.game_mode)}</strong></span>
        </div>
        <div className="ui-context-actions" aria-label="Factorio process controls">
            {serverStatus?.running
                ? <>
                    <Button size="sm" onClick={stopServer} isLoading={isStopping || serverStatus?.stopping} isDisabled={isKilling || serverStatus?.stopping}>
                        <FontAwesomeIcon icon={faStop}/> <span className="ui-context-action-label">Save & stop</span>
                    </Button>
                    <Button size="sm" type="danger" onClick={() => setIsKillDialogOpen(true)} isDisabled={busy} title="Immediately kill Factorio without saving">
                        <FontAwesomeIcon icon={faSkullCrossbones}/> <span className="ui-context-action-label">Force stop</span>
                    </Button>
                </>
                : <Button
                    size="sm"
                    type="success"
                    onClick={startServer}
                    isLoading={isStarting}
                    isDisabled={busy || !activeProfile.selected_save}
                    title={activeProfile.selected_save ? `Start ${activeProfile.selected_save}` : "Create or upload a save before starting Factorio"}
                >
                    <FontAwesomeIcon icon={faPlay}/> <span className="ui-context-action-label">Start server</span>
                </Button>}
        </div>
        <ConfirmDialog
            title="Force-stop Factorio?"
            content="This kills the game process immediately. Recent progress may be lost. Use Save & stop whenever possible."
            isOpen={isKillDialogOpen}
            close={() => setIsKillDialogOpen(false)}
            onSuccess={killServer}
        />
    </section>;
};

export default ProfileContextBar;
