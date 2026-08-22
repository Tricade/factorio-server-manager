import React from "react";

const StatusBadge = ({status}) => {
    const normalized = status === "running" || status === "stopping" ? status : "stopped";
    const labels = {running: "Running", stopping: "Stopping", stopped: "Stopped"};
    return <span className={`ui-status-badge ui-status-badge--${normalized}`}>{labels[normalized]}</span>;
};

export default StatusBadge;
