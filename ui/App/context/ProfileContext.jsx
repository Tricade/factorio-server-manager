import React, {createContext, useCallback, useContext, useEffect, useMemo, useRef, useState} from "react";
import profilesResource from "../../api/resources/profiles";

const ProfileContext = createContext(null);

const errorText = error => {
    const value = error?.response?.data?.error || error?.response?.data || error?.message;
    return typeof value === "string" && value.trim() ? value.trim() : "Profiles could not be loaded.";
};

const validateState = result => {
    if (!result || !Array.isArray(result.profiles) || !result.active_profile_id) {
        throw new Error("The profile API returned an invalid response.");
    }
    return result;
};

export const ProfileProvider = ({children}) => {
    const [state, setState] = useState(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState("");
    const hasLoaded = useRef(false);

    const applyProfileState = useCallback(result => {
        const validated = validateState(result);
        setState(validated);
        setError("");
        return validated;
    }, []);

    const refreshProfiles = useCallback(async () => {
        if (!hasLoaded.current) setIsLoading(true);
        try {
            return applyProfileState(await profilesResource.list());
        } catch (requestError) {
            setError(errorText(requestError));
            throw requestError;
        } finally {
            hasLoaded.current = true;
            setIsLoading(false);
        }
    }, [applyProfileState]);

    useEffect(() => {
        refreshProfiles().catch(() => undefined);
    }, [refreshProfiles]);

    const activeProfile = useMemo(
        () => state?.profiles?.find(profile => profile.id === state.active_profile_id) || null,
        [state]
    );

    const value = useMemo(() => ({
        state,
        activeProfile,
        isLoading,
        error,
        applyProfileState,
        refreshProfiles
    }), [state, activeProfile, isLoading, error, applyProfileState, refreshProfiles]);

    return <ProfileContext.Provider value={value}>{children}</ProfileContext.Provider>;
};

export const useProfiles = () => {
    const context = useContext(ProfileContext);
    if (!context) throw new Error("useProfiles must be used inside ProfileProvider.");
    return context;
};
