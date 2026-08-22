import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCloudArrowUp} from "@fortawesome/free-solid-svg-icons";
import saves from "../../../../api/resources/saves";
import Button from "../../../components/Button";
import Error from "../../../components/Error";
import Label from "../../../components/Label";

const UploadSaveForm = ({onSuccess}) => {
    const {register, handleSubmit, reset, formState: {errors}} = useForm();
    const [fileName, setFileName] = useState("Choose a Factorio save…");
    const [isUploading, setIsUploading] = useState(false);
    const savefileRegistration = register("savefile", {required: true});

    const onSubmit = async data => {
        setIsUploading(true);
        try {
            await saves.upload(data.savefile[0]);
            reset();
            setFileName("Choose a Factorio save…");
            await onSuccess();
            window.flash("Save uploaded.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Save could not be uploaded.", "red");
        } finally {
            setIsUploading(false);
        }
    };

    return <form onSubmit={handleSubmit(onSubmit)}>
        <div className="mb-4">
            <Label text="Save archive" htmlFor="upload-savefile"/>
            <label className="ui-file-input px-4" htmlFor="upload-savefile">
                <FontAwesomeIcon className="text-orange" icon={faCloudArrowUp}/>
                <span className="truncate">{fileName}</span>
                <input
                    id="upload-savefile"
                    className="absolute inset-0 opacity-0 cursor-pointer"
                    {...savefileRegistration}
                    onChange={event => {
                        savefileRegistration.onChange(event);
                        setFileName(event.currentTarget.files?.[0]?.name || "Choose a Factorio save…");
                    }}
                    accept=".zip,application/zip"
                    type="file"
                />
            </label>
            <Error error={errors.savefile} message="Choose a .zip save file."/>
        </div>
        <Button type="success" isLoading={isUploading} isSubmit><FontAwesomeIcon icon={faCloudArrowUp}/> Upload save</Button>
    </form>;
};

export default UploadSaveForm;
