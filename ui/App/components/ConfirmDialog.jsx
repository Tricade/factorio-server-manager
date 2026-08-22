import React, {useState} from 'react';
import Modal from "./Modal";
import Button from "./Button";

function ConfirmDialog({title, content, isOpen, close, onSuccess}) {

    const [isLoading, setIsLoading] = useState(false);

    const confirm = async () => {
        setIsLoading(true);
        try {
            await Promise.resolve(onSuccess());
            close();
        } catch (error) {
            window.flash(error?.response?.data || "The action could not be completed.", "red");
        } finally {
            setIsLoading(false);
        }
    }

    return (
        <Modal
            title={title}
            content={content}
            actions={
                <>
                    <Button size="sm" type="secondary" isDisabled={isLoading} onClick={close}>Cancel</Button>
                    <Button size="sm" isLoading={isLoading} type="danger" onClick={confirm}>Confirm</Button>
                </>
            }
            isOpen={isOpen}
        />
    );
}

export default ConfirmDialog;
