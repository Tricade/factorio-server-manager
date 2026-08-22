import React from "react";
import HelpTip from "./HelpTip";

const Label = ({text, htmlFor, help = null}) => {
    return (
        <div className="ui-label-row">
            <label className="ui-label" htmlFor={htmlFor}>{text}</label>
            {help && <HelpTip content={help} label={`${text} help`}/>}
        </div>
    )
}

export default Label;
