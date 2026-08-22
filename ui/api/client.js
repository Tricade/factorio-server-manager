import Axios from "axios";

const client = Axios.create({
    withCredentials: true,
    headers: {
        'Content-Type': 'application/json'
    }
});

client.interceptors.response.use(res => res, err => {
    const status = err.response?.status;
    if (!err.response) {
        window.flash("The server is not reachable. Check the manager connection.", "red");
    } else if(status === 502) {
        window.flash("Service not available", "red");
    } else if (status !== 401) {
        const message = typeof err.response.data === "string"
            ? err.response.data
            : err.response.data?.message || "The request could not be completed.";
        window.flash(message, "red");
    }
    return Promise.reject(err);
});

export default client;
