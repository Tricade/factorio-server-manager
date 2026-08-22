import Input from "./Input";
import React, {useState} from "react";
import {faEye, faEyeSlash} from "@fortawesome/free-solid-svg-icons";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";

const InputPassword = ({register, defaultValue}) => {

    const [type, setType] = useState("password");

    let icon;
    if (type === "password") {
        icon = faEye;
    } else {
        icon = faEyeSlash
    }

    return (
        <div className="relative">
            <Input type={type} defaultValue={defaultValue} register={register} placeholder="*************"/>
            <button
                type="button"
                className="absolute inset-y-0 right-0 px-3 text-gray-light hover:text-white"
                onClick={() => setType(type === "password" ? "text" : "password")}
                aria-label={type === "password" ? "Show password" : "Hide password"}
            >
                <FontAwesomeIcon fixedWidth={true} icon={icon} />
            </button>
        </div>
    )
}
export default InputPassword;
