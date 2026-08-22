import React from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faGear, faLayerGroup} from "@fortawesome/free-solid-svg-icons";

const ScopeBadge = ({scope = "profile", className = ""}) => {
    const managerWide = scope === "manager";
    return <span className={`${className} ui-scope-badge ui-scope-badge--${managerWide ? "manager" : "profile"}`}>
        <FontAwesomeIcon icon={managerWide ? faGear : faLayerGroup}/>
        {managerWide ? "Manager-wide" : "Active profile"}
    </span>;
};

export default ScopeBadge;
