import Panel from "./Panel";
import React, {useEffect, useRef} from "react";
import * as ReactDom from "react-dom";
import {focusableElements, lockBodyScroll, trapFocus} from "./overlay";

const modalRoot = document.getElementById('modal-root');

const Modal = ({title, content, isOpen, actions = null, close = null, dismissDisabled = false}) => {
    const dialogRef = useRef(null);
    const closeRef = useRef(close);
    const dismissDisabledRef = useRef(dismissDisabled);
    closeRef.current = close;
    dismissDisabledRef.current = dismissDisabled;

    useEffect(() => {
        if (!isOpen) return undefined;
        const previouslyFocused = document.activeElement;
        const unlockBodyScroll = lockBodyScroll();
        const focusDialog = window.requestAnimationFrame(() => {
            const firstControl = focusableElements(dialogRef.current)[0];
            (firstControl || dialogRef.current)?.focus();
        });
        const handleKeyDown = event => {
            if (event.key === "Escape" && !dismissDisabledRef.current && closeRef.current) {
                event.preventDefault();
                closeRef.current();
                return;
            }
            trapFocus(event, dialogRef.current);
        };
        document.addEventListener("keydown", handleKeyDown);
        return () => {
            window.cancelAnimationFrame(focusDialog);
            document.removeEventListener("keydown", handleKeyDown);
            unlockBodyScroll();
            if (previouslyFocused instanceof HTMLElement && previouslyFocused.isConnected) previouslyFocused.focus();
        };
    }, [isOpen]);

    if (!isOpen) return null;

    return ReactDom.createPortal(
        <div
            ref={dialogRef}
            className="ui-modal-overlay"
            role="dialog"
            aria-modal="true"
            aria-label={title}
            tabIndex={-1}
            onPointerDown={event => {
                if (event.target === event.currentTarget && !dismissDisabled && close) close();
            }}
        >
            <Panel title={title} className="ui-modal-panel" content={content} actions={actions}/>
        </div>,
        modalRoot
    )
}

export default Modal;
