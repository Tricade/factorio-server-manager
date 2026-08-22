import React, {useCallback} from "react";

const TabTitle = ({ title, setSelectedTab, index, isActive }) => {

    const onClick = useCallback(() => {
        setSelectedTab(index)
    }, [setSelectedTab, index])

    return (
            <button
                type="button"
                role="tab"
                aria-selected={isActive}
                className={`ui-tab-title${isActive ? " is-active" : ""}`}
                onClick={onClick}
            >{title}</button>
    )
}

export default TabTitle;
