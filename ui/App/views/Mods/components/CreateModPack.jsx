import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faPlus} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../api/resources/mods";
import Button from "../../../components/Button";
import Modal from "../../../components/Modal";
import Label from "../../../components/Label";
import Input from "../../../components/Input";
import Error from "../../../components/Error";

const CreateModPack = ({onSuccess}) => {
    const [isCreating, setIsCreating] = useState(false);
    const [isOpen, setIsOpen] = useState(false);
    const {handleSubmit, register, reset, formState: {errors}} = useForm();

    const createModPack = async data => {
        setIsCreating(true);
        try {
            await modsResource.packs.create(data.name);
            await onSuccess();
            reset();
            setIsOpen(false);
            window.flash(`${data.name} created.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "Mod pack could not be created.", "red");
        } finally {
            setIsCreating(false);
        }
    };

    return <>
        <Button size="sm" onClick={() => setIsOpen(true)}><FontAwesomeIcon icon={faPlus}/> Save current mod set</Button>
        <Modal
            title="Create mod pack"
            isOpen={isOpen}
            content={<form id="create-mod-pack" onSubmit={handleSubmit(createModPack)}>
                <Label text="Pack name" htmlFor="name"/>
                <Input register={register("name", {required: true, pattern: /^[^\\/:*?\"<>|]+$/})} placeholder="Space Age co-op"/>
                <Error error={errors.name} message="Enter a valid mod pack name."/>
            </form>}
            actions={<>
                <Button onClick={() => setIsOpen(false)} size="sm" type="secondary" isDisabled={isCreating}>Cancel</Button>
                <Button form="create-mod-pack" size="sm" isLoading={isCreating} isSubmit>Create pack</Button>
            </>}
        />
    </>;
};

export default CreateModPack;
