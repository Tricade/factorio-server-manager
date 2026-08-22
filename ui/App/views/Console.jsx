import React, {useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faPaperPlane, faTerminal} from "@fortawesome/free-solid-svg-icons";
import socket from "../../api/socket";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Button from "../components/Button";
import Alert from "../components/Alert";
import LogViewport from "../components/LogViewport";

const Console = ({serverStatus, socketState}) => {
    const [logs, setLogs] = useState([]);
    const [command, setCommand] = useState("");
    const running = Boolean(serverStatus?.running && !serverStatus?.stopping);

    useEffect(() => {
        const appendLog = line => setLogs(lines => [...lines.slice(-499), line]);
        const commandError = message => window.flash(message, "red");
        socket.on("gamelog", appendLog);
        socket.on("command_error", commandError);
        socket.emit("log subscribe");
        return () => {
            socket.off("gamelog", appendLog);
            socket.off("command_error", commandError);
            socket.emit("log unsubscribe");
        };
    }, []);

    const sendCommand = event => {
        event.preventDefault();
        const trimmed = command.trim();
        if (!trimmed || !running || socketState !== "connected") return;
        socket.emit("command send", trimmed);
        setCommand("");
    };

    return <>
        <PageHeader title="Console"/>
        {!running && <Alert type="warning" className="mb-5">Start Factorio to open its live console.</Alert>}
        {running && socketState !== "connected" && <Alert type="danger" className="mb-5">The live connection is reconnecting. Commands remain disabled until it is back.</Alert>}
        <Panel
            title="Factorio output"
            headerAction={<span className="ui-status-badge">{logs.length} lines</span>}
            content={<>
                <LogViewport
                    lines={logs}
                    ariaLive
                    label="Live Factorio console output"
                    emptyContent={running ? <p className="text-gray-light">Waiting for Factorio output…</p> : null}
                />
                <form className="mt-3 flex gap-2" onSubmit={sendCommand}>
                    <div className="relative flex-1">
                        <FontAwesomeIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-light" icon={faTerminal}/>
                        <input
                            className="ui-input pl-10 font-mono"
                            value={command}
                            onChange={event => setCommand(event.target.value)}
                            disabled={!running || socketState !== "connected"}
                            placeholder="/help"
                            aria-label="Console command"
                        />
                    </div>
                    <Button isSubmit isDisabled={!running || socketState !== "connected" || !command.trim()}><FontAwesomeIcon icon={faPaperPlane}/> Send</Button>
                </form>
            </>}
        />
    </>;
};

export default Console;
