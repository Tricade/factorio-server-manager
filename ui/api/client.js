import Axios from "axios";

export const authenticationRequiredEvent = "fsm:authentication-required";

const client = Axios.create({
    withCredentials: true,
    headers: {
        'Content-Type': 'application/json'
    }
});

client.interceptors.response.use(res => res, err => {
    const status = err.response?.status;
    if (!err.response) {
        const message = err.config?.networkErrorMessage || "The control service is not reachable. Check the connection.";
        err.userMessage = message;
        window.flash(message, "red");
    } else if (status === 401) {
        window.dispatchEvent(new Event(authenticationRequiredEvent));
    } else if(status === 502) {
        window.flash("Service not available", "red");
    } else {
        const message = typeof err.response.data === "string"
            ? err.response.data
            : err.response.data?.error || err.response.data?.message || "The request could not be completed.";
        window.flash(message, "red");
    }
    return Promise.reject(err);
});

export default client;
