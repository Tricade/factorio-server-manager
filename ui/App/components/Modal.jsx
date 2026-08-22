import Panel from "./Panel";
import React, {useEffect} from "react";
import * as ReactDom from "react-dom";

const modalRoot = document.getElementById('modal-root');

const Modal = ({title, content, isOpen, actions = null}) => {
    useEffect(() => {
        if (!isOpen) return undefined;
        const originalOverflow = document.body.style.overflow;
        document.body.style.overflow = "hidden";
        return () => {
            document.body.style.overflow = originalOverflow;
        };
    }, [isOpen]);

    if (!isOpen) return null;

    return ReactDom.createPortal(
        <div className="ui-modal-overlay" role="dialog" aria-modal="true" aria-label={title}>
            <Panel title={title} className="ui-modal-panel" content={content} actions={actions}/>
        </div>,
        modalRoot
    )
}

export default Modal;
