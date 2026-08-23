import React from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faCheck, faCloudArrowDown, faCubes, faSpinner, faTriangleExclamation} from "@fortawesome/free-solid-svg-icons";
import Modal from "../../../../../components/Modal";
import Button from "../../../../../components/Button";
import Alert from "../../../../../components/Alert";

const DependencyMeta = ({item}) => <div className="mt-1 flex flex-wrap gap-2 text-xs text-gray-light">
    <span className="font-mono">{item.name} · {item.version}</span>
    {item.built_in && <span className="text-blue-light">Built in</span>}
    {item.installed && <span className="text-green">Installed</span>}
    {item.required_by?.length > 0 && <span>for {item.required_by.join(", ")}</span>}
</div>;

const RequiredDependency = ({item}) => <label className="ui-checkbox ui-dependency">
    <input type="checkbox" checked readOnly disabled/>
    <span className="min-w-0">
        <span className="block font-semibold text-white">{item.title || item.name}</span>
        <DependencyMeta item={item}/>
    </span>
</label>;

const OptionalDependency = ({item, onToggle, disabled}) => <label className="ui-checkbox ui-dependency">
    <input
        type="checkbox"
        checked={Boolean(item.selected)}
        disabled={disabled}
        onChange={event => onToggle(item.name, event.target.checked)}
    />
    <span className="min-w-0">
        <span className="block font-semibold text-white">{item.title || item.name}</span>
        <DependencyMeta item={item}/>
        <span className={`mt-1 block text-xs ${item.kind === "recommended" ? "text-orange" : "text-gray-light"}`}>
            {item.kind === "recommended" ? "Recommended, but not required" : "Optional extension"}
        </span>
    </span>
</label>;

const DependencyInstallForm = ({plan, isOpen, close, onToggleOptional, onInstall, isPlanning, isInstalling}) => {
    const hasConflicts = Boolean(plan?.conflicts?.length);
    const required = plan?.required || [];
    const optional = plan?.optional || [];
    const selectedCount = optional.filter(item => item.selected).length;

    return <Modal
        isOpen={isOpen}
        title="Review mod dependencies"
        close={close}
        dismissDisabled={isPlanning || isInstalling}
        content={!plan
            ? <div className="ui-empty-state"><FontAwesomeIcon icon={faSpinner} spin/><p className="mt-3">Resolving dependencies…</p></div>
            : <div className="max-h-[65vh] space-y-5 overflow-y-auto pr-1">
                <div className="ui-dependency-root">
                    <div className="flex items-center gap-3">
                        <FontAwesomeIcon className="text-orange" icon={faCubes}/>
                        <div>
                            <div className="font-bold text-white">{plan.root.title || plan.root.name}</div>
                            <DependencyMeta item={plan.root}/>
                        </div>
                    </div>
                </div>

                {hasConflicts && <Alert type="danger">
                    <FontAwesomeIcon icon={faTriangleExclamation}/> Disable or remove these incompatible mods first: {plan.conflicts.map(conflict => conflict.name).join(", ")}.
                </Alert>}

                <section>
                    <div className="mb-2 flex items-center justify-between gap-3">
                        <h3 className="font-bold text-white">Required</h3>
                        <span className="text-xs text-gray-light">Selected automatically</span>
                    </div>
                    {required.length === 0
                        ? <div className="rounded-lg border border-white border-opacity-5 p-3 text-sm text-gray-light"><FontAwesomeIcon className="mr-2 text-green" icon={faCheck}/>No additional required downloads.</div>
                        : <div className="space-y-2">{required.map(item => <RequiredDependency key={item.name} item={item}/>)}</div>}
                </section>

                {optional.length > 0 && <section>
                    <div className="mb-2 flex items-center justify-between gap-3">
                        <h3 className="font-bold text-white">Optional</h3>
                        <span className="text-xs text-gray-light">Off by default</span>
                    </div>
                    <div className="space-y-2">{optional.map(item => <OptionalDependency
                        key={item.name}
                        item={item}
                        disabled={isPlanning || isInstalling}
                        onToggle={onToggleOptional}
                    />)}</div>
                </section>}

                {plan.warnings?.map(warning => <Alert key={warning} type="warning">{warning}</Alert>)}
            </div>}
        actions={<>
            <Button onClick={close} size="sm" type="secondary" isDisabled={isPlanning || isInstalling}>Back</Button>
            <Button
                onClick={onInstall}
                size="sm"
                type="success"
                isLoading={isInstalling}
                isDisabled={!plan || isPlanning || hasConflicts}
            >
                <FontAwesomeIcon icon={faCloudArrowDown}/> Download mod set{selectedCount ? ` + ${selectedCount} optional` : ""}
            </Button>
        </>}
    />;
};

export default DependencyInstallForm;
