import React, {useState} from "react";
import TabTitle from "./TabTitle";

const TabControl = ({children}) => {
    const [selectedTab, setSelectedTab] = useState(0)

    return (
        <div className="ui-tabs">
            <div className="ui-tabs__titles" role="tablist">
                {children.map((item, index) => (
                    <TabTitle
                        key={index}
                        title={item.props.title}
                        index={index}
                        isActive={index === selectedTab}
                        setSelectedTab={setSelectedTab}
                    />
                ))}
            </div>
            <div className="ui-tabs__content" role="tabpanel">
                {children[selectedTab]}
            </div>
        </div>
    )
}

export default TabControl
