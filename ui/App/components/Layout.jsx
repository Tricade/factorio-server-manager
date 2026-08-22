import React, {useEffect, useRef, useState} from "react";
import {Link, NavLink, Outlet, useLocation} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faBars, faCheck, faChevronRight, faCloudArrowDown, faFileLines, faFloppyDisk,
    faGamepad, faGaugeHigh, faLayerGroup, faMugHot, faMusic, faPuzzlePiece,
    faPlus, faRightFromBracket, faServer, faSliders, faTerminal, faUsers, faXmark
} from "@fortawesome/free-solid-svg-icons";
import Button from "./Button";
import ProfileContextBar from "./ProfileContextBar";
import HelpTip from "./HelpTip";
import {useProfiles} from "../context/ProfileContext";
import profilesResource from "../../api/resources/profiles";

const profileLinks = [
    ["Overview", "/", faGaugeHigh],
    ["Saves & Checkpoints", "/saves", faFloppyDisk],
    ["Mods", "/mods", faPuzzlePiece],
    ["Server settings", "/server-settings", faSliders],
    ["Game settings", "/game-settings", faGamepad],
    ["Console", "/console", faTerminal],
    ["Logs", "/logs", faFileLines],
    ["Version & mode", "/releases", faCloudArrowDown]
];

const managerLinks = [
    ["Profiles", "/profiles", faLayerGroup],
    ["Users & access", "/user-management", faUsers]
];

const externalReferenceLinks = [
    ["Factory radio", "https://suno.com/playlist/ccf52e9d-ab58-4950-aea1-b3bb7f2aa585", faMusic],
    ["Support on Ko-fi", "https://ko-fi.com/tricade", faMugHot]
];

const mobileLinks = [
    ["Overview", "/", faGaugeHigh],
    ["Saves", "/saves", faFloppyDisk],
    ["Mods", "/mods", faPuzzlePiece]
];

const modeLabel = mode => mode === "space-age" ? "Space Age" : mode === "factorio" ? "Factorio" : "Custom";

