import React from "react";

const Select = ({register, options, className = "", defaultValue = "", disabled = undefined, id = undefined}) => {

    return (
        <div className={className}>
        <select
            className="ui-select"
            {...register}
            id={id || register?.name}
            defaultValue={defaultValue}
            disabled={disabled}
        >
            {options.map(option => <option value={option.value} key={option.value}>{option.name}</option>)}
        </select>
        </div>
    )
}

export default Select;
