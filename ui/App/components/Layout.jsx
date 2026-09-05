import React, {useEffect, useRef, useState} from "react";
import {Link, NavLink, Outlet, useLocation} from "react-router-dom";
import {FontAwesomeIcon} from "@fortawesome/react-fontawesome";
import {
    faBars, faCheck, faChevronRight, faCloudArrowDown, faFileLines, faFloppyDisk,
    faGamepad, faGaugeHigh, faLayerGroup, faMugHot, faMusic, faPuzzlePiece,
    faPlus, faRightFromBracket, faSliders, faTerminal, faUsers, faXmark
} from "@fortawesome/free-solid-svg-icons";
import Button from "./Button";
import ProfileContextBar from "./ProfileContextBar";
import HelpTip from "./HelpTip";
import {useProfiles} from "../context/ProfileContext";
import profilesResource from "../../api/resources/profiles";
import BrandMark from "./BrandMark";
import {lockBodyScroll} from "./overlay";

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
const isManagerPath = pathname => ["/profiles", "/user-management"].some(path => pathname === path || pathname.startsWith(path + "/"));

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

const Layout = ({handleLogout, serverStatus, refreshServerStatus, currentUser, socketState, canManage = false}) => {
    const [isNavOpen, setIsNavOpen] = useState(false);
    const [isCompactNavigation, setIsCompactNavigation] = useState(() => window.matchMedia("(max-width: 1099px)").matches);
    const [isProfileMenuOpen, setIsProfileMenuOpen] = useState(false);
    const [switchingProfileID, setSwitchingProfileID] = useState("");
    const profileMenuRef = useRef(null);
    const sidebarRef = useRef(null);
    const location = useLocation();
    const {state, activeProfile, isLoading: isLoadingProfile, applyProfileState} = useProfiles();
    const isPrimaryMobileRoute = mobileLinks.some(([, to]) => to === location.pathname);
    const profileSwitchLocked = !canManage || serverStatus?.known === false || serverStatus?.running || serverStatus?.stopping;
    const visibleProfileLinks = canManage ? profileLinks : profileLinks.filter(([, path]) => path !== "/console");
    const visibleManagerLinks = canManage
        ? managerLinks
        : managerLinks.map(item => item[1] === "/user-management" ? ["Account & access", item[1], item[2]] : item);
    const outletKey = isManagerPath(location.pathname)
        ? `manager:${location.pathname}`
        : `profile:${activeProfile?.id || "loading"}`;
    const revision = !["", "unknown", "local"].includes(__FSM_UI_REVISION__) ? ` · ${__FSM_UI_REVISION__.slice(0, 8)}` : "";

    useEffect(() => {
        const media = window.matchMedia("(max-width: 1099px)");
        const update = event => setIsCompactNavigation(event.matches);
        if (media.addEventListener) media.addEventListener("change", update);
        else media.addListener(update);
        return () => {
            if (media.removeEventListener) media.removeEventListener("change", update);
            else media.removeListener(update);
        };
    }, []);

    useEffect(() => {
        const hidden = isCompactNavigation && !isNavOpen;
        sidebarRef.current?.toggleAttribute("inert", hidden);
    }, [isCompactNavigation, isNavOpen]);

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
        if (!isNavOpen || !isCompactNavigation) return undefined;
        const closeOnEscape = event => {
            if (event.key === "Escape") setIsNavOpen(false);
        };
        const unlockBodyScroll = lockBodyScroll();
        document.addEventListener("keydown", closeOnEscape);
        return () => {
            unlockBodyScroll();
            document.removeEventListener("keydown", closeOnEscape);
        };
    }, [isCompactNavigation, isNavOpen]);

    const switchProfile = async profile => {
        if (!profile || profile.active || profileSwitchLocked) return;
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

    const sidebarContent = <>
        <div className="ui-sidebar__brand">
            <BrandMark/>
            <div className="ui-sidebar__brand-copy">
                <strong>Factorio Server Control</strong>
            </div>
            <button className="ui-sidebar__close" onClick={() => setIsNavOpen(false)} aria-label="Close navigation">
                <FontAwesomeIcon icon={faXmark}/>
            </button>
        </div>

        <div className="ui-profile-menu-wrap" ref={profileMenuRef}>
            <button
                className="ui-profile-switcher"
                type="button"
                aria-expanded={isProfileMenuOpen}
                aria-controls="profile-switch-menu"
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

            {isProfileMenuOpen && <div className="ui-profile-menu" id="profile-switch-menu" aria-label="Switch profile">
                <div className="ui-profile-menu__header">
                    <span>Profiles</span>
                    {profileSwitchLocked && <HelpTip content={!canManage ? "Viewer accounts cannot switch profiles." : serverStatus?.known === false ? "Wait until the Factorio process status is known." : "Stop Factorio before switching profiles."} label="Why profile switching is disabled"/>}
                </div>
                <div className="ui-profile-menu__list">
                    {(state?.profiles || []).map(profile => <button
                        className={`ui-profile-menu__item${profile.active ? " is-active" : ""}`}
                        type="button"
                        key={profile.id}
                        disabled={profile.active || Boolean(switchingProfileID) || profileSwitchLocked}
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
                {canManage && <Link className="ui-profile-menu__new" to="/profiles?new=fresh">
                    <FontAwesomeIcon icon={faPlus}/> New profile
                </Link>}
            </div>}
        </div>

        <div className="ui-sidebar__scroll">
            <nav aria-label="Primary navigation">
                <div className="ui-nav-section">
                    <p className="ui-nav-section-title">Server</p>
                    {visibleProfileLinks.map(item => <NavigationLink key={item[1]} item={item}/>) }
                </div>
                <div className="ui-nav-section">
                    <p className="ui-nav-section-title">Manager</p>
                    {visibleManagerLinks.map(item => <NavigationLink key={item[1]} item={item}/>) }
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
                    title={socketState === "connected" ? "Control connection online" : socketState === "connecting" ? "Control connection is reconnecting" : "Live updates offline"}
                    aria-label={socketState === "connected" ? "Control connection online" : socketState === "connecting" ? "Control connection is reconnecting" : "Live updates offline"}
                />
                <span className="ui-manager-product" title="Factorio Server Control">Factorio Server Control</span>
                <span className="ui-manager-version" title={`Factorio Server Control UI revision ${__FSM_UI_REVISION__}`}>v{__FSM_UI_VERSION__}{revision}</span>
            </div>
        </div>
    </>;

    return <div className="ui-app-shell">
        <header className="ui-mobile-appbar">
            <button className="ui-mobile-menu" onClick={() => setIsNavOpen(true)} aria-label="Open navigation">
                <FontAwesomeIcon icon={faBars}/>
            </button>
            <BrandMark/>
            <div className="ui-mobile-brand-copy">
                <strong>Factorio Server Control</strong>
                <span>{activeProfile?.name || "Manager"}</span>
            </div>
        </header>

        {isNavOpen && <button className="ui-sidebar-scrim" onClick={() => setIsNavOpen(false)} aria-label="Close navigation overlay"/>}
        <aside
            ref={sidebarRef}
            className={`ui-sidebar${isNavOpen ? " is-open" : ""}`}
            aria-label="Application navigation"
            aria-hidden={isCompactNavigation && !isNavOpen ? "true" : undefined}
        >
            {sidebarContent}
        </aside>

        <main className="ui-main">
            <ProfileContextBar serverStatus={serverStatus} refreshServerStatus={refreshServerStatus} canManage={canManage}/>
            <div className="ui-page">
                <Outlet key={outletKey}/>
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
