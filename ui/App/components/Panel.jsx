import React from "react";
import HelpTip from "./HelpTip";

const Panel = ({title, description, help = description, content, actions, className = "", headerAction = null}) => {
    return (
        <section className={`${className} ui-panel`}>
            {(title || help || headerAction) && <div className="ui-panel__header">
                <div className="ui-panel__heading">
                    {title && <h2 className="ui-panel__title">{title}</h2>}
                    {help && <HelpTip content={help} label={`${title || "Panel"} help`}/>}
                </div>
                {headerAction}
            </div>}
            <div className="ui-panel__body">
                {content}
            </div>
            {actions && <div className="ui-panel__actions">{actions}</div>}
        </section>
    )
}

export default Panel;
