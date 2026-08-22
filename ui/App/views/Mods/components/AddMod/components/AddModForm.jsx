import React, {useEffect, useRef, useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faArrowRightFromBracket, faExternalLinkAlt, faMagnifyingGlass, faSpinner} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../../../api/resources/mods";
import Button from "../../../../../components/Button";
import ButtonLink from "../../../../../components/ButtonLink";
import Label from "../../../../../components/Label";
import Input from "../../../../../components/Input";
import Error from "../../../../../components/Error";
import SelectVersionForm from "./SelectVersionForm";
import DependencyInstallForm from "./DependencyInstallForm";

const AddModForm = ({setIsFactorioAuthenticated, fuse, factorioVersion, refetchInstalledMods}) => {
    const {register, watch, setValue, handleSubmit, formState: {errors}} = useForm();
    const [suggestedMods, setSuggestedMods] = useState([]);
    const [selectedMod, setSelectedMod] = useState(null);
    const [hoveredMod, setHoveredMod] = useState(0);
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [isLoadingVersions, setIsLoadingVersions] = useState(false);
    const [releases, setReleases] = useState([]);
    const [selectedRelease, setSelectedRelease] = useState(null);
    const [dependencyPlan, setDependencyPlan] = useState(null);
    const [selectedOptional, setSelectedOptional] = useState([]);
    const [isDependencyModalOpen, setIsDependencyModalOpen] = useState(false);
    const [isPlanningDependencies, setIsPlanningDependencies] = useState(false);
    const [isInstallingPlan, setIsInstallingPlan] = useState(false);
    const debounceTimer = useRef(null);
    const query = watch("mod", "");

    useEffect(() => {
        if (!fuse || selectedMod?.item?.title === query) return undefined;
        if (debounceTimer.current) window.clearTimeout(debounceTimer.current);
        debounceTimer.current = window.setTimeout(() => {
            setHoveredMod(0);
            setSuggestedMods(query.trim().length >= 2 ? fuse.search(query, {limit: 8}) : []);
        }, 160);
        return () => window.clearTimeout(debounceTimer.current);
    }, [query, fuse, selectedMod]);

    const selectMod = result => {
        if (debounceTimer.current) window.clearTimeout(debounceTimer.current);
        setValue("mod", result.item.title, {shouldValidate: true});
        setSelectedMod(result);
        setSuggestedMods([]);
        setReleases([]);
        setSelectedRelease(null);
        setDependencyPlan(null);
        setSelectedOptional([]);
    };

    const handleKeyDown = event => {
        if (event.key === "ArrowDown") {
            event.preventDefault();
            setHoveredMod(index => Math.min(index + 1, suggestedMods.length - 1));
        } else if (event.key === "ArrowUp") {
            event.preventDefault();
            setHoveredMod(index => Math.max(index - 1, 0));
        } else if (event.key === "Enter" && suggestedMods[hoveredMod]) {
            event.preventDefault();
            selectMod(suggestedMods[hoveredMod]);
        } else if (event.key === "Escape") {
            setSuggestedMods([]);
        }
    };

    const openSelectVersionModal = async () => {
        if (!selectedMod) return;
        setIsLoadingVersions(true);
        try {
            const modInfo = await modsResource.portal.info(selectedMod.item.name);
            setReleases((modInfo.releases || []).filter(release => release.compatibility));
            setIsModalOpen(true);
        } catch (error) {
            window.flash(error?.response?.data || "Mod versions could not be loaded.", "red");
        } finally {
            setIsLoadingVersions(false);
        }
    };

    const reviewDependencies = async release => {
        setIsPlanningDependencies(true);
        try {
            const plan = await modsResource.portal.planInstall(selectedMod.item.name, release.version, []);
            setSelectedRelease(release);
            setSelectedOptional([]);
            setDependencyPlan(plan);
            setIsDependencyModalOpen(true);
            return true;
        } catch (error) {
            window.flash(error?.response?.data || "Mod dependencies could not be resolved.", "red");
            return false;
        } finally {
            setIsPlanningDependencies(false);
        }
    };

    const toggleOptionalDependency = async (name, enabled) => {
        if (!selectedRelease) return;
        const nextSelection = enabled
            ? [...new Set([...selectedOptional, name])]
            : selectedOptional.filter(candidate => candidate !== name);
        setSelectedOptional(nextSelection);
        setIsPlanningDependencies(true);
        try {
            const plan = await modsResource.portal.planInstall(selectedMod.item.name, selectedRelease.version, nextSelection);
            setDependencyPlan(plan);
        } catch (error) {
            setSelectedOptional(selectedOptional);
            window.flash(error?.response?.data || "Optional dependencies could not be resolved.", "red");
        } finally {
            setIsPlanningDependencies(false);
        }
    };

    const installPlan = async () => {
        if (!selectedRelease) return;
        setIsInstallingPlan(true);
        try {
            await modsResource.portal.installResolved(selectedMod.item.name, selectedRelease.version, selectedOptional);
            await refetchInstalledMods();
            setIsDependencyModalOpen(false);
            window.flash(`${selectedMod.item.title} and its selected dependencies were installed.`, "green");
        } catch (error) {
            const message = error?.response?.data?.error || error?.response?.data || "Mod set could not be installed.";
            window.flash(message, "red");
        } finally {
            setIsInstallingPlan(false);
        }
    };

    const logout = async () => {
        try {
            await modsResource.portal.logout();
            setIsFactorioAuthenticated(false);
        } catch (error) {
            window.flash("Factorio portal logout failed.", "red");
        }
    };

    return <form onSubmit={handleSubmit(openSelectVersionModal)}>
        <SelectVersionForm isOpen={isModalOpen} releases={releases} review={reviewDependencies} close={() => setIsModalOpen(false)}/>
        <DependencyInstallForm
            plan={dependencyPlan}
            isOpen={isDependencyModalOpen}
            close={() => setIsDependencyModalOpen(false)}
            onToggleOptional={toggleOptionalDependency}
            onInstall={installPlan}
            isPlanning={isPlanningDependencies}
            isInstalling={isInstallingPlan}
        />
        <div className="relative mb-4">
            <Label text={`Search compatible Factorio ${factorioVersion || "server"} mods`} htmlFor="mod"/>
            {fuse
                ? <div className="relative">
                    <FontAwesomeIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-light" icon={faMagnifyingGlass}/>
                    <Input className="pl-10" register={register("mod", {required: true})} placeholder="Search by title or internal name…" hasAutoComplete={false} onKeyDown={handleKeyDown}/>
                </div>
                : <div className="ui-input flex items-center gap-2 text-gray-light"><FontAwesomeIcon icon={faSpinner} spin/> Loading the portal index…</div>}
            <Error error={errors.mod} message="Choose a mod from the search results."/>
            {suggestedMods.length > 0 && <div className="ui-search-results">
                {suggestedMods.map((result, index) => <button
                    type="button"
                    className={"ui-search-result" + (hoveredMod === index ? " is-active" : "")}
                    onMouseEnter={() => setHoveredMod(index)}
                    onClick={() => selectMod(result)}
                    key={result.item.name}
                >
                    <span className="block text-sm font-bold text-white">{result.item.title}</span>
                    <span className="block mt-1 text-xs text-gray-light">{result.item.name}</span>
                </button>)}
            </div>}
        </div>
        <div className="flex flex-wrap gap-2">
            <Button isDisabled={!selectedMod} isLoading={isLoadingVersions} isSubmit><FontAwesomeIcon icon={faMagnifyingGlass}/> Choose version</Button>
            <Button onClick={logout} type="secondary"><FontAwesomeIcon icon={faArrowRightFromBracket}/> Disconnect portal account</Button>
            <ButtonLink href="https://mods.factorio.com" target="_blank" type="ghost"><FontAwesomeIcon icon={faExternalLinkAlt}/> Open mod portal</ButtonLink>
        </div>
    </form>;
};

export default AddModForm;
