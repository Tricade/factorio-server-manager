import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faKey} from "@fortawesome/free-solid-svg-icons";
import user from "../../../../api/resources/user";
import Button from "../../../components/Button";
import Label from "../../../components/Label";
import InputPassword from "../../../components/InputPassword";
import Error from "../../../components/Error";

const ChangePasswordForm = () => {
    const {register, handleSubmit, reset, formState: {errors}, watch} = useForm();
    const [isSaving, setIsSaving] = useState(false);
    const newPassword = watch("new_password");

    const onSubmit = async data => {
        setIsSaving(true);
        try {
            await user.changePassword(data);
            window.flash("Password changed.", "green");
            reset();
        } catch (error) {
            window.flash(error?.response?.data || "Password could not be changed.", "red");
        } finally {
            setIsSaving(false);
        }
    };

    return <form onSubmit={handleSubmit(onSubmit)}>
        <div className="mb-4"><Label htmlFor="old_password" text="Current password"/><InputPassword register={register("old_password", {required: true})}/><Error error={errors.old_password} message="Current password is required."/></div>
        <div className="mb-4"><Label htmlFor="new_password" text="New password"/><InputPassword register={register("new_password", {required: true, minLength: 10})}/><Error error={errors.new_password} message="Use at least 10 characters."/></div>
        <div className="mb-4"><Label htmlFor="new_password_confirmation" text="Confirm new password"/><InputPassword register={register("new_password_confirmation", {required: true, validate: value => value === newPassword})}/><Error error={errors.new_password_confirmation} message="Passwords must match."/></div>
        <Button isSubmit isLoading={isSaving} type="success"><FontAwesomeIcon icon={faKey}/> Change password</Button>
    </form>;
};

export default ChangePasswordForm;
