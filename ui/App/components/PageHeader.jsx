import React from "react";
import HelpTip from "./HelpTip";

const PageHeader = ({title, description, help = description, actions = null}) => (
    <header className="ui-page-header">
        <div>
            <div className="ui-page-title-row">
                <h1 className="ui-page-title">{title}</h1>
                {help && <HelpTip content={help} label={`${title} help`}/>}
            </div>
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </header>
);

export default PageHeader;
