import React, {useCallback, useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faArrowsRotate, faFileLines} from "@fortawesome/free-solid-svg-icons";
import log from "../../api/resources/log";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Button from "../components/Button";
import LogViewport from "../components/LogViewport";

const Logs = () => {
    const [logs, setLogs] = useState([]);
    const [isLoading, setIsLoading] = useState(true);

    const loadLogs = useCallback(async () => {
        setIsLoading(true);
        try {
            setLogs(await log.tail() || []);
        } catch (error) {
            window.flash(error?.response?.data || error?.message || "Factorio logs could not be loaded.", "red");
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => { loadLogs(); }, [loadLogs]);

    return <>
        <PageHeader title="Factorio logs" help="Common password and token arguments are redacted." actions={<Button type="secondary" size="sm" isLoading={isLoading} onClick={loadLogs}><FontAwesomeIcon icon={faArrowsRotate}/> Refresh</Button>}/>
        <Panel title="Recent output" headerAction={<span className="ui-status-badge">{logs.length} lines</span>} content={<LogViewport
            lines={logs}
            label="Recent Factorio log output"
            emptyContent={<div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faFileLines}/><p className="mt-3">{isLoading ? "Loading logs…" : "No log lines available."}</p></div></div>}
        />}/>
    </>;
};

export default Logs;
