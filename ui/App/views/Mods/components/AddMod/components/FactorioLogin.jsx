import React, {useState} from "react";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faLink} from "@fortawesome/free-solid-svg-icons";
import modsResource from "../../../../../../api/resources/mods";
import Input from "../../../../../components/Input";
import InputPassword from "../../../../../components/InputPassword";
import Label from "../../../../../components/Label";
import Button from "../../../../../components/Button";
import Error from "../../../../../components/Error";

const FactorioLogin = ({setIsFactorioAuthenticated}) => {
    const {register, handleSubmit, formState: {errors}} = useForm();
    const [isLoading, setIsLoading] = useState(false);

    const login = async ({username, password}) => {
        setIsLoading(true);
        try {
            await modsResource.portal.login(username, password);
            setIsFactorioAuthenticated(true);
            window.flash("Factorio mod portal connected.", "green");
        } catch (error) {
            window.flash("Factorio username/email and password did not match.", "red");
        } finally {
            setIsLoading(false);
        }
    };

    return <form onSubmit={handleSubmit(login)}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
            <div>
                <Label text="Factorio username or email" htmlFor="username" help="A Factorio account is required only for downloading portal mods. Credentials stay on this manager instance."/>
                <Input register={register("username", {required: true})} hasAutoComplete/>
                <Error error={errors.username} message="Username or email is required."/>
            </div>
            <div>
                <Label text="Factorio password" htmlFor="password"/>
                <InputPassword register={register("password", {required: true})}/>
                <Error error={errors.password} message="Password is required."/>
            </div>
        </div>
        <Button isSubmit isLoading={isLoading}><FontAwesomeIcon icon={faLink}/> Connect account</Button>
    </form>;
};

export default FactorioLogin;
