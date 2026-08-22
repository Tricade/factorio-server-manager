import React from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCircleExclamation, faCircleInfo, faTriangleExclamation} from "@fortawesome/free-solid-svg-icons";

const Alert = ({children, type = "info", className = ""}) => {
    const icons = {info: faCircleInfo, warning: faTriangleExclamation, danger: faCircleExclamation};
    const normalized = icons[type] ? type : "info";
    return <div className={`${className} ui-alert ui-alert--${normalized}`} role={normalized === "danger" ? "alert" : undefined}>
        <FontAwesomeIcon className="mt-1 flex-shrink-0" icon={icons[normalized]}/>
        <div>{children}</div>
    </div>;
};

export default Alert;
