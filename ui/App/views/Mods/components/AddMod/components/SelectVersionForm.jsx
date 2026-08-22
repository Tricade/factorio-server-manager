import React, {useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCheck, faCloudArrowDown, faTimes} from "@fortawesome/free-solid-svg-icons";
import Modal from "../../../../../components/Modal";
import Button from "../../../../../components/Button";
import EmptyState from "../../../../../components/EmptyState";

const SelectVersionForm = ({releases, isOpen, close, review}) => {
    const [reviewingVersion, setReviewingVersion] = useState(null);

    const select = async release => {
        setReviewingVersion(release.version);
        try {
            if (await review(release)) close();
        } finally {
            setReviewingVersion(null);
        }
    };

    return <Modal
        isOpen={isOpen}
        title="Choose mod version"
        content={releases.length === 0
            ? <EmptyState title="No releases found" description="This mod does not publish a compatible downloadable release."/>
            : <div className="ui-table-wrap max-h-96 overflow-y-auto">
                <table className="ui-table" style={{minWidth: "30rem"}}>
                    <thead><tr><th>Version</th><th>Compatibility</th><th className="text-right">Dependencies</th></tr></thead>
                    <tbody>{[...releases].reverse().map(release => <tr key={`${release.version}-${release.file_name}`}>
                        <td><span className="font-mono text-white">{release.version}</span></td>
                        <td>{release.compatibility
                            ? <span className="text-green"><FontAwesomeIcon icon={faCheck}/> Compatible</span>
                            : <span className="text-red"><FontAwesomeIcon icon={faTimes}/> Incompatible</span>}</td>
                        <td><div className="flex justify-end">{release.compatibility && <Button
                            size="sm"
                            type="secondary"
                            isLoading={reviewingVersion === release.version}
                            isDisabled={Boolean(reviewingVersion)}
                            onClick={() => select(release)}
                        ><FontAwesomeIcon icon={faCloudArrowDown}/> Review</Button>}</div></td>
                    </tr>)}</tbody>
                </table>
            </div>}
        actions={<Button onClick={close} size="sm" type="secondary" isDisabled={Boolean(reviewingVersion)}>Close</Button>}
    />;
};

export default SelectVersionForm;
