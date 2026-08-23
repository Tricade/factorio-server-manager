import React, {useEffect, useState} from "react";
import AddModForm from "./components/AddModForm";
import FactorioLogin from "./components/FactorioLogin";
import modResource from "../../../../../api/resources/mods";
import Alert from "../../../../components/Alert";
import Button from "../../../../components/Button";

const AddMod = ({refetchInstalledMods, fuse, factorioVersion, portalError, retryPortal}) => {

    const [isFactorioAuthenticated, setIsFactorioAuthenticated] = useState(false);
    const [isLoading, setIsLoading] = useState(true);
    const [loadError, setLoadError] = useState("");

    const loadStatus = async () => {
        setIsLoading(true);
        setLoadError("");
        try {
            setIsFactorioAuthenticated(Boolean(await modResource.portal.status()));
        } catch (error) {
            setLoadError("The Factorio mod portal connection status could not be loaded.");
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => { loadStatus(); }, []);

    if (isLoading) return <p className="text-sm text-gray-light">Checking the Factorio mod portal connection…</p>;

    if (portalError) return <div>
        <Alert type="warning" className="mb-4">{portalError}</Alert>
        <Button type="secondary" size="sm" onClick={retryPortal}>Retry</Button>
    </div>;

    if (loadError) return <div>
        <Alert type="danger" className="mb-4">{loadError}</Alert>
        <Button type="secondary" size="sm" onClick={loadStatus}>Retry</Button>
    </div>;

    return isFactorioAuthenticated
        ? <AddModForm fuse={fuse} factorioVersion={factorioVersion} setIsFactorioAuthenticated={setIsFactorioAuthenticated} refetchInstalledMods={refetchInstalledMods}/>
        : <FactorioLogin setIsFactorioAuthenticated={setIsFactorioAuthenticated}/>
};

export default AddMod;
