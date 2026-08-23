import React, {useEffect, useMemo, useRef, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faDice, faGlobe, faImage, faPlus, faRotate, faWandMagicSparkles
} from "@fortawesome/free-solid-svg-icons";
import saves from "../../../../api/resources/saves";
import Alert from "../../../components/Alert";
import Button from "../../../components/Button";
import Label from "../../../components/Label";
import HelpTip from "../../../components/HelpTip";
import TabControl from "../../../components/Tabs/TabControl";
import Tab from "../../../components/Tabs/Tab";

const MAX_SEED = 4294967295;

const scaleChoices = [
    {value: "", label: "Preset"},
    {value: "0", label: "Off"},
    {value: "0.17", label: "Very low"},
    {value: "0.5", label: "Low"},
    {value: "1", label: "Normal"},
    {value: "2", label: "High"},
    {value: "4", label: "Very high"},
    {value: "6", label: "Maximum"}
];

const startingAreaChoices = [
    {value: "", label: "Preset default"},
    {value: "0.5", label: "Small"},
    {value: "1", label: "Normal"},
    {value: "2", label: "Big"},
    {value: "4", label: "Very big"},
    {value: "6", label: "Maximum"}
];

const booleanChoices = [
    {value: "", label: "Preset default"},
    {value: "false", label: "Disabled"},
    {value: "true", label: "Enabled"}
];

const randomSeed = () => {
    const value = new Uint32Array(1);
    window.crypto.getRandomValues(value);
    return String(value[0]);
};

const errorText = error => {
    const value = error?.response?.data || error?.message || "The operation failed.";
    return typeof value === "string" ? value.trim() : "The operation failed.";
};

const OptionalSelect = ({id, value, onChange, options, disabled = false}) => <select
    id={id}
    className="ui-select"
    value={value}
    disabled={disabled}
    onChange={event => onChange(event.target.value)}
