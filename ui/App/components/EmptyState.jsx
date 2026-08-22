import React from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faBoxOpen} from "@fortawesome/free-solid-svg-icons";

const EmptyState = ({title, description, icon = faBoxOpen}) => (
    <div className="ui-empty-state">
        <div>
            <div className="ui-empty-state__icon"><FontAwesomeIcon icon={icon}/></div>
            <p className="font-bold text-white">{title}</p>
            {description && <p className="mt-1 text-sm">{description}</p>}
        </div>
    </div>
);

export default EmptyState;
