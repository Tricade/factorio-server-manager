import regeneratorRuntime from "regenerator-runtime"
import Bus from "./notifications"
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App/App.jsx';

window.__FSM_UI_BUILD__ = {version: __FSM_UI_VERSION__, revision: __FSM_UI_REVISION__};
window.flash = (message, color="gray-light") => {
    const candidate = typeof message === "string"
        ? message
        : typeof message?.error === "string"
            ? message.error
            : typeof message?.message === "string" ? message.message : "The request could not be completed.";
    Bus.emit('flash', {message: candidate.trim() || "The request could not be completed.", color});
};

const root = ReactDOM.createRoot(document.getElementById('app'));
root.render(<App/>);
