import React, {useEffect, useId, useRef, useState} from "react";
import TabTitle from "./TabTitle";

const TabControl = ({children}) => {
    const tabs = React.Children.toArray(children);
    const [selectedTab, setSelectedTab] = useState(0);
    const tabRefs = useRef([]);
    const id = useId();

    useEffect(() => {
        if (selectedTab >= tabs.length) setSelectedTab(Math.max(0, tabs.length - 1));
    }, [selectedTab, tabs.length]);

    const selectFromKeyboard = (event, index) => {
        let nextIndex = index;
        if (event.key === "ArrowRight") nextIndex = (index + 1) % tabs.length;
        else if (event.key === "ArrowLeft") nextIndex = (index - 1 + tabs.length) % tabs.length;
        else if (event.key === "Home") nextIndex = 0;
        else if (event.key === "End") nextIndex = tabs.length - 1;
        else return;
        event.preventDefault();
        setSelectedTab(nextIndex);
        tabRefs.current[nextIndex]?.focus();
    };

    if (tabs.length === 0) return null;

    return (
        <div className="ui-tabs">
            <div className="ui-tabs__titles" role="tablist">
                {tabs.map((item, index) => (
                    <TabTitle
                        key={item.key || index}
                        title={item.props.title}
                        index={index}
                        isActive={index === selectedTab}
                        setSelectedTab={setSelectedTab}
                        id={`${id}-tab-${index}`}
                        controls={`${id}-panel-${index}`}
                        buttonRef={element => { tabRefs.current[index] = element; }}
                        onKeyDown={event => selectFromKeyboard(event, index)}
                    />
                ))}
            </div>
            <div
                className="ui-tabs__content"
                id={`${id}-panel-${selectedTab}`}
                role="tabpanel"
                aria-labelledby={`${id}-tab-${selectedTab}`}
                tabIndex={0}
            >
                {tabs[selectedTab]}
            </div>
        </div>
    )
}

export default TabControl
