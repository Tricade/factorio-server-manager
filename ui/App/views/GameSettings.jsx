import React, {useCallback, useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faGamepad, faRotate} from "@fortawesome/free-solid-svg-icons";
import settingsResource from "../../api/resources/settings";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import EmptyState from "../components/EmptyState";
import Alert from "../components/Alert";
import Button from "../components/Button";
import ScopeBadge from "../components/ScopeBadge";

const humanize = value => value.replace(/[_-]/g, " ").replace(/\b\w/g, letter => letter.toUpperCase());

const GameSettings = () => {
    const [settingsCategories, setSettingsCategories] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");

    const loadSettings = useCallback(async () => {
        setIsLoading(true);
        setLoadError("");
        try {
            const result = await settingsResource.game.list();
            setSettingsCategories(result || {});
        } catch (error) {
            const message = error?.response?.data || "Game configuration could not be loaded.";
            setLoadError(message);
            setSettingsCategories(null);
            window.flash(message, "red");
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => {
        loadSettings();
    }, [loadSettings]);

    const populatedCategories = Object.entries(settingsCategories || {})
        .filter(([, values]) => values && Object.keys(values).length > 0);
    const settingCount = populatedCategories.reduce((count, [, values]) => count + Object.keys(values).length, 0);

    return <>
        <PageHeader
            title="Game settings"
            help="Read-only Factorio process configuration. Multiplayer rules, autosaves and visibility are editable under Server settings."
            actions={!isLoading && !loadError ? <span className="ui-status-badge">{settingCount} values</span> : null}
        />
        {isLoading
            ? <Panel headerAction={<ScopeBadge/>} content={<div className="ui-empty-state"><div><FontAwesomeIcon className="text-orange" icon={faGamepad} spin/><p className="mt-3">Reading game settings…</p></div></div>}/>
            : loadError
                ? <Panel headerAction={<ScopeBadge/>} content={<div>
                    <Alert type="danger" className="mb-4">{loadError}</Alert>
                    <Button type="secondary" onClick={loadSettings}><FontAwesomeIcon icon={faRotate}/> Try again</Button>
                </div>}/>
            : populatedCategories.length === 0
                ? <Panel headerAction={<ScopeBadge/>} content={<EmptyState icon={faGamepad} title="No game settings found"/>}/>
                : <div className="grid grid-cols-1 xl:grid-cols-2 gap-5">{populatedCategories.map(([category, values]) => <Panel
                    key={category}
                    title={humanize(category)}
                    headerAction={<div className="flex flex-wrap items-center justify-end gap-2"><span className="ui-status-badge">{Object.keys(values).length} {Object.keys(values).length === 1 ? "value" : "values"}</span><ScopeBadge/></div>}
                    content={<dl>{Object.entries(values).map(([name, value]) => <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 py-2.5 border-b border-white border-opacity-5 last:border-0" key={name}>
                            <dt className="text-sm text-gray-light">{humanize(name)}</dt>
                            <dd className="font-mono text-sm text-white break-all">{String(value)}</dd>
                        </div>)}</dl>}
                />)}</div>}
    </>;
};

export default GameSettings;
