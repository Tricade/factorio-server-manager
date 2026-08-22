import React, {useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faArrowUp, faCheck, faSpinner, faTimes, faToggleOff, faToggleOn, faTrashAlt} from "@fortawesome/free-solid-svg-icons";
import {coerce, gt, satisfies} from "semver";
import modsResource from "../../../../api/resources/mods";
import IconButton from "../../../components/IconButton";

const Mod = ({mod, factorioVersion, toggleMod, deleteMod, updateMod, addUpdatableMod, disabled = false}) => {
    const [newVersion, setNewVersion] = useState(null);
    const [isUpdating, setIsUpdating] = useState(false);

    const runAction = async (action, fallback) => {
        try {
            await action();
        } catch (error) {
            window.flash(error?.response?.data || fallback, "red");
        }
    };

    useEffect(() => {
        let active = true;
        if (disabled || !mod.name) return undefined;
        (async () => {
            try {
                const data = await modsResource.portal.info(mod.name);
                const installedVersion = coerce(mod.version);
                let newestRelease;
                (data.releases || []).forEach(release => {
                    const releaseVersion = coerce(release.version);
                    const requiredFactorioVersion = coerce(release.info_json?.factorio_version);
                    const compatible = installedVersion && releaseVersion && requiredFactorioVersion &&
                        gt(releaseVersion, installedVersion) && (
                            satisfies(factorioVersion, `~${requiredFactorioVersion.version}`) ||
                            (satisfies(factorioVersion, "1.0.0") && satisfies(requiredFactorioVersion, "0.18.x"))
                        );
                    if (compatible && (!newestRelease || gt(releaseVersion, coerce(newestRelease.version)))) newestRelease = release;
                });
                if (!active) return;
                if (newestRelease && newestRelease.version !== mod.version) {
                    const installable = {downloadUrl: newestRelease.download_url, fileName: newestRelease.file_name, modName: mod.name};
                    setNewVersion(installable);
                    if (typeof addUpdatableMod === "function") addUpdatableMod(installable);
                } else {
                    setNewVersion(null);
                }
            } catch (error) {
                if (active) setNewVersion(null);
            }
        })();
        return () => { active = false; };
    }, [disabled, mod.name, mod.version, factorioVersion, addUpdatableMod]);

    const installUpdate = async () => {
        setIsUpdating(true);
        try {
            await updateMod(newVersion);
            window.flash(`${mod.title || mod.name} updated.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "Mod could not be updated.", "red");
        } finally {
            setIsUpdating(false);
        }
    };

    return <tr>
        <td><div className="font-bold text-white">{mod.title || mod.name}</div><div className="text-xs text-gray-light">{mod.name}</div></td>
        <td>{disabled
            ? <FontAwesomeIcon className={mod.enabled ? "text-green" : "text-gray-light"} icon={mod.enabled ? faCheck : faTimes}/>
            : <IconButton
                className={mod.enabled ? "text-green" : "text-gray-light"}
                label={`${mod.enabled ? "Disable" : "Enable"} ${mod.title || mod.name}`}
                icon={mod.enabled ? faToggleOn : faToggleOff}
                onClick={() => runAction(() => toggleMod(mod.name), "Mod could not be toggled.")}
            />}
        </td>
        <td><span className={`ui-status-badge ${mod.compatibility ? "ui-status-badge--running" : "ui-status-badge--stopped"}`}>
            {mod.compatibility ? "Compatible" : "Mismatch"}
        </span></td>
        <td><span className="font-mono">{mod.version}</span>{newVersion && <span className="ml-2 text-xs text-orange">Update</span>}</td>
        <td><span className="font-mono text-sm">{mod.factorio_version}</span></td>
        <td><div className="flex justify-end gap-2">
            {!disabled && newVersion && (
                <IconButton label={`Update ${mod.title || mod.name}`} icon={isUpdating ? faSpinner : faArrowUp} spin={isUpdating} disabled={isUpdating} onClick={installUpdate}/>
            )}
            {!disabled && (
                <IconButton type="danger" label={`Delete ${mod.title || mod.name}`} icon={faTrashAlt} onClick={() => runAction(() => deleteMod(mod.name), "Mod could not be deleted.")}/>
            )}
        </div></td>
    </tr>;
};

export default Mod;
