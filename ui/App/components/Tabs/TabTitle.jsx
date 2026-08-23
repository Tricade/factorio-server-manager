import React, {useCallback} from "react";

const TabTitle = ({title, setSelectedTab, index, isActive, id, controls, buttonRef, onKeyDown}) => {

    const onClick = useCallback(() => {
        setSelectedTab(index)
    }, [setSelectedTab, index])

    return (
            <button
                type="button"
                role="tab"
                id={id}
                aria-controls={controls}
                aria-selected={isActive}
                tabIndex={isActive ? 0 : -1}
                className={`ui-tab-title${isActive ? " is-active" : ""}`}
                onClick={onClick}
                onKeyDown={onKeyDown}
                ref={buttonRef}
            >{title}</button>
    )
}

export default TabTitle;
