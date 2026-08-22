import React from "react";

const Input = ({
                   register,
                   placeholder = undefined,
                   type = "text",
                   defaultValue = undefined,
                   hasAutoComplete = true,
                   onKeyDown = () => undefined,
                   min = undefined,
                   max = undefined,
                   value = undefined,
                   disabled = false,
                   className = "",
                   id = undefined,
                   step = undefined,
                   autoFocus = false
               }) => {
    return (
        <input
            className={`${className} ui-input`}
            placeholder={placeholder}
            {...register}
            id={id || register?.name}
            type={type}
            onKeyDown={onKeyDown}
            autoComplete={hasAutoComplete ? "on" : "off"}
            defaultValue={defaultValue}
            min={min}
            max={max}
            step={step}
            value={value}
            disabled={disabled}
            autoFocus={autoFocus}
        />
    )
}

export default Input;
