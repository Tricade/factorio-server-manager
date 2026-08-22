import React, {useEffect, useRef, useState} from "react";
import {useForm} from "react-hook-form";
import {useNavigate, useSearchParams} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faBoxArchive, faCheck, faClone, faFloppyDisk, faLayerGroup,
    faPen, faPlay, faPlus, faPuzzlePiece, faTrash, faXmark
} from "@fortawesome/free-solid-svg-icons";
import profilesResource from "../../api/resources/profiles";
import PageHeader from "../components/PageHeader";
import Panel from "../components/Panel";
import Button from "../components/Button";
import Input from "../components/Input";
import Select from "../components/Select";
import Label from "../components/Label";
import Alert from "../components/Alert";
import EmptyState from "../components/EmptyState";
import ConfirmDialog from "../components/ConfirmDialog";
import {useProfiles} from "../context/ProfileContext";

const sourceOptions = [
    {value: "empty", name: "New world (no save)"},
    {value: "clone", name: "Copy the active profile"}
];

const profileError = (error, fallback) => error?.response?.data?.error || error?.response?.data || fallback;

const Profiles = ({serverStatus, refreshServerStatus}) => {
    const {state, activeProfile, isLoading, applyProfileState, refreshProfiles} = useProfiles();
    const [searchParams, setSearchParams] = useSearchParams();
    const navigate = useNavigate();
    const requestedFreshProfile = searchParams.get("new") === "fresh";
    const [showCreate, setShowCreate] = useState(requestedFreshProfile);
    const [pendingProfile, setPendingProfile] = useState(null);
    const [creating, setCreating] = useState(false);
    const [activating, setActivating] = useState(null);
    const [editing, setEditing] = useState(null);
    const [savingEdit, setSavingEdit] = useState(false);
    const [deleting, setDeleting] = useState(null);
    const refreshOnMount = useRef(Boolean(state));
    const {register, handleSubmit, reset, watch} = useForm({
        defaultValues: {name: "", description: "", source: "empty"}
    });
    const locked = Boolean(serverStatus?.running || serverStatus?.stopping);
    const profileUIBusy = locked || creating || isLoading || !activeProfile;
    const source = watch("source");

    useEffect(() => {
        if (refreshOnMount.current) refreshProfiles().catch(() => undefined);
    }, [refreshProfiles]);

    const openCreate = () => {
        reset({name: "", description: "", source: "empty"});
        setPendingProfile(null);
        setShowCreate(true);
    };

    const closeCreate = () => {
        setShowCreate(false);
        if (requestedFreshProfile) setSearchParams({}, {replace: true});
    };

    const create = async values => {
        setCreating(true);
        try {
            const existingIDs = new Set((state?.profiles || []).map(profile => profile.id));
            const result = applyProfileState(await profilesResource.create({
                name: values.name.trim(),
                description: values.description.trim(),
                source: values.source
            }));
            const createdProfile = result.profiles.find(profile => !existingIDs.has(profile.id) && profile.name === values.name.trim())
                || result.profiles.find(profile => profile.name === values.name.trim());
            setPendingProfile(createdProfile ? {...createdProfile, source: values.source} : null);
            setShowCreate(false);
            setSearchParams({}, {replace: true});
            reset({name: "", description: "", source: "empty"});
            window.flash("Profile created. Activate it when you are ready to switch setups.", "green");
        } catch (error) {
            window.flash(profileError(error, "Profile could not be created."), "red");
        } finally {
            setCreating(false);
        }
    };

    const activate = async (profile, continueSetup = false) => {
        setActivating(profile.id);
        try {
            applyProfileState(await profilesResource.activate(profile.id));
            await refreshServerStatus();
            setEditing(null);
            setPendingProfile(null);
            window.flash(profile.name + " is active. Factorio remains stopped until you start it.", "green");
            if (continueSetup) navigate(profile.save_count === 0 ? "/saves" : "/");
        } catch (error) {
            window.flash(profileError(error, profile.name + " could not be activated."), "red");
        } finally {
            setActivating(null);
        }
    };

    const beginEdit = profile => setEditing({
        id: profile.id,
        name: profile.name,
        description: profile.description || ""
    });

    const saveEdit = async event => {
        event.preventDefault();
        if (!editing) return;
        setSavingEdit(true);
        try {
            applyProfileState(await profilesResource.update(editing.id, {
                name: editing.name.trim(),
                description: editing.description.trim()
            }));
            setEditing(null);
            window.flash("Profile details saved.", "green");
        } catch (error) {
            window.flash(profileError(error, "Profile details could not be saved."), "red");
        } finally {
            setSavingEdit(false);
        }
    };

    const remove = async () => {
        if (!deleting) return;
        applyProfileState(await profilesResource.remove(deleting.id));
        window.flash(deleting.name + " was deleted.", "green");
        setDeleting(null);
    };

    return <>
        <PageHeader
            title="Profiles"
            help="Profiles keep saves, versions, modes, mods and settings separate. Activating one snapshots the current setup and leaves Factorio stopped."
            actions={<Button onClick={openCreate} isDisabled={profileUIBusy}>
                <FontAwesomeIcon icon={faPlus}/> New profile
            </Button>}
        />

        {locked && <Alert type="warning" className="mb-5">
            Save and stop Factorio before creating a snapshot or switching profiles.
        </Alert>}

        {pendingProfile && <Panel
            title={`${pendingProfile.name} is ready`}
            description={pendingProfile.source === "empty"
                ? "This profile has no save yet. Activate it to open the new-world generator."
                : "This copy remains stopped and separate until you activate it."}
            className="mb-5"
            content={<div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <ProfileFact label="Starting point" value={pendingProfile.source === "empty" ? "No save" : "Copied setup"}/>
                <ProfileFact label="Mode" value={pendingProfile.game_mode === "space-age" ? "Space Age" : "Factorio"}/>
                <ProfileFact label="Version" value={pendingProfile.installed_version || "Current runtime"}/>
            </div>}
            actions={<>
                <Button type="ghost" onClick={() => setPendingProfile(null)}>Dismiss</Button>
                <Button type="success" isLoading={activating === pendingProfile.id} isDisabled={locked || Boolean(activating)} onClick={() => activate(pendingProfile, true)}>
                    <FontAwesomeIcon icon={faPlay}/> {pendingProfile.source === "empty" ? "Activate & create world" : "Activate profile"}
                </Button>
            </>}
        />}

        {showCreate && <Panel
            title="Create profile"
            className="mb-5"
            content={<form id="create-profile-form" onSubmit={handleSubmit(create)} className="grid grid-cols-1 lg:grid-cols-3 gap-4">
                <div>
                    <Label text="Name" htmlFor="profile-name"/>
                    <Input
                        id="profile-name"
                        disabled={profileUIBusy}
                        register={register("name", {required: true, maxLength: 64})}
                        placeholder="Space Age megabase"
                    />
                </div>
                <div>
                    <Label text="Starting point" htmlFor="profile-source" help={source === "clone"
                        ? `Copies saves, installed mods, settings and runtime state from ${activeProfile?.name || "the active profile"}. The copy starts stopped.`
                        : "Creates a stopped base-game profile without saves or downloaded mods. Activate it to create or upload its first world."}/>
                    <Select
                        id="profile-source"
                        disabled={profileUIBusy}
                        register={register("source")}
                        options={sourceOptions}
                    />
                </div>
                <div>
                    <Label text="Description" htmlFor="profile-description"/>
                    <Input
                        id="profile-description"
                        disabled={profileUIBusy}
                        register={register("description", {maxLength: 500})}
                        placeholder="Optional note"
                    />
                </div>
            </form>}
            actions={<>
                <Button type="ghost" isDisabled={creating} onClick={closeCreate}><FontAwesomeIcon icon={faXmark}/> Cancel</Button>
                <Button type="success" form="create-profile-form" isSubmit isLoading={creating} isDisabled={profileUIBusy}>
                    <FontAwesomeIcon icon={faPlus}/> Create profile
                </Button>
            </>}
        />}

        <Panel
            content={isLoading
                ? <div className="py-8 text-center text-gray-light">Loading profiles…</div>
                : !state?.profiles?.length
                    ? <EmptyState title="No profiles found"/>
                    : <div className="ui-profile-list">
                        {state.profiles.map(profile => {
                            const isEditing = editing?.id === profile.id;
                            return <article
                                key={profile.id}
                                className={"ui-profile-card" + (profile.active ? " is-active" : "")}
                            >
                                {isEditing ? <form onSubmit={saveEdit}>
                                    <label className="mb-2 block text-sm font-bold text-white" htmlFor={"edit-name-" + profile.id}>Name</label>
                                    <input
                                        id={"edit-name-" + profile.id}
                                        className="ui-input"
                                        value={editing.name}
                                        maxLength={64}
                                        autoFocus
                                        disabled={savingEdit}
                                        onChange={event => setEditing(current => ({...current, name: event.target.value}))}
                                    />
                                    <label className="mb-2 mt-4 block text-sm font-bold text-white" htmlFor={"edit-description-" + profile.id}>Description</label>
                                    <textarea
                                        id={"edit-description-" + profile.id}
                                        className="ui-input min-h-24"
                                        value={editing.description}
                                        maxLength={500}
                                        disabled={savingEdit}
                                        onChange={event => setEditing(current => ({...current, description: event.target.value}))}
                                    />
                                    <div className="mt-4 flex justify-end gap-2">
                                        <Button size="sm" type="secondary" isDisabled={savingEdit} onClick={() => setEditing(null)}>
                                            <FontAwesomeIcon icon={faXmark}/> Cancel
                                        </Button>
                                        <Button size="sm" type="success" isSubmit isLoading={savingEdit} isDisabled={!editing.name.trim()}>
                                            <FontAwesomeIcon icon={faCheck}/> Save
                                        </Button>
                                    </div>
                                </form> : <>
                                    <div className="ui-profile-card__header">
                                        <div className={"ui-profile-card__icon" + (profile.active ? " is-active" : "")}>
                                            <FontAwesomeIcon icon={profile.active ? faLayerGroup : faBoxArchive}/>
                                        </div>
                                        <div className="ui-profile-card__identity">
                                            <div className="flex flex-wrap items-center gap-2">
                                                <h3 className="font-bold text-white">{profile.name}</h3>
                                                {profile.active && <span className="ui-status-badge ui-status-badge--running">Active</span>}
                                            </div>
                                            {profile.description && <p>{profile.description}</p>}
                                        </div>
                                    </div>
                                    <div className="ui-profile-card__facts">
                                        <ProfileFact label="Version" value={profile.installed_version || "Unknown"}/>
                                        <ProfileFact
                                            label="Mode"
                                            value={profile.game_mode === "space-age"
                                                ? "Space Age"
                                                : profile.game_mode === "factorio" ? "Factorio" : "Custom"}
                                        />
                                        <ProfileFact label="Saves" value={profile.save_count} icon={faFloppyDisk}/>
                                        <ProfileFact label="Mods" value={profile.mod_count} icon={faPuzzlePiece}/>
                                    </div>
                                    {profile.selected_save && <p
                                        className="ui-profile-card__save"
                                        title={profile.selected_save}
                                    >
                                        Save: {profile.selected_save}
                                    </p>}
                                    <div className="ui-profile-card__actions">
                                        <Button size="sm" type="ghost" isDisabled={Boolean(activating)} onClick={() => beginEdit(profile)}>
                                            <FontAwesomeIcon icon={faPen}/> Edit
                                        </Button>
                                        {!profile.active && <Button
                                            size="sm"
                                            type="danger"
                                            isDisabled={Boolean(activating)}
                                            onClick={() => setDeleting(profile)}
                                        >
                                            <FontAwesomeIcon icon={faTrash}/> Delete
                                        </Button>}
                                        {!profile.active && <Button
                                            size="sm"
                                            type="success"
                                            isLoading={activating === profile.id}
                                            isDisabled={locked || Boolean(activating)}
                                            onClick={() => activate(profile)}
                                        >
                                            <FontAwesomeIcon icon={faPlay}/> Activate
                                        </Button>}
                                    </div>
                                </>}
                            </article>;
                        })}
                    </div>}
        />

        <ConfirmDialog
            title={"Delete " + (deleting?.name || "profile") + "?"}
            content="The inactive profile archive, including all saves and its private mod set, will be permanently deleted. The active setup is not changed."
            isOpen={Boolean(deleting)}
            close={() => setDeleting(null)}
            onSuccess={remove}
        />
    </>;
};

const ProfileFact = ({label, value, icon = faClone}) => <div className="ui-profile-fact">
    <p><FontAwesomeIcon icon={icon}/>{label}</p>
    <strong title={String(value)}>{value}</strong>
</div>;

export default Profiles;
