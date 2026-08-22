import React, {useCallback, useEffect, useRef, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCheckCircle, faCircleExclamation, faCircleInfo, faXmark} from "@fortawesome/free-solid-svg-icons";
import Bus from "../../notifications";

export const Flash = () => {
    const [toast, setToast] = useState(null);
    const timer = useRef(null);

    const flashListener = useCallback(({message, color}) => {
        if (timer.current) window.clearTimeout(timer.current);
        setToast({message, color});
        timer.current = window.setTimeout(() => setToast(null), 5000);
    }, []);

    useEffect(() => {
        Bus.addListener("flash", flashListener);
        return () => {
            Bus.removeListener("flash", flashListener);
            if (timer.current) window.clearTimeout(timer.current);
        };
    }, [flashListener]);

    if (!toast) return null;
    const icon = toast.color === "green" ? faCheckCircle : toast.color === "red" ? faCircleExclamation : faCircleInfo;

    return <div className="fixed z-50 right-4 bottom-4 md:right-7 md:bottom-7" role="status" aria-live="polite">
        <div className={`ui-toast ui-toast--${toast.color}`}>
            <FontAwesomeIcon className={toast.color === "green" ? "text-green mt-1" : toast.color === "red" ? "text-red mt-1" : "text-blue mt-1"} icon={icon}/>
            <p className="flex-1 text-sm">{toast.message}</p>
            <button className="text-gray-light hover:text-white" onClick={() => setToast(null)} aria-label="Dismiss notification">
                <FontAwesomeIcon icon={faXmark}/>
            </button>
        </div>
    </div>;
};