const Layout = ({handleLogout, serverStatus, refreshServerStatus, currentUser, socketState}) => {
    const [isNavOpen, setIsNavOpen] = useState(false);
    const [isProfileMenuOpen, setIsProfileMenuOpen] = useState(false);
    const [switchingProfileID, setSwitchingProfileID] = useState("");
    const profileMenuRef = useRef(null);
    const location = useLocation();
    const {state, activeProfile, isLoading: isLoadingProfile, applyProfileState} = useProfiles();
    const isPrimaryMobileRoute = mobileLinks.some(([, to]) => to === location.pathname);
    const revision = !["", "unknown", "local"].includes(__FSM_UI_REVISION__) ? ` · ${__FSM_UI_REVISION__.slice(0, 8)}` : "";

    useEffect(() => {
        setIsNavOpen(false);
        setIsProfileMenuOpen(false);
        window.scrollTo({top: 0, left: 0, behavior: "auto"});
    }, [location.pathname]);

    useEffect(() => {
        if (!isProfileMenuOpen) return undefined;
        const closeOnOutsideClick = event => {
            if (!profileMenuRef.current?.contains(event.target)) setIsProfileMenuOpen(false);
        };
        const closeOnEscape = event => {
            if (event.key === "Escape") setIsProfileMenuOpen(false);
        };
        document.addEventListener("pointerdown", closeOnOutsideClick);
        document.addEventListener("keydown", closeOnEscape);
        return () => {
            document.removeEventListener("pointerdown", closeOnOutsideClick);
            document.removeEventListener("keydown", closeOnEscape);
        };
    }, [isProfileMenuOpen]);

    useEffect(() => {
        if (!isNavOpen) return undefined;
        const closeOnEscape = event => {
            if (event.key === "Escape") setIsNavOpen(false);
        };
        const originalOverflow = document.body.style.overflow;
        document.body.style.overflow = "hidden";
        document.addEventListener("keydown", closeOnEscape);
        return () => {
            document.body.style.overflow = originalOverflow;
            document.removeEventListener("keydown", closeOnEscape);
        };
    }, [isNavOpen]);

    const switchProfile = async profile => {
        if (!profile || profile.active || serverStatus?.running || serverStatus?.stopping) return;
        setSwitchingProfileID(profile.id);
        try {
            applyProfileState(await profilesResource.activate(profile.id));
            await refreshServerStatus?.();
            setIsProfileMenuOpen(false);
            window.flash(`${profile.name} activated.`, "green");
        } catch (error) {
            const message = error?.response?.data?.error || error?.response?.data;
            window.flash(typeof message === "string" ? message : `${profile.name} could not be activated.`, "red");
        } finally {
            setSwitchingProfileID("");
        }
    };

    const NavigationLink = ({item}) => {
        const [label, to, icon] = item;
        return <NavLink
            end={to === "/"}
            to={to}
            className={({isActive}) => `ui-nav-link${isActive ? " is-active" : ""}`}
        >
            <FontAwesomeIcon className="ui-nav-link__icon" icon={icon}/>
            <span>{label}</span>
        </NavLink>;
    };

    const ExternalLink = ({item}) => {
        const [label, href, icon] = item;
        return <a className="ui-nav-link" href={href} target="_blank" rel="noreferrer">
            <FontAwesomeIcon className="ui-nav-link__icon" icon={icon}/>
            <span>{label}</span>
            <span className="ui-nav-link__external" aria-hidden="true">↗</span>
        </a>;
    };

    const SidebarContent = () => <>
        <div className="ui-sidebar__brand">
            <div className="ui-brand-mark"><FontAwesomeIcon icon={faServer}/></div>
            <div className="ui-sidebar__brand-copy">
                <strong>Factorio Server Manager</strong>
            </div>
            <button className="ui-sidebar__close" onClick={() => setIsNavOpen(false)} aria-label="Close navigation">
                <FontAwesomeIcon icon={faXmark}/>
            </button>
        </div>

        <div className="ui-profile-menu-wrap" ref={profileMenuRef}>
            <button
                className="ui-profile-switcher"
                type="button"
                aria-haspopup="menu"
                aria-expanded={isProfileMenuOpen}
                onClick={() => setIsProfileMenuOpen(open => !open)}
            >
                <div className="ui-profile-switcher__icon"><FontAwesomeIcon icon={faLayerGroup}/></div>
                <div className="ui-profile-switcher__copy">
                    <span>Active profile</span>
                    <strong>{isLoadingProfile && !activeProfile ? "Loading…" : activeProfile?.name || "Unavailable"}</strong>
                    {activeProfile && <small>{modeLabel(activeProfile.game_mode)} · {activeProfile.installed_version || "Unknown version"}</small>}
                </div>
                <FontAwesomeIcon className={`ui-profile-switcher__arrow${isProfileMenuOpen ? " is-open" : ""}`} icon={faChevronRight}/>
            </button>

            {isProfileMenuOpen && <div className="ui-profile-menu" role="menu" aria-label="Switch profile">
                <div className="ui-profile-menu__header">
                    <span>Profiles</span>
                    {(serverStatus?.running || serverStatus?.stopping) && <HelpTip content="Stop Factorio before switching profiles." label="Why profile switching is disabled"/>}
                </div>
                <div className="ui-profile-menu__list">
                    {(state?.profiles || []).map(profile => <button
                        className={`ui-profile-menu__item${profile.active ? " is-active" : ""}`}
                        type="button"
                        role="menuitem"
                        key={profile.id}
                        disabled={profile.active || Boolean(switchingProfileID) || serverStatus?.running || serverStatus?.stopping}
                        onClick={() => switchProfile(profile)}
                    >
                        <span>
                            <strong>{profile.name}</strong>
                            <small>{modeLabel(profile.game_mode)} · {profile.installed_version || "Unknown"}</small>
                        </span>
                        {profile.active && <FontAwesomeIcon icon={faCheck}/>}
                        {switchingProfileID === profile.id && <span className="ui-profile-menu__busy"/>}
                    </button>)}
                </div>
                <Link className="ui-profile-menu__new" to="/profiles?new=fresh" role="menuitem">
                    <FontAwesomeIcon icon={faPlus}/> New profile
                </Link>
            </div>}
        </div>

        <div className="ui-sidebar__scroll">
            <nav aria-label="Primary navigation">
                <div className="ui-nav-section">
                    <p className="ui-nav-section-title">Server</p>
                    {profileLinks.map(item => <NavigationLink key={item[1]} item={item}/>) }
                </div>
                <div className="ui-nav-section">
                    <p className="ui-nav-section-title">Manager</p>
                    {managerLinks.map(item => <NavigationLink key={item[1]} item={item}/>) }
                </div>
                <div className="ui-nav-section">
                    <p className="ui-nav-section-title">Reference</p>
                    {externalReferenceLinks.map(item => <ExternalLink key={item[1]} item={item}/>) }
                </div>
            </nav>
        </div>

        <div className="ui-sidebar__footer">
            <div className="ui-account-row">
                <div className="ui-account-avatar">{(currentUser?.username || "U").slice(0, 1).toUpperCase()}</div>
                <div className="ui-account-copy">
                    <strong>{currentUser?.username || "User"}</strong>
                    <span>{currentUser?.role || "operator"}</span>
                </div>
                <Button type="ghost" size="sm" className="ui-signout-button" onClick={handleLogout} title="Sign out">
                    <FontAwesomeIcon icon={faRightFromBracket}/><span>Sign out</span>
                </Button>
            </div>
            <div className="ui-manager-meta">
                <span
                    className={`ui-connection-dot ui-connection-dot--${socketState}`}
                    title={socketState === "connected" ? "Manager connection online" : socketState === "connecting" ? "Manager connection is reconnecting" : "Live updates offline"}
                    aria-label={socketState === "connected" ? "Manager connection online" : socketState === "connecting" ? "Manager connection is reconnecting" : "Live updates offline"}
                />
                <span className="ui-manager-version" title={`UI revision ${__FSM_UI_REVISION__}`}>v{__FSM_UI_VERSION__}{revision}</span>
            </div>
        </div>
    </>;

    return <div className="ui-app-shell">
        <header className="ui-mobile-appbar">
            <button className="ui-mobile-menu" onClick={() => setIsNavOpen(true)} aria-label="Open navigation">
                <FontAwesomeIcon icon={faBars}/>
            </button>
            <div className="ui-brand-mark"><FontAwesomeIcon icon={faServer}/></div>
            <div className="ui-mobile-brand-copy">
                <strong>Factorio Server Manager</strong>
                <span>{activeProfile?.name || "Manager"}</span>
            </div>
        </header>

        {isNavOpen && <button className="ui-sidebar-scrim" onClick={() => setIsNavOpen(false)} aria-label="Close navigation overlay"/>}
        <aside className={`ui-sidebar${isNavOpen ? " is-open" : ""}`} aria-label="Application navigation">
            <SidebarContent/>
        </aside>

        <main className="ui-main">
            <ProfileContextBar serverStatus={serverStatus} refreshServerStatus={refreshServerStatus}/>
            <div className="ui-page">
                <Outlet/>
            </div>
        </main>

        <nav className="ui-mobile-tabbar" aria-label="Mobile navigation">
            {mobileLinks.map(([label, to, icon]) => <NavLink
                key={to}
                end={to === "/"}
                to={to}
                className={({isActive}) => `ui-mobile-tab${isActive ? " is-active" : ""}`}
            >
                <FontAwesomeIcon icon={icon}/><span>{label}</span>
            </NavLink>)}
            <button className={`ui-mobile-tab${isPrimaryMobileRoute ? "" : " is-active"}`} type="button" onClick={() => setIsNavOpen(true)}>
                <FontAwesomeIcon icon={faBars}/><span>More</span>
            </button>
        </nav>
    </div>;
};

export default Layout;