>
    {options.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
</select>;

const NewWorldForm = ({onSuccess}) => {
    const [options, setOptions] = useState(null);
    const [loadError, setLoadError] = useState("");
    const [name, setName] = useState("");
    const [preset, setPreset] = useState("default");
    const [seed, setSeed] = useState(randomSeed);
    const [previewSize, setPreviewSize] = useState(768);
    const [width, setWidth] = useState("");
    const [height, setHeight] = useState("");
    const [startingArea, setStartingArea] = useState("");
    const [peacefulMode, setPeacefulMode] = useState("");
    const [noEnemiesMode, setNoEnemiesMode] = useState("");
    const [selectedPlanet, setSelectedPlanet] = useState("nauvis");
    const [controlOverrides, setControlOverrides] = useState({});
    const [previews, setPreviews] = useState({});
    const [busyAction, setBusyAction] = useState("");
    const previewURLs = useRef({});

    useEffect(() => {
        let active = true;
        saves.generation.options()
            .then(result => {
                if (!active) return;
                setOptions(result);
                setSelectedPlanet(result?.planets?.[0]?.name || "nauvis");
                if (result?.preview_sizes?.includes(768)) setPreviewSize(768);
                else if (result?.preview_sizes?.length) setPreviewSize(result.preview_sizes[0]);
            })
            .catch(error => active && setLoadError(errorText(error)));
        return () => { active = false; };
    }, []);

    useEffect(() => () => {
        Object.values(previewURLs.current).forEach(url => URL.revokeObjectURL(url));
    }, []);

    const controlsByPlanet = useMemo(() => {
        const grouped = {};
        (options?.controls || []).forEach(control => {
            if (!grouped[control.planet]) grouped[control.planet] = [];
            grouped[control.planet].push(control);
        });
        return grouped;
    }, [options]);

    const updateControl = (name, field, value) => {
        setControlOverrides(current => {
            const next = {...current};
            const control = {...(next[name] || {})};
            if (value === "") delete control[field];
            else control[field] = Number(value);
            if (Object.keys(control).length === 0) delete next[name];
            else next[name] = control;
            return next;
        });
    };

    const optionalNumber = value => value === "" ? undefined : Number(value);
    const optionalBoolean = value => value === "" ? undefined : value === "true";

    const payloadFor = planet => ({
        name,
        preset,
        seed: Number(seed),
        planet,
        preview_size: Number(previewSize),
        width: optionalNumber(width),
        height: optionalNumber(height),
        starting_area: optionalNumber(startingArea),
        peaceful_mode: optionalBoolean(peacefulMode),
        no_enemies_mode: optionalBoolean(noEnemiesMode),
        controls: controlOverrides
    });

    const settingsSignature = planet => {
        const payload = payloadFor(planet);
        delete payload.name;
        return JSON.stringify(payload);
    };

    const validateBasics = requireName => {
        if (requireName && !name.trim()) return "Enter a save name.";
        const numericSeed = Number(seed);
        if (!Number.isInteger(numericSeed) || numericSeed < 0 || numericSeed > MAX_SEED) {
            return `Seed must be a whole number between 0 and ${MAX_SEED}.`;
        }
        for (const [label, value] of [["Width", width], ["Height", height]]) {
            if (value !== "" && (!Number.isInteger(Number(value)) || Number(value) < 0 || Number(value) > 2000000)) {
                return `${label} must be empty for the preset or a whole number from 0 to 2000000.`;
            }
        }
        return "";
    };

    const storePreview = (planet, blob, signature, renderedSeed, renderedSize) => {
        if (previewURLs.current[planet]) URL.revokeObjectURL(previewURLs.current[planet]);
        const url = URL.createObjectURL(blob);
        previewURLs.current[planet] = url;
        setPreviews(current => ({
            ...current,
            [planet]: {url, signature, seed: renderedSeed, size: renderedSize}
        }));
    };

    const generatePreview = async (planet, partOfBatch = false) => {
        const validation = validateBasics(false);
        if (validation) {
            window.flash(validation, "red");
            return false;
        }
        if (!partOfBatch) setBusyAction(`preview:${planet}`);
        try {
            const payload = payloadFor(planet);
            const signature = settingsSignature(planet);
            const blob = await saves.generation.preview(payload);
            storePreview(planet, blob, signature, payload.seed, payload.preview_size);
            return true;
        } catch (error) {
            window.flash(errorText(error), "red");
            return false;
        } finally {
            if (!partOfBatch) setBusyAction("");
        }
    };

    const generateAllPreviews = async () => {
        const validation = validateBasics(false);
        if (validation) {
            window.flash(validation, "red");
            return;
        }
        setBusyAction("preview:all");
        try {
            for (const planet of options.planets) {
                setSelectedPlanet(planet.name);
                const successful = await generatePreview(planet.name, true);
                if (!successful) break;
            }
        } finally {
            setBusyAction("");
        }
    };

    const createWorld = async event => {
        event.preventDefault();
        const validation = validateBasics(true);
        if (validation) {
            window.flash(validation, "red");
            return;
        }
        setBusyAction("create");
        try {
            const created = await saves.generation.create(payloadFor("nauvis"));
            await onSuccess();
            setName("");
            window.flash(`${created?.name || "World"} created successfully.`, "green");
        } catch (error) {
            window.flash(errorText(error), "red");
        } finally {
            setBusyAction("");
        }
    };

    if (loadError) return <Alert type="danger">{loadError}</Alert>;
    if (!options) return <div className="ui-world-loading"><FontAwesomeIcon icon={faGlobe} spin/> Reading Factorio's world generator…</div>;

    const selectedPlanetInfo = options.planets.find(planet => planet.name === selectedPlanet) || options.planets[0];
    const selectedControls = controlsByPlanet[selectedPlanet] || [];
    const selectedPreview = previews[selectedPlanet];
    const selectedPreviewIsCurrent = selectedPreview?.signature === settingsSignature(selectedPlanet);
    const busy = Boolean(busyAction);
    const currentPreset = options.presets.find(item => item.name === preset);
    return <form onSubmit={createWorld} className="ui-world-builder">
        <TabControl>
            <Tab title="Basics">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                        <Label text="Save name" htmlFor="new-world-name" help="The .zip extension is added automatically."/>
                        <input id="new-world-name" className="ui-input" value={name} disabled={busy}
                               placeholder="space-factory.zip" onChange={event => setName(event.target.value)}/>
                    </div>
                    <div>
                        <Label text="Map preset" htmlFor="new-world-preset" help={currentPreset?.description}/>
                        <select id="new-world-preset" className="ui-select" value={preset} disabled={busy}
                                onChange={event => setPreset(event.target.value)}>
                            {options.presets.map(item => <option key={item.name} value={item.name}>{item.label}</option>)}
                        </select>
                    </div>
                    <div>
                        <Label text="Map seed" htmlFor="new-world-seed" help="Every planet preview and the final save use this seed."/>
                        <div className="flex gap-2">
                            <input id="new-world-seed" className="ui-input" type="number" min="0" max={MAX_SEED}
                                   value={seed} disabled={busy} onChange={event => setSeed(event.target.value)}/>
                            <Button type="secondary" title="Generate a random seed" isDisabled={busy} onClick={() => setSeed(randomSeed())}>
                                <FontAwesomeIcon icon={faDice}/>
                            </Button>
                        </div>
                    </div>
                    <div>
                        <Label text="Preview resolution" htmlFor="new-world-preview-size" help="Higher resolutions take longer, especially with large mod packs."/>
                        <select id="new-world-preview-size" className="ui-select" value={previewSize} disabled={busy}
                                onChange={event => setPreviewSize(Number(event.target.value))}>
                            {options.preview_sizes.map(size => <option key={size} value={size}>{size} × {size}</option>)}
                        </select>
                    </div>
                </div>

                <div className="ui-world-subsection">
                    <div className="flex items-center gap-2"><h3>Starting world</h3><HelpTip content="Preset default keeps Factorio's selected preset unchanged." label="Starting world help"/></div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-5 gap-3 mt-4">
                        <div>
                            <Label text="Width" htmlFor="new-world-width" help="0 means infinite."/>
                            <input id="new-world-width" className="ui-input" type="number" min="0" max="2000000"
                                   placeholder="Preset default" value={width} disabled={busy} onChange={event => setWidth(event.target.value)}/>
                        </div>
                        <div>
                            <Label text="Height" htmlFor="new-world-height" help="0 means infinite."/>
                            <input id="new-world-height" className="ui-input" type="number" min="0" max="2000000"
                                   placeholder="Preset default" value={height} disabled={busy} onChange={event => setHeight(event.target.value)}/>
                        </div>
                        <div>
                            <Label text="Starting area" htmlFor="new-world-starting-area"/>
                            <OptionalSelect id="new-world-starting-area" value={startingArea} disabled={busy}
                                            onChange={setStartingArea} options={startingAreaChoices}/>
                        </div>
                        <div>
                            <Label text="Peaceful mode" htmlFor="new-world-peaceful"/>
                            <OptionalSelect id="new-world-peaceful" value={peacefulMode} disabled={busy}
                                            onChange={setPeacefulMode} options={booleanChoices}/>
                        </div>
                        <div>
                            <Label text="No enemies" htmlFor="new-world-no-enemies"/>
                            <OptionalSelect id="new-world-no-enemies" value={noEnemiesMode} disabled={busy}
                                            onChange={setNoEnemiesMode} options={booleanChoices}/>
                        </div>
                    </div>
                </div>
            </Tab>

            <Tab title="Planet settings">
                <div className="ui-world-planets" role="group" aria-label="Planet settings">
                    {options.planets.map(planet => <button key={planet.name} type="button"
                        aria-pressed={planet.name === selectedPlanet}
                        className={planet.name === selectedPlanet ? "is-active" : ""}
                        onClick={() => setSelectedPlanet(planet.name)}>
                        <strong>{planet.label}</strong><small>{planet.name === "nauvis" ? "Starting world" : "Space Age"}</small>
                    </button>)}
                </div>

                <div className="ui-world-planet-heading">
                    <div><span className="ui-world-orbit"><FontAwesomeIcon icon={faGlobe}/></span></div>
                    <div><div className="flex items-center gap-2"><h3>{selectedPlanetInfo.label}</h3><HelpTip content={`${selectedPlanetInfo.description} “Preset” leaves values untouched; overrides are shared by previews and save creation.`} label={`${selectedPlanetInfo.label} settings help`}/></div></div>
                    {Object.keys(controlOverrides).length > 0 && <Button size="sm" type="ghost" isDisabled={busy}
                        onClick={() => setControlOverrides({})}><FontAwesomeIcon icon={faRotate}/> Reset all overrides</Button>}
                </div>

                {selectedControls.length === 0
                    ? <Alert>Factorio does not expose adjustable autoplace controls for this planet.</Alert>
                    : <div className="ui-table-wrap"><table className="ui-table ui-world-controls">
                        <thead><tr><th>Feature</th><th>Frequency</th><th>Size</th><th>Richness</th></tr></thead>
                        <tbody>{selectedControls.map(control => {
                            const current = controlOverrides[control.name] || {};
                            const choices = control.can_disable ? scaleChoices : scaleChoices.filter(choice => choice.value !== "0");
                            return <tr key={control.name}>
                                <td><strong className="text-white">{control.label}</strong><small>{control.category}</small></td>
                                <td><OptionalSelect id={`${control.name}-frequency`} value={current.frequency ?? ""} disabled={busy}
                                                    onChange={value => updateControl(control.name, "frequency", value)} options={choices}/></td>
                                <td><OptionalSelect id={`${control.name}-size`} value={current.size ?? ""} disabled={busy}
                                                    onChange={value => updateControl(control.name, "size", value)} options={choices}/></td>
                                <td>{control.supports_richness
                                    ? <OptionalSelect id={`${control.name}-richness`} value={current.richness ?? ""} disabled={busy}
                                                      onChange={value => updateControl(control.name, "richness", value)} options={choices}/>
                                    : <span className="text-gray-light">—</span>}</td>
                            </tr>;
                        })}</tbody>
                    </table></div>}
            </Tab>

            <Tab title="Map preview">
                <div className="ui-world-preview-toolbar">
                    <div>
                        <div className="flex items-center gap-2"><h3>Factorio-rendered preview</h3><HelpTip content="The headless game renders this image with the active version, game mode and mods." label="Map preview help"/></div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                        <Button type="secondary" isDisabled={busy} isLoading={busyAction === `preview:${selectedPlanet}`}
                                onClick={() => generatePreview(selectedPlanet)}>
                            <FontAwesomeIcon icon={faImage}/> Preview {selectedPlanetInfo.label}
                        </Button>
                        {options.planets.length > 1 && <Button type="ghost" isDisabled={busy} isLoading={busyAction === "preview:all"}
                                onClick={generateAllPreviews}>
                            <FontAwesomeIcon icon={faWandMagicSparkles}/> Preview all planets
                        </Button>}
                    </div>
                </div>

                <div className="ui-world-planets ui-world-planets--preview" role="group" aria-label="Planet previews">
                    {options.planets.map(planet => <button key={planet.name} type="button"
                        aria-pressed={planet.name === selectedPlanet}
                        className={planet.name === selectedPlanet ? "is-active" : ""}
                        onClick={() => setSelectedPlanet(planet.name)}>
                        <strong>{planet.label}</strong>
                        <small>{previews[planet.name]
                            ? previews[planet.name].signature === settingsSignature(planet.name) ? "Ready" : "Refresh needed"
                            : "Not rendered"}</small>
                    </button>)}
                </div>

                <div className="ui-world-preview">
                    {selectedPreview
                        ? <>
                            <img src={selectedPreview.url} alt={`${selectedPlanetInfo.label} map preview for seed ${selectedPreview.seed}`}/>
                            {!selectedPreviewIsCurrent && <div className="ui-world-preview__stale">
                                <FontAwesomeIcon icon={faRotate}/> Settings changed — render this planet again
                            </div>}
                            <div className="ui-world-preview__caption">
                                <span><strong>{selectedPlanetInfo.label}</strong> · seed {selectedPreview.seed}</span>
                                <span>{selectedPreview.size} × {selectedPreview.size}</span>
                            </div>
                        </>
                        : <div className="ui-world-preview__empty">
                            <FontAwesomeIcon icon={faImage}/>
                            <strong>No {selectedPlanetInfo.label} preview yet</strong>
                        </div>}
                </div>
            </Tab>
        </TabControl>

        <div className="ui-world-actions">
            <Button type="success" isSubmit isLoading={busyAction === "create"} isDisabled={busy}>
                <FontAwesomeIcon icon={faPlus}/> Create new world
            </Button>
        </div>
    </form>;
};

export default NewWorldForm;
