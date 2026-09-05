import React, {useEffect, useMemo, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faFloppyDisk, faMagnifyingGlass, faRotateLeft, faSliders} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../api/resources/mods";
import Panel from "../../../components/Panel";
import Alert from "../../../components/Alert";
import Button from "../../../components/Button";
import EmptyState from "../../../components/EmptyState";
import HelpTip from "../../../components/HelpTip";
import ScopeBadge from "../../../components/ScopeBadge";
import modStartupSettingsHelpers from "../modStartupSettings.cjs";

const {
    cloneSettingValue,
    settingValuesEqual,
    normalizeSettingDraft,
    formatSettingValue,
    colorToHex,
    hexToColor,
    clampColorComponent
} = modStartupSettingsHelpers;

const draftFromResponse = response => Object.fromEntries(
    (response?.groups || []).flatMap(group => group.settings || []).map(setting => [setting.name, cloneSettingValue(setting.value)])
);

const responseMessage = (error, fallback) => {
    const value = error?.response?.data;
    return typeof value === "string" && value.trim() ? value.trim() : fallback;
};

const ModStartupSettingControl = ({setting, value, onChange, disabled, id}) => {
    if (setting.allowed_values?.length) {
        return <select
            className="ui-select"
            id={id}
            value={JSON.stringify(value)}
            disabled={disabled}
            onChange={event => onChange(JSON.parse(event.target.value))}
        >
            {setting.allowed_values.map((allowed, index) => <option value={JSON.stringify(allowed)} key={`${setting.name}-${index}`}>
                {formatSettingValue(allowed)}
            </option>)}
        </select>;
    }
    if (setting.type === "bool-setting") {
        return <label className="ui-checkbox ui-mod-setting__checkbox" htmlFor={id}>
            <input id={id} type="checkbox" checked={Boolean(value)} disabled={disabled} onChange={event => onChange(event.target.checked)}/>
            <span>{value ? "Enabled" : "Disabled"}</span>
        </label>;
    }
    if (setting.type === "color-setting") {
        const color = value || {r: 0, g: 0, b: 0, a: 1};
        return <div className="ui-mod-setting__color-row">
            <input
                id={id}
                className="ui-mod-setting__color"
                type="color"
                value={colorToHex(color)}
                disabled={disabled}
                onChange={event => onChange(hexToColor(event.target.value, color.a ?? 1))}
            />
            <label className="ui-mod-setting__alpha">
                <span>Alpha</span>
                <input
                    className="ui-input"
                    type="number"
                    min="0"
                    max="1"
                    step="0.01"
                    value={color.a ?? 1}
                    disabled={disabled}
                    onChange={event => onChange({...color, a: clampColorComponent(event.target.value)})}
                />
            </label>
            <code>{colorToHex(color)}</code>
        </div>;
    }
    const numeric = setting.type === "int-setting" || setting.type === "double-setting";
    return <input
        className="ui-input"
        id={id}
        type={numeric ? "number" : "text"}
        min={numeric ? setting.minimum_value : undefined}
        max={numeric ? setting.maximum_value : undefined}
        step={setting.type === "int-setting" ? 1 : setting.type === "double-setting" ? "any" : undefined}
        value={value ?? ""}
        disabled={disabled}
        autoComplete="off"
        onChange={event => onChange(event.target.value)}
    />;
};

