import React, {useState} from "react";
import {useLocation, useNavigate} from "react-router-dom";
import {useForm} from "react-hook-form";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faLock} from "@fortawesome/free-solid-svg-icons";
import user from "../../api/resources/user";
import Button from "../components/Button";
import Panel from "../components/Panel";
import Input from "../components/Input";
import InputPassword from "../components/InputPassword";
import Label from "../components/Label";
import Error from "../components/Error";
import BrandMark from "../components/BrandMark";

const Login = ({handleLogin, isChecking = false}) => {
    const {register, handleSubmit, formState: {errors}} = useForm();
    const [isSubmitting, setIsSubmitting] = useState(false);
    const navigate = useNavigate();
    const location = useLocation();

    const onSubmit = async data => {
        setIsSubmitting(true);
        try {
            const loginAttempt = await user.login(data);
            if (loginAttempt?.username) {
                await handleLogin(loginAttempt);
                navigate(location.state?.from || "/", {replace: true});
            }
        } catch (error) {
            window.flash("Login failed. Check username and password.", "red");
        } finally {
            setIsSubmitting(false);
        }
    };

    return <div className="ui-login-shell">
        <div className="ui-login-panel">
            <div className="mb-6 text-center">
                <BrandMark className="ui-login-brand mx-auto mb-4"/>
                <h1 className="text-2xl font-bold tracking-tight">Factorio Server Control</h1>
            </div>
            <Panel content={<form onSubmit={handleSubmit(onSubmit)}>
                <div className="mb-4">
                    <Label text="Username" htmlFor="username"/>
                    <Input register={register("username", {required: true})} placeholder="admin" hasAutoComplete autoFocus/>
                    <Error error={errors.username} message="Username is required."/>
                </div>
                <div className="mb-5">
                    <Label text="Password" htmlFor="password"/>
                    <InputPassword register={register("password", {required: true})}/>
                    <Error error={errors.password} message="Password is required."/>
                </div>
                <Button type="default" className="w-full" isSubmit isLoading={isSubmitting || isChecking}>
                    <FontAwesomeIcon icon={faLock}/> Sign in
                </Button>
            </form>}/>
        </div>
    </div>;
};

export default Login;
