import React from "react";
import HelpTip from "./HelpTip";

const Checkbox = ({text, register, checked, help = null}) => {
    return (
        <div className="ui-checkbox-row">
            <label className="ui-checkbox">
                <input
                    {...register}
                    id={register?.name}
                    type="checkbox" defaultChecked={checked}/>
                <span>{text}</span>
            </label>
            {help && <HelpTip content={help} label={`${text} help`}/>}
        </div>
    )
}

export default Checkbox;