const ModStartupSettings = ({profileId, statusLocked, refreshKey = 0}) => {
    const [response, setResponse] = useState(null);
    const [draft, setDraft] = useState({});
    const [isLoading, setIsLoading] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [error, setError] = useState("");
    const [search, setSearch] = useState("");
    const [openGroups, setOpenGroups] = useState({});
    const [retryToken, setRetryToken] = useState(0);

    useEffect(() => {
        let active = true;
        setError("");
        setSearch("");
        if (!profileId || statusLocked) {
            setResponse(null);
            setDraft({});
            setOpenGroups({});
            setIsLoading(false);
            return () => { active = false; };
        }
        setIsLoading(true);
        modsResource.startupSettings.get()
            .then(result => {
                if (!active) return;
                setResponse(result);
                setDraft(draftFromResponse(result));
                setOpenGroups(result?.groups?.length ? {[result.groups[0].mod]: true} : {});
            })
            .catch(loadError => {
                if (!active) return;
                setResponse(null);
                setDraft({});
                setOpenGroups({});
                setError(responseMessage(loadError, "Mod startup settings could not be loaded."));
            })
            .finally(() => {
                if (active) setIsLoading(false);
            });
        return () => { active = false; };
    }, [profileId, statusLocked, refreshKey, retryToken]);

    const allSettings = useMemo(() => (response?.groups || []).flatMap(group => group.settings || []), [response]);
    const dirtySettings = useMemo(() => allSettings.filter(setting => !settingValuesEqual(draft[setting.name], setting.value)), [allSettings, draft]);
    const hasNonDefault = useMemo(() => allSettings.some(setting => !settingValuesEqual(draft[setting.name], setting.default_value)), [allSettings, draft]);
    const filteredGroups = useMemo(() => {
        const query = search.trim().toLowerCase();
        if (!query) return response?.groups || [];
        return (response?.groups || []).map(group => ({
            ...group,
            settings: group.settings.filter(setting => [group.title, group.mod, setting.display_name, setting.name, setting.description]
                .some(value => String(value || "").toLowerCase().includes(query)))
        })).filter(group => group.settings.length > 0);
    }, [response, search]);

    const save = async () => {
        let changes;
        try {
            changes = dirtySettings.map(setting => ({name: setting.name, value: normalizeSettingDraft(setting, draft[setting.name])}));
        } catch (validationError) {
            window.flash(validationError.message, "red");
            return;
        }
        if (!changes.length) return;
        setIsSaving(true);
        try {
            const result = await modsResource.startupSettings.update(response.revision, changes);
            setResponse(result);
            setDraft(draftFromResponse(result));
            window.flash("Mod startup settings saved.", "green");
        } catch (saveError) {
            window.flash(responseMessage(saveError, "Mod startup settings could not be saved."), "red");
            if (saveError?.response?.status === 409) setRetryToken(token => token + 1);
        } finally {
            setIsSaving(false);
        }
    };

    const resetAll = () => setDraft(current => ({
        ...current,
        ...Object.fromEntries(allSettings.map(setting => [setting.name, cloneSettingValue(setting.default_value)]))
    }));

    const content = statusLocked
        ? <Alert type="warning">Stop Factorio to load or edit startup settings.</Alert>
        : isLoading
            ? <div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faSliders} spin/><p className="mt-3">Evaluating installed mods…</p></div></div>
            : error
                ? <Alert type="danger"><div className="flex flex-wrap items-center gap-3"><span>{error}</span><Button size="sm" type="secondary" onClick={() => setRetryToken(token => token + 1)}>Retry</Button></div></Alert>
                : !response || allSettings.length === 0
                    ? <EmptyState icon={faSliders} title="No startup settings" description="The enabled mods do not expose visible startup settings."/>
                    : <div className="ui-mod-settings">
                        {allSettings.length > 6 && <label className="ui-mod-settings__search">
                            <span className="sr-only">Search mod startup settings</span>
                            <FontAwesomeIcon icon={faMagnifyingGlass}/>
                            <input className="ui-input" type="search" value={search} placeholder="Search settings" onChange={event => setSearch(event.target.value)}/>
                        </label>}
                        {filteredGroups.length === 0
                            ? <EmptyState icon={faMagnifyingGlass} title="No matching settings"/>
                            : filteredGroups.map((group, groupIndex) => <details
                                className="ui-mod-settings-group"
                                open={Boolean(openGroups[group.mod])}
                                key={group.mod}
                                onToggle={event => {
                                    const isOpen = event.currentTarget.open;
                                    setOpenGroups(current => current[group.mod] === isOpen ? current : {...current, [group.mod]: isOpen});
                                }}
                            >
                                <summary>
                                    <span><strong>{group.title}</strong><code>{group.mod}</code></span>
                                    <span className="ui-status-badge">{group.settings.length}</span>
                                </summary>
                                <div className="ui-mod-settings-grid">
                                    {group.settings.map((setting, settingIndex) => {
                                        const id = `mod-startup-${groupIndex}-${settingIndex}`;
                                        const changed = !settingValuesEqual(draft[setting.name], setting.value);
                                        return <div className={`ui-mod-setting${changed ? " is-dirty" : ""}`} key={setting.name}>
                                            <div className="ui-mod-setting__heading">
                                                <div>
                                                    <div className="ui-label-row">
                                                        <label className="ui-label" htmlFor={id}>{setting.display_name}</label>
                                                        {setting.description && <HelpTip content={setting.description} label={`${setting.display_name} help`}/>}
                                                    </div>
                                                    <code>{setting.name}</code>
                                                </div>
                                                <Button
                                                    className="ui-mod-setting__reset"
                                                    size="sm"
                                                    type="ghost"
                                                    title={`Reset ${setting.display_name} to its default`}
                                                    isDisabled={settingValuesEqual(draft[setting.name], setting.default_value)}
                                                    onClick={() => setDraft(current => ({...current, [setting.name]: cloneSettingValue(setting.default_value)}))}
                                                >
                                                    <FontAwesomeIcon icon={faRotateLeft}/> Reset
                                                </Button>
                                            </div>
                                            <ModStartupSettingControl
                                                setting={setting}
                                                value={draft[setting.name]}
                                                onChange={value => setDraft(current => ({...current, [setting.name]: value}))}
                                                disabled={isSaving}
                                                id={id}
                                            />
                                            <div className="ui-mod-setting__meta">
                                                <span>Default: <strong>{formatSettingValue(setting.default_value)}</strong></span>
                                                {(setting.minimum_value !== undefined || setting.maximum_value !== undefined) && <span>
                                                    Range: <strong>{setting.minimum_value ?? "−∞"} – {setting.maximum_value ?? "∞"}</strong>
                                                </span>}
                                            </div>
                                        </div>;
                                    })}
                                </div>
                            </details>)}
                    </div>;

    return <Panel
        title="Mod startup settings"
        help="Startup values are evaluated by the installed Factorio version, stored with the active profile, and applied the next time Factorio starts. Runtime-global and per-player settings are intentionally unchanged."
        className="mb-5"
        headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><ScopeBadge/>{dirtySettings.length > 0 && <span className="ui-status-badge ui-status-badge--warning">{dirtySettings.length} changed</span>}</div>}
        content={content}
        actions={!statusLocked && response && allSettings.length > 0 ? <>
            <Button size="sm" type="secondary" isDisabled={!hasNonDefault || isSaving} onClick={resetAll}>
                <FontAwesomeIcon icon={faRotateLeft}/> Reset all
            </Button>
            <Button size="sm" isLoading={isSaving} isDisabled={dirtySettings.length === 0} onClick={save}>
                <FontAwesomeIcon icon={faFloppyDisk}/> Save settings
            </Button>
        </> : null}
    />;
};

export default ModStartupSettings;
