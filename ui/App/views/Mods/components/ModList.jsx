import React from "react";
import {faPuzzlePiece} from "@fortawesome/free-solid-svg-icons";
import Mod from "./Mod";
import EmptyState from "../../../components/EmptyState";

const ModList = ({mods = [], factorioVersion, updateMod, toggleMod, deleteMod, addUpdatableMod = null, disabled = false}) => {
    if (mods.length === 0) return <EmptyState icon={faPuzzlePiece} title="No mods in this set"/>;

    return <div className="ui-table-wrap">
        <table className="ui-table">
            <thead><tr><th>Mod</th><th>Enabled</th><th>Compatibility</th><th>Version</th><th>Factorio</th><th className="text-right">Actions</th></tr></thead>
            <tbody>{factorioVersion !== null && mods.map(mod => <Mod
                mod={mod}
                key={mod.name || `${mod.title}-${mod.version}`}
                updateMod={updateMod}
                toggleMod={toggleMod}
                deleteMod={deleteMod}
                addUpdatableMod={addUpdatableMod}
                factorioVersion={factorioVersion}
                disabled={disabled}
            />)}</tbody>
        </table>
    </div>;
};

export default ModList;
