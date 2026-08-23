import React, {useCallback, useEffect, useState} from "react";
import {BrowserRouter, Navigate, Outlet, Route, Routes} from "react-router-dom";
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
import BrandMark from "./components/BrandMark";
import {authenticationRequiredEvent} from "../api/client";

const AppLoader = () => <div className="ui-login-shell">
    <div className="text-center">
        <BrandMark className="mx-auto mb-4" isLoading/>
        <p className="font-bold">Connecting to Factorio Server Control</p>
    </div>
</div>;

const ProtectedRoute = ({authState}) => {
    if (authState === "checking") return <AppLoader/>;
    if (authState !== "authenticated") return <Navigate to="/login" replace state={{from: window.location.pathname}}/>;
    return <Outlet/>;
};

const normalizeServerStatus = status => {
    if (!status || typeof status !== "object" || typeof status.running !== "boolean" || typeof status.stopping !== "boolean") {
        throw new Error("The server status endpoint returned an invalid response.");
    }
    return {...status, known: true};
};

const App = () => {
    const [authState, setAuthState] = useState("checking");
    const [currentUser, setCurrentUser] = useState(null);
    const [serverStatus, setServerStatus] = useState({running: false, stopping: false, known: false});
    const [socketState, setSocketState] = useState("disconnected");
    const canManage = currentUser?.role === "admin";

    const refreshServerStatus = useCallback(async () => {
        const currentServerStatus = normalizeServerStatus(await server.status());
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
        const requireAuthentication = () => {
            socket.disconnect();
            setCurrentUser(null);
            setAuthState("anonymous");
            setServerStatus({running: false, stopping: false, known: false});
        };
        window.addEventListener(authenticationRequiredEvent, requireAuthentication);
        return () => window.removeEventListener(authenticationRequiredEvent, requireAuthentication);
    }, []);

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
                const parsedStatus = typeof status === "string" ? JSON.parse(status) : status;
                setServerStatus(normalizeServerStatus(parsedStatus));
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
            setServerStatus({running: false, stopping: false, known: false});
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
                <Route element={<ProfileProvider><Layout handleLogout={handleLogout} serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} currentUser={currentUser} socketState={socketState} canManage={canManage}/></ProfileProvider>}>
                    <Route index element={<Controls serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} canManage={canManage}/>}/>
                    <Route path="saves" element={<Saves serverStatus={serverStatus} canManage={canManage}/>}/>
                    <Route path="mods" element={<Mods serverStatus={serverStatus} canManage={canManage}/>}/>
                    <Route path="server-settings" element={<ServerSettings serverStatus={serverStatus} canManage={canManage}/>}/>
                    <Route path="game-settings" element={<GameSettings serverStatus={serverStatus}/>}/>
                    <Route path="console" element={canManage ? <Console serverStatus={serverStatus} socketState={socketState}/> : <Navigate to="/" replace/>}/>
                    <Route path="logs" element={<Logs/>}/>
                    <Route path="releases" element={<Releases serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} canManage={canManage}/>}/>
                    <Route path="profiles" element={<Profiles serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} canManage={canManage}/>}/>
                    <Route path="user-management" element={<UserManagement currentUser={currentUser} canManage={canManage}/>}/>
                </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace/>}/>
        </Routes>
        <Flash/>
    </BrowserRouter>;
};

export default App;
