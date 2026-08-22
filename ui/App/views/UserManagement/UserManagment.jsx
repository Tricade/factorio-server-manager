import React, {useCallback, useEffect, useState} from "react";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {faKey, faTrashAlt, faUserPlus, faUsers} from "@fortawesome/free-solid-svg-icons";
import user from "../../../api/resources/user";
import PageHeader from "../../components/PageHeader";
import Panel from "../../components/Panel";
import IconButton from "../../components/IconButton";
import EmptyState from "../../components/EmptyState";
import ConfirmDialog from "../../components/ConfirmDialog";
import CreateUserForm from "./components/CreateUserForm";
import ChangePasswordForm from "./components/ChangePasswordForm";

const UserManagement = ({currentUser}) => {
    const [users, setUsers] = useState([]);
    const [userToDelete, setUserToDelete] = useState(null);

    const updateList = useCallback(async () => {
        try {
            setUsers(await user.list() || []);
        } catch (error) {
            window.flash(error?.response?.data || "Users could not be loaded.", "red");
        }
    }, []);

    useEffect(() => { updateList(); }, [updateList]);

    const deleteUser = async username => {
        await user.delete(username);
        await updateList();
        window.flash(`${username} removed.`, "green");
    };

    return <>
        <PageHeader title="Users & access" actions={<div className="ui-status-badge">{users.length} {users.length === 1 ? "user" : "users"}</div>}/>
        <Panel
            title="Manager users"
            description="Every account currently has administrative access to this manager."
            className="mb-5"
            content={users.length === 0
                ? <EmptyState icon={faUsers} title="No users returned"/>
                : <div className="ui-table-wrap"><table className="ui-table">
                    <thead><tr><th>User</th><th>Role</th><th>Email</th><th className="text-right">Actions</th></tr></thead>
                    <tbody>{users.map(entry => <tr key={entry.username}>
                        <td><div className="flex items-center gap-3"><div className="grid w-8 h-8 place-items-center rounded-full bg-orange bg-opacity-10 text-orange font-bold">{entry.username.slice(0, 1).toUpperCase()}</div><div><span className="font-bold text-white">{entry.username}</span>{entry.username === currentUser?.username && <span className="ml-2 text-xs text-orange">You</span>}</div></div></td>
                        <td><span className="ui-status-badge">{entry.role}</span></td>
                        <td>{entry.email || "—"}</td>
                        <td><div className="flex justify-end"><IconButton type="danger" label={`Delete ${entry.username}`} icon={faTrashAlt} disabled={entry.username === currentUser?.username} onClick={() => setUserToDelete(entry)}/></div></td>
                    </tr>)}</tbody>
                </table></div>}
        />
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-5">
            <Panel title={`Change ${currentUser?.username || "your"} password`} headerAction={<FontAwesomeIcon className="text-orange" icon={faKey}/>} content={<ChangePasswordForm/>}/>
            <Panel title="Create user" headerAction={<FontAwesomeIcon className="text-orange" icon={faUserPlus}/>} content={<CreateUserForm updateUserList={updateList}/>}/>
        </div>
        <ConfirmDialog
            title="Delete user?"
            content={userToDelete ? `${userToDelete.username} will lose access to this manager.` : ""}
            isOpen={Boolean(userToDelete)}
            close={() => setUserToDelete(null)}
            onSuccess={() => deleteUser(userToDelete.username)}
        />
    </>;
};

export default UserManagement;
