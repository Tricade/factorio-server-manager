import React, {useCallback, useEffect, useState} from "react";
import {BrowserRouter, Navigate, Outlet, Route, Routes} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faGear} from "@fortawesome/free-solid-svg-icons";
import user from "../api/resources/user";
import server from "../api/resources/server";
import socket from "../api/socket";
import Layout from "./components/Layout";
import Login from "./views/Login";
import Controls from "./views/Controls";
import Logs from "./views/Logs";
import Saves from "./views/Saves/Saves";
import Mods from "./views/Mods/Mods";
import UserManagement from "./views/UserManagement/UserManagment";
import ServerSettings from "./views/ServerSettings";
import GameSettings from "./views/GameSettings";
import Console from "./views/Console";
import Releases from "./views/Releases";
import Profiles from "./views/Profiles";
import {Flash} from "./components/Flash";
import {ProfileProvider} from "./context/ProfileContext";

const AppLoader = () => <div className="ui-login-shell">
    <div className="text-center">
        <div className="ui-brand-mark mx-auto mb-4"><FontAwesomeIcon icon={faGear} spin/></div>
        <p className="font-bold">Connecting to server manager</p>
    </div>
</div>;

const ProtectedRoute = ({authState}) => {
    if (authState === "checking") return <AppLoader/>;
    if (authState !== "authenticated") return <Navigate to="/login" replace state={{from: window.location.pathname}}/>;
    return <Outlet/>;
};

const App = () => {
    const [authState, setAuthState] = useState("checking");
    const [currentUser, setCurrentUser] = useState(null);
    const [serverStatus, setServerStatus] = useState({running: false, stopping: false});
    const [socketState, setSocketState] = useState("disconnected");

    const refreshServerStatus = useCallback(async () => {
        const currentServerStatus = await server.status();
        setServerStatus(currentServerStatus);
        return currentServerStatus;
    }, []);

    const handleAuthenticated = useCallback(async authenticatedUser => {
        setCurrentUser(authenticatedUser);
        setAuthState("authenticated");
        try {
            await refreshServerStatus();
        } catch (error) {
            window.flash("Logged in, but the server status could not be loaded.", "red");
        }
    }, [refreshServerStatus]);

    useEffect(() => {
        let active = true;
        user.status()
            .then(status => active && handleAuthenticated(status))
            .catch(() => active && setAuthState("anonymous"));
        return () => { active = false; };
    }, [handleAuthenticated]);

    useEffect(() => {
        if (authState !== "authenticated") {
            socket.disconnect();
            return undefined;
        }

        const receiveStatus = status => {
            try {
                setServerStatus(typeof status === "string" ? JSON.parse(status) : status);
            } catch (error) {
                window.flash("Received an invalid server status update.", "red");
            }
        };
        const receiveConnectionState = state => setSocketState(state);

        socket.on("server_status", receiveStatus);
        socket.on("connection_state", receiveConnectionState);
        socket.connect();
        socket.emit("server status subscribe");

        return () => {
            socket.off("server_status", receiveStatus);
            socket.off("connection_state", receiveConnectionState);
            socket.disconnect();
        };
    }, [authState]);

    const handleLogout = useCallback(async () => {
        try {
            await user.logout();
        } finally {
            socket.disconnect();
            setCurrentUser(null);
            setAuthState("anonymous");
            setServerStatus({running: false, stopping: false});
        }
    }, []);

    return <BrowserRouter>
        <Routes>
            <Route path="login" element={
                authState === "authenticated"
                    ? <Navigate to="/" replace/>
                    : <Login handleLogin={handleAuthenticated} isChecking={authState === "checking"}/>
            }/>
            <Route element={<ProtectedRoute authState={authState}/> }>
                <Route element={<ProfileProvider><Layout handleLogout={handleLogout} serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} currentUser={currentUser} socketState={socketState}/></ProfileProvider>}>
                    <Route index element={<Controls serverStatus={serverStatus} refreshServerStatus={refreshServerStatus}/>}/>
                    <Route path="saves" element={<Saves serverStatus={serverStatus}/>}/>
                    <Route path="mods" element={<Mods serverStatus={serverStatus}/>}/>
                    <Route path="server-settings" element={<ServerSettings serverStatus={serverStatus}/>}/>
                    <Route path="game-settings" element={<GameSettings serverStatus={serverStatus}/>}/>
                    <Route path="console" element={<Console serverStatus={serverStatus} socketState={socketState}/>}/>
                    <Route path="logs" element={<Logs/>}/>
                    <Route path="releases" element={<Releases serverStatus={serverStatus} refreshServerStatus={refreshServerStatus}/>}/>
                    <Route path="profiles" element={<Profiles serverStatus={serverStatus} refreshServerStatus={refreshServerStatus}/>}/>
                    <Route path="user-management" element={<UserManagement currentUser={currentUser}/>}/>
                </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace/>}/>
        </Routes>
        <Flash/>
    </BrowserRouter>;
};

export default App;
