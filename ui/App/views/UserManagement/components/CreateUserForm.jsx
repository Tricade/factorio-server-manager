import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faUserPlus} from "@fortawesome/free-solid-svg-icons";
import user from "../../../../api/resources/user";
import Button from "../../../components/Button";
import Label from "../../../components/Label";
import Input from "../../../components/Input";
import InputPassword from "../../../components/InputPassword";
import Error from "../../../components/Error";
import Select from "../../../components/Select";

const CreateUserForm = ({updateUserList}) => {
    const [isSaving, setIsSaving] = useState(false);
    const {register, handleSubmit, reset, formState: {errors}, watch} = useForm({defaultValues: {role: "viewer"}});
    const password = watch("password");
    const role = watch("role");

    const onSubmit = async data => {
        setIsSaving(true);
        try {
            await user.add(data);
            await updateUserList();
            reset();
            window.flash(`${data.username} created.`, "green");
        } catch (error) {
            window.flash(error?.response?.data || "User could not be created.", "red");
        } finally {
            setIsSaving(false);
        }
    };

    return <form onSubmit={handleSubmit(onSubmit)}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-4">
            <div><Label htmlFor="username" text="Username"/><Input register={register("username", {required: true, pattern: /^[A-Za-z0-9_.-]+$/})} placeholder="operator"/><Error error={errors.username} message="Use letters, digits, dots, dashes or underscores."/></div>
            <div><Label htmlFor="email" text="Email"/><Input register={register("email")} type="email" placeholder="optional@example.com"/><Error error={errors.email} message="Enter a valid email address."/></div>
            <div><Label htmlFor="role" text="Role"/><Select register={register("role", {required: true})} options={[{value: "viewer", name: "Viewer (read-only)"}, {value: "admin", name: "Administrator"}]}/></div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
            <div><Label htmlFor="password" text="Password"/><InputPassword register={register("password", {required: true, minLength: 10})}/><Error error={errors.password} message="Use at least 10 characters."/></div>
            <div><Label htmlFor="password_confirmation" text="Confirm password"/><InputPassword register={register("password_confirmation", {required: true, validate: value => value === password})}/><Error error={errors.password_confirmation} message="Passwords must match."/></div>
        </div>
        <Button isSubmit isLoading={isSaving} type="success"><FontAwesomeIcon icon={faUserPlus}/> Create {role === "admin" ? "administrator" : "viewer"}</Button>
    </form>;
};

export default CreateUserForm;
