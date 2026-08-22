import React from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";

const IconButton = ({icon, label, onClick, type = "default", disabled = false, spin = false, className = ""}) => (
    <button
        type="button"
        className={`${className} ui-icon-button${type === "danger" ? " ui-icon-button--danger" : ""}`}
        onClick={onClick}
        disabled={disabled}
        aria-label={label}
        title={label}
    >
        <FontAwesomeIcon icon={icon} spin={spin}/>
    </button>
);

export default IconButton;
