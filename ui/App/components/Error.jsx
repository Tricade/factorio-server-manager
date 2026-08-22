import React from "react"

const Error = ({error, message}) => {
    if (error) {
        return (
            <span className="block mt-1 text-sm text-red" role="alert">
                {message}
            </span>
        )
    }
    return null
}

export default Error
