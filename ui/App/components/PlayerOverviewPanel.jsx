import React, {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faClock, faRotate, faUsers} from "@fortawesome/free-solid-svg-icons";
import serverResource from "../../api/resources/server";
import Alert from "./Alert";
import Button from "./Button";
import Panel from "./Panel";
import ScopeBadge from "./ScopeBadge";

const errorText = error => {
    const value = error?.response?.data?.error || error?.response?.data || error?.message;
    return typeof value === "string" && value.trim() ? value.trim() : "Player data could not be loaded.";
};

const formatDate = value => {
    const date = value ? new Date(value) : null;
    return !date || Number.isNaN(date.getTime()) ? "Unknown time" : date.toLocaleString();
};

const formatPlaytime = value => {
    const seconds = Math.max(0, Number(value) || 0);
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m`;
    return seconds > 0 ? "<1m" : "0m";
};

const validateOverview = (result, profileID) => {
    if (!result || result.profile_id !== profileID || !Array.isArray(result.online_players) || !Array.isArray(result.players)) {
        throw new Error("The player endpoint returned data for a different or invalid profile.");
    }
    return result;
};

const PlayerOverviewPanel = ({profileID, serverStatus}) => {
    const [overview, setOverview] = useState(null);
    const [loadError, setLoadError] = useState("");
    const [isLoading, setIsLoading] = useState(true);
    const requestID = useRef(0);
    const running = Boolean(serverStatus?.known !== false && serverStatus?.running);

    const load = useCallback(async (showLoading = false) => {
        const currentRequest = ++requestID.current;
        if (showLoading) setIsLoading(true);
        try {
            const result = validateOverview(await serverResource.players(), profileID);
            if (currentRequest !== requestID.current) return;
            setOverview(result);
            setLoadError("");
        } catch (error) {
            if (currentRequest === requestID.current) setLoadError(errorText(error));
        } finally {
            if (currentRequest === requestID.current) setIsLoading(false);
        }
    }, [profileID]);

    useEffect(() => {
        setOverview(null);
        setLoadError("");
        load(true);
    }, [load, running]);

    useEffect(() => {
        if (!running) return undefined;
        const timer = window.setInterval(() => load(false), 15000);
        return () => window.clearInterval(timer);
    }, [load, running]);

    const rankedPlayers = useMemo(() => (overview?.players || [])
        .filter(player => Number(player.rank) > 0 || Number(player.online_time_seconds) > 0)
        .sort((left, right) => Number(left.rank || Number.MAX_SAFE_INTEGER) - Number(right.rank || Number.MAX_SAFE_INTEGER)), [overview]);

    const liveStatus = !overview?.server_running
        ? "Server stopped"
        : overview.live_available ? `${overview.online_count} online` : "Live unavailable";

    return <Panel
        className="ui-player-overview mt-5"
        title="Players"
        help="Live presence comes from the running server. Playtime is read from the latest completed factory-map snapshot and may be older."
        headerAction={<div className="flex flex-wrap items-center justify-end gap-2">
            <ScopeBadge/>
            {overview && <span className={`ui-status-badge ${overview.server_running && overview.live_available ? "ui-status-badge--running" : "ui-status-badge--stopped"}`}>{liveStatus}</span>}
        </div>}
        content={isLoading && !overview
            ? <div className="ui-player-loading"><FontAwesomeIcon icon={faRotate} spin/> Loading player data…</div>
            : loadError && !overview
                ? <div>
                    <Alert type="danger" className="mb-4">{loadError}</Alert>
                    <Button type="secondary" onClick={() => load(true)} isLoading={isLoading}><FontAwesomeIcon icon={faRotate}/> Try again</Button>
                </div>
                : <div className="ui-player-overview__grid">
                    <section className="ui-player-live" aria-labelledby="player-live-heading">
                        <div className="ui-player-section-heading">
                            <div><FontAwesomeIcon icon={faUsers}/><h3 id="player-live-heading">Online now</h3></div>
                            <Button type="ghost" size="sm" onClick={() => load(true)} isLoading={isLoading} title="Refresh player data"><FontAwesomeIcon icon={faRotate}/> Refresh</Button>
                        </div>
                        {loadError && <Alert type="warning" className="mb-3">Refresh failed. The last successful data remains visible.</Alert>}
                        {!overview?.server_running
                            ? <p className="ui-player-note">Factorio is stopped, so no live presence is available.</p>
                            : !overview.live_available
                                ? <Alert type="warning">The live player list is temporarily unavailable.</Alert>
                                : overview.online_players.length === 0
                                    ? <p className="ui-player-note">No players are online.</p>
                                    : <ul className="ui-player-chips" aria-label={`${overview.online_count} players online`}>
                                        {overview.online_players.map(name => <li key={name}>{name}</li>)}
                                    </ul>}
                    </section>

                    <section className="ui-player-ranking" aria-labelledby="player-ranking-heading">
                        <div className="ui-player-section-heading">
                            <div><FontAwesomeIcon icon={faClock}/><div><h3 id="player-ranking-heading">Playtime ranking</h3>
                                {overview?.statistics_as_of && <small title={`Game tick ${overview.statistics_game_tick || 0}`}>Snapshot {formatDate(overview.statistics_as_of)}{overview.statistics_save_name ? ` · ${overview.statistics_save_name}` : ""}</small>}
                            </div></div>
                        </div>
                        {!overview?.statistics_as_of
                            ? <p className="ui-player-note">No playtime snapshot yet. Generate a factory map to collect one.</p>
                            : rankedPlayers.length === 0
                                ? <p className="ui-player-note">This snapshot does not contain player playtime yet.</p>
                                : <div className="ui-table-wrap ui-player-ranking__table-wrap">
                                    <table className="ui-table ui-player-ranking__table">
                                        <thead><tr><th>Rank</th><th>Player</th><th>Playtime</th><th>Live</th></tr></thead>
                                        <tbody>{rankedPlayers.map(player => <tr key={player.name}>
                                            <td>#{player.rank}</td>
                                            <td className="font-bold text-white">{player.name}</td>
                                            <td>{formatPlaytime(player.online_time_seconds)}</td>
                                            <td>{player.online ? <span className="text-green">Online</span> : <span className="text-gray-light">—</span>}</td>
                                        </tr>)}</tbody>
                                    </table>
                                </div>}
                    </section>
                </div>}
    />;
};

export default PlayerOverviewPanel;
