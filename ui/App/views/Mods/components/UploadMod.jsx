import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCloudArrowUp} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../api/resources/mods";
import Button from "../../../components/Button";
import Label from "../../../components/Label";
import Error from "../../../components/Error";

const UploadMod = ({refetchInstalledMods}) => {
    const [fileName, setFileName] = useState("Choose a mod archive…");
    const {register, handleSubmit, reset, formState: {errors}} = useForm();
    const [isUploading, setIsUploading] = useState(false);
    const registration = register("mod_file", {required: true});

    const onSubmit = async data => {
        setIsUploading(true);
        try {
            await modsResource.upload(data.mod_file[0]);
            await refetchInstalledMods();
            reset();
            setFileName("Choose a mod archive…");
            window.flash("Mod archive uploaded.", "green");
        } catch (error) {
            window.flash(error?.response?.data || "Mod could not be uploaded.", "red");
        } finally {
            setIsUploading(false);
        }
    };

    return <form onSubmit={handleSubmit(onSubmit)}>
        <div className="mb-4">
            <Label text="Factorio mod archive" htmlFor="mod_file"/>
            <label className="ui-file-input px-4" htmlFor="mod_file">
                <FontAwesomeIcon className="text-orange" icon={faCloudArrowUp}/>
                <span className="truncate">{fileName}</span>
                <input
                    {...registration}
                    className="absolute inset-0 opacity-0 cursor-pointer"
                    onChange={event => {
                        registration.onChange(event);
                        setFileName(event.currentTarget.files?.[0]?.name || "Choose a mod archive…");
                    }}
                    id="mod_file"
                    type="file"
                    accept=".zip,application/zip"
                />
            </label>
            <Error error={errors.mod_file} message="Choose a Factorio mod .zip file."/>
        </div>
        <Button isLoading={isUploading} isSubmit><FontAwesomeIcon icon={faCloudArrowUp}/> Upload mod</Button>
    </form>;
};

export default UploadMod;
