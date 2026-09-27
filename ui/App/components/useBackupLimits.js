import {useEffect, useState} from "react";
import backups from "../../api/resources/backups";

export default function useBackupLimits(open) {
    const [limits, setLimits] = useState(null);
    const [loading, setLoading] = useState(true);
    const [failed, setFailed] = useState(false);
    const [revision, setRevision] = useState(0);
    useEffect(() => {
        if (!open) return undefined;
        let current = true;
        setLimits(null); setLoading(true); setFailed(false);
        backups.limits().then(value => { if (current) setLimits(value); })
            .catch(() => { if (current) setFailed(true); })
            .finally(() => { if (current) setLoading(false); });
        return () => { current = false; };
    }, [open, revision]);
    return {limits, setLimits, loading, failed, retry: () => setRevision(value => value + 1)};
}
