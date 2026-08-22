import React from "react";
import {faSpinner} from "@fortawesome/free-solid-svg-icons";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";

const Button = ({children, type = "default", onClick, isSubmit, className = "", size, isLoading, isDisabled = false, title, form}) => {
    const variant = ["success", "danger", "secondary", "ghost"].includes(type) ? type : "default";

    return (
        <button
            onClick={onClick}
            disabled={isDisabled || isLoading}
            className={`${className} ui-button ui-button--${variant}${size === "sm" ? " ui-button--sm" : ""}`}
            type={isSubmit ? "submit" : "button"}
            form={form}
            aria-busy={isLoading || undefined}
            title={title}
        >
            {isLoading && <FontAwesomeIcon icon={faSpinner} spin={true}/>} {children}
        </button>
    );
}

export default Button;
