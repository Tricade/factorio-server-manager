import React from "react";

const ButtonLink = ({children, href, type = "default", target, className = "", size, download}) => {
    const variant = ["success", "danger", "secondary", "ghost"].includes(type) ? type : "default";

    return (
        <a
            href={href}
            target={target || "_self"}
            rel={target === "_blank" ? "noreferrer" : undefined}
            download={download}
            className={`${className} ui-button ui-button--${variant}${size === "sm" ? " ui-button--sm" : ""}`}>
            {children}
        </a>
    );
}

export default ButtonLink;
