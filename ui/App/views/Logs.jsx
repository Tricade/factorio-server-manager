import React, {useCallback, useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faArrowsRotate, faFileLines} from "@fortawesome/free-solid-svg-icons";
import log from "../../api/resources/log";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Button from "../components/Button";
import LogViewport from "../components/LogViewport";
import Alert from "../components/Alert";
import ScopeBadge from "../components/ScopeBadge";

const Logs = () => {
    const [logs, setLogs] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");

    const loadLogs = useCallback(async () => {
        setIsLoading(true);
        setLoadError("");
        try {
            setLogs(await log.tail() || []);
        } catch (error) {
            setLoadError("Factorio logs could not be loaded.");
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => { loadLogs(); }, [loadLogs]);

    return <>
        <PageHeader title="Factorio logs" help="Common password and token arguments are redacted." actions={<Button type="secondary" size="sm" isLoading={isLoading} onClick={loadLogs}><FontAwesomeIcon icon={faArrowsRotate}/> Refresh</Button>}/>
        <Panel title="Recent output" headerAction={<div className="flex flex-wrap items-center gap-2"><ScopeBadge/><span className="ui-status-badge">{loadError && logs.length === 0 ? "Lines unavailable" : `${logs.length} lines`}</span></div>} content={<>
            {loadError && <Alert type={logs.length ? "warning" : "danger"} className="mb-4">{loadError} {logs.length ? "The last successful output remains visible." : "Use Refresh to try again."}</Alert>}
            <LogViewport
            lines={logs}
            label="Recent Factorio log output"
            emptyContent={<div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faFileLines}/><p className="mt-3">{isLoading ? "Loading logs…" : "No log lines available."}</p></div></div>}
        /></>}/>
    </>;
};

export default Logs;
