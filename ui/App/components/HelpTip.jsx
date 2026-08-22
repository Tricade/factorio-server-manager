import React, {useId} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faQuestion} from "@fortawesome/free-solid-svg-icons";

const HelpTip = ({content, label = "More information", className = ""}) => {
    const id = useId();
    if (!content) return null;

    return <span className={`ui-help-tip ${className}`.trim()}>
        <button className="ui-help-tip__trigger" type="button" aria-label={label} aria-describedby={id}>
            <FontAwesomeIcon icon={faQuestion}/>
        </button>
        <span className="ui-help-tip__content" id={id} role="tooltip">{content}</span>
    </span>;
};

export default HelpTip;
