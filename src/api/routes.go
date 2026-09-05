package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/api/websocket"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
	ServerOff   bool // Set to `true' if factorio server has to be turned off to call this
}

type Routes []Route

func ServerOffMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// only run if server is turned off
		server := factorio.GetFactorioServer()
		if server.IsBusy() {
			http.Error(w, "factorio server still running", http.StatusLocked)
		} else {
			next.ServeHTTP(w, r)
		}
		return
	})
}

func ProfileDataMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Profile handlers own the exclusive lock themselves. Every other API
		// request holds a shared lock so an activation cannot replace saves,
		// mods or config while that request is using them.
		path := strings.TrimSuffix(r.URL.Path, "/")
		// Start owns the shared profile-data lock until the child process is
		// launched. Taking it here as well could deadlock behind a queued writer.
		modPackLoadOwnsLock := strings.HasPrefix(path, "/api/mods/packs/") && strings.HasSuffix(path, "/load")
		saveModImportOwnsLock := path == "/api/saves/mods/import"
		modStartupSettingsOwnsLock := path == "/api/mods/startup-settings"
		if strings.HasPrefix(path, "/api/profiles") || path == "/api/server/start" || modPackLoadOwnsLock || saveModImportOwnsLock || modStartupSettingsOwnsLock {
			next.ServeHTTP(w, r)
			return
		}
		unlock := factorio.LockProfileDataRead()
		defer unlock()
		next.ServeHTTP(w, r)
	})
}

func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func frontendFileHandler(prefix string) http.Handler {
	handler := http.Handler(http.FileServer(http.Dir("./app/")))
	if prefix != "" {
		handler = http.StripPrefix(prefix, handler)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The generated index adds the UI version to asset URLs. Revalidation here
		// also prevents an old index document from pinning a previous UI bundle.
		w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
		handler.ServeHTTP(w, r)
	})
}

func NewRouter() *mux.Router {
	mainRouter := mux.NewRouter().StrictSlash(true)
	mainRouter.Use(SecurityHeadersMiddleware)

	// create subrouter for authenticated calls
	subRouter := mainRouter.NewRoute().Subrouter()
	subRouter.Use(AuthMiddleware)

	// API subrouter
	// Serves all JSON REST handlers prefixed with /api
	apiRouter := mainRouter.PathPrefix("/api").Subrouter()
	apiRouter.Use(AuthMiddleware)
	apiRouter.Use(ProfileDataMiddleware)

	// use subrouter for calls, that run only, when server is turned off
	serverOffRouter := apiRouter.NewRoute().Subrouter()
	serverOffRouter.Use(ServerOffMiddleware)

	apiRouter.NewRoute().Subrouter()
	for _, route := range apiRoutes {
		var router *mux.Router
		if route.ServerOff {
			router = serverOffRouter
		} else {
			router = apiRouter
		}
		handler := http.Handler(route.HandlerFunc)
		if routeRequiresAdministrator(route) {
			handler = RequireAdministrator(handler)
		}
		router.Path(route.Pattern).
			Methods(route.Method).
			Name(route.Name).
			Handler(handler)
	}
	// Gorilla subrouters otherwise turn a method mismatch into a generic 404.
	// Register path-only fallbacks after every real route so obsolete mutating
	// GET clients and other wrong methods receive an explicit 405 and can never
	// fall through to the SPA handler.
	for _, route := range apiRoutes {
		apiRouter.Path(route.Pattern).HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		})
	}

	// The login handler does not check for authentication.
	mainRouter.Path("/api/login").
		Methods("POST").
		Name("LoginUser").
		HandlerFunc(LoginUser)

	// Route for initializing websocket connection
	// Clients connecting to /ws establish websocket connection by upgrading
	// HTTP session.
	// Ensure user is logged in with the AuthorizeHandler middleware
	subRouter.Path("/ws").
		Methods("GET").
		Name("Websocket").
		Handler(
			http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					principal, ok := authenticatedUserFromRequest(r)
					if !ok {
						http.Error(w, "authentication is no longer valid", http.StatusUnauthorized)
						return
					}
					current, err := auth.getUser(principal.Username)
					if err != nil {
						http.Error(w, "authentication is no longer valid", http.StatusUnauthorized)
						return
					}
					websocket.ServeWs(w, r, websocketCommandAuthorizer(current.Username, current.Password))
				},
			),
		)

	// Serves the frontend application from the app directory
	// Uses basic file server to serve index.html and Javascript application
	// Routes match the ones defined in React frontend application
	mainRouter.Path("/login").
		Methods("GET").
		Name("Login").
		Handler(frontendFileHandler("/login"))

	subRouter.Path("/saves").
		Methods("GET").
		Name("Saves").
		Handler(frontendFileHandler("/saves"))
	subRouter.Path("/mods").
		Methods("GET").
		Name("Mods").
		Handler(frontendFileHandler("/mods"))
	subRouter.Path("/server-settings").
		Methods("GET").
		Name("Server settings").
		Handler(frontendFileHandler("/server-settings"))
	subRouter.Path("/game-settings").
		Methods("GET").
		Name("Game settings").
		Handler(frontendFileHandler("/game-settings"))
	subRouter.Path("/console").
		Methods("GET").
		Name("Console").
		Handler(frontendFileHandler("/console"))
	subRouter.Path("/logs").
		Methods("GET").
		Name("Logs").
		Handler(frontendFileHandler("/logs"))
	subRouter.Path("/releases").
		Methods("GET").
		Name("Releases").
		Handler(frontendFileHandler("/releases"))
	subRouter.Path("/profiles").
		Methods("GET").
		Name("Profiles").
		Handler(frontendFileHandler("/profiles"))
	subRouter.Path("/user-management").
		Methods("GET").
		Name("User management").
		Handler(frontendFileHandler("/user-management"))
	// catch all route
	mainRouter.PathPrefix("/").
		Methods("GET").
		Name("Index").
		Handler(frontendFileHandler(""))

	return mainRouter
}

func websocketCommandAuthorizer(username, passwordHash string) func() bool {
	return func() bool {
		current, err := auth.getUser(username)
		if err != nil || current.Role != UserRoleAdmin {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(current.Password), []byte(passwordHash)) == 1
	}
}

var nonAdminMutationRoutes = map[string]bool{
	"LogoutUser":     true,
	"ChangePassword": true,
}

// Mutations are administrator-only by default. The two explicit exceptions
// are self-service session/password actions. User enumeration is also
// administrator-only even though it is read-only.
func routeRequiresAdministrator(route Route) bool {
	if route.Name == "ListUsers" || route.Name == "GetModStartupSettings" {
		return true
	}
	method := strings.ToUpper(route.Method)
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return !nonAdminMutationRoutes[route.Name]
}

// Defines all API REST endpoints
// All routes are prefixed with /api
var apiRoutes = Routes{
	{
		"ListProfiles",
		"GET",
		"/profiles",
		ListProfilesHandler,
		false,
	}, {
		"CreateProfile",
		"POST",
		"/profiles",
		CreateProfileHandler,
		false,
	}, {
		"UpdateProfile",
		"PATCH",
		"/profiles/{profile}",
		UpdateProfileHandler,
		false,
	}, {
		"UpdateProfileStartup",
		"PUT",
		"/profiles/{profile}/startup",
		UpdateProfileStartupHandler,
		true,
	}, {
		"DeleteProfile",
		"DELETE",
		"/profiles/{profile}",
		DeleteProfileHandler,
		false,
	}, {
		"ActivateProfile",
		"POST",
		"/profiles/{profile}/activate",
		ActivateProfileHandler,
		false,
	},
	{
		"ListSaves",
		"GET",
		"/saves/list",
		ListSaves,
		false,
	}, {
		"DlSave",
		"GET",
		"/saves/dl/{save}",
		DLSave,
		false,
	}, {
		"UploadSave",
		"POST",
		"/saves/upload",
		UploadSave,
		true,
	}, {
		"RemoveSave",
		"DELETE",
		"/saves/rm/{save}",
		RemoveSave,
		true,
	}, {
		"CreateSave",
		"POST",
		"/saves/create/{save}",
		CreateSaveHandler,
		true,
	}, {
		"GetWorldGenerationOptions",
		"GET",
		"/saves/generation/options",
		GetWorldGenerationOptions,
		false,
	}, {
		"PreviewWorld",
		"POST",
		"/saves/generation/preview",
		PreviewWorld,
		true,
	}, {
		"CreateWorld",
		"POST",
		"/saves/generation/create",
		CreateWorld,
		true,
	}, {
		"LoadModsFromSave",
		"POST",
		"/saves/mods",
		LoadModsFromSaveHandler,
		true,
	}, {
		"ImportModsFromSave",
		"POST",
		"/saves/mods/import",
		ImportModsFromSaveHandler,
		true,
	}, {
		"ListCheckpoints",
		"GET",
		"/checkpoints",
		ListCheckpoints,
		false,
	}, {
		"SaveCheckpointSettings",
		"PUT",
		"/checkpoints/settings",
		SaveCheckpointSettings,
		false,
	}, {
		"CreateCheckpoint",
		"POST",
		"/checkpoints",
		CreateCheckpoint,
		false,
	}, {
		"DownloadCheckpoint",
		"GET",
		"/checkpoints/{checkpoint}/download",
		DownloadCheckpoint,
		false,
	}, {
		"RestoreCheckpoint",
		"POST",
		"/checkpoints/{checkpoint}/restore",
		RestoreCheckpoint,
		true,
	}, {
		"DeleteCheckpoint",
		"DELETE",
		"/checkpoints/{checkpoint}",
		DeleteCheckpoint,
		false,
	}, {
		"LogTail",
		"GET",
		"/log/tail",
		LogTail,
		false,
	}, {
		"LoadConfig",
		"GET",
		"/config",
		LoadConfig,
		false,
	}, {
		"StartServer",
		"POST",
		"/server/start",
		StartServer,
		true,
	}, {
		"StopServer",
		"POST",
		"/server/stop",
		StopServer,
		false,
	}, {
		"KillServer",
		"POST",
		"/server/kill",
		KillServer,
		false,
	}, {
		"RunningServer",
		"GET",
		"/server/status",
		CheckServer,
		false,
	}, {
		"PlayerOverview",
		"GET",
		"/server/players",
		GetPlayerOverview,
		false,
	}, {
		"GetAutostartSettings",
		"GET",
		"/server/autostart",
		GetAutostartSettings,
		false,
	}, {
		"UpdateAutostartSettings",
		"PUT",
		"/server/autostart",
		UpdateAutostartSettings,
		false,
	}, {
		"GetMapSnapshot",
		"GET",
		"/map-snapshot",
		GetMapSnapshot,
		false,
	}, {
		"RefreshMapSnapshot",
		"POST",
		"/map-snapshot/refresh",
		RefreshMapSnapshot,
		false,
	}, {
		"UpdateMapSnapshotSettings",
		"PUT",
		"/map-snapshot/settings",
		UpdateMapSnapshotSettings,
		false,
	}, {
		"GetMapSnapshotImage",
		"GET",
		"/map-snapshot/surfaces/{surface}",
		GetMapSnapshotImage,
		false,
	}, {
		"GetMapSnapshotEntities",
		"GET",
		"/map-snapshot/surfaces/{surface}/entities",
		GetMapSnapshotEntities,
		false,
	}, {
		"FactorioVersion",
		"GET",
		"/server/facVersion",
		FactorioVersion,
		false,
	}, {
		"InstallFactorioRelease",
		"POST",
		"/server/release/install",
		InstallFactorioRelease,
		true,
	}, {
		"FactorioReleaseStatus",
		"GET",
		"/server/release/status",
		FactorioReleaseStatus,
		false,
	}, {
		"GetGameMode",
		"GET",
		"/server/game-mode",
		GetGameMode,
		false,
	}, {
		"UpdateGameMode",
		"POST",
		"/server/game-mode",
		UpdateGameMode,
		true,
	}, {
		"LogoutUser",
		"POST",
		"/logout",
		LogoutUser,
		false,
	}, {
		"StatusUser",
		"GET",
		"/user/status",
		GetCurrentLogin,
		false,
	}, {
		"ListUsers",
		"GET",
		"/user/list",
		ListUsers,
		false,
	}, {
		"AddUser",
		"POST",
		"/user/add",
		AddUser,
		false,
	}, {
		"RemoveUser",
		"POST",
		"/user/remove",
		RemoveUser,
		false,
	}, {
		"ChangePassword",
		"POST",
		"/user/password",
		ChangePassword,
		false,
	}, {
		"GetServerSettings",
		"GET",
		"/settings",
		GetServerSettings,
		false,
	}, {
		"UpdateServerSettings",
		"POST",
		"/settings/update",
		UpdateServerSettings,
		true,
	},
	// Mod Portal Stuff
	{
		"ModPortalListAllMods",
		"GET",
		"/mods/portal/list",
		ModPortalListModsHandler,
		false,
	}, {
		"ModPortalGetModInfo",
		"GET",
		"/mods/portal/info/{mod}",
		ModPortalModInfoHandler,
		false,
	}, {
		"ModPortalInstallMod",
		"POST",
		"/mods/portal/install",
		ModPortalInstallHandler,
		true,
	}, {
		"ModPortalPlanInstall",
		"POST",
		"/mods/portal/install/plan",
		ModPortalInstallPlanHandler,
		false,
	}, {
		"ModPortalInstallResolved",
		"POST",
		"/mods/portal/install/resolved",
		ModPortalInstallResolvedHandler,
		true,
	}, {
		"ModPortalLogin",
		"POST",
		"/mods/portal/login",
		ModPortalLoginHandler,
		false,
	}, {
		"ModPortalLoginStatus",
		"GET",
		"/mods/portal/loginstatus",
		ModPortalLoginStatusHandler,
		false,
	}, {
		"ModPortalLogout",
		"DELETE",
		"/mods/portal/logout",
		ModPortalLogoutHandler,
		false,
	}, {
		"ModPortalInstallMultiple",
		"POST",
		"/mods/portal/install/multiple",
		ModPortalInstallMultipleHandler,
		true,
	},
	// Mods Stuff
	{
		"ListInstalledMods",
		"GET",
		"/mods/list",
		ListInstalledModsHandler,
		false,
	}, {
		"ToggleMod",
		"POST",
		"/mods/toggle",
		ModToggleHandler,
		true,
	}, {
		"DeleteMod",
		"POST",
		"/mods/delete",
		ModDeleteHandler,
		true,
	}, {
		"DeleteAllMods",
		"POST",
		"/mods/delete/all",
		ModDeleteAllHandler,
		true,
	}, {
		"UpdateMod",
		"POST",
		"/mods/update",
		ModUpdateHandler,
		true,
	}, {
		"UploadMod",
		"POST",
		"/mods/upload",
		ModUploadHandler,
		true,
	}, {
		"DownloadMods",
		"GET",
		"/mods/download",
		ModDownloadHandler,
		false,
	}, {
		"GetModStartupSettings",
		"GET",
		"/mods/startup-settings",
		GetModStartupSettingsHandler,
		true,
	}, {
		"UpdateModStartupSettings",
		"PATCH",
		"/mods/startup-settings",
		UpdateModStartupSettingsHandler,
		true,
	},
	// Mod Packs
	{
		"ModPacksList",
		"GET",
		"/mods/packs/list",
		ModPackListHandler,
		false,
	}, {
		"ModPackCreate",
		"POST",
		"/mods/packs/create",
		ModPackCreateHandler,
		false,
	}, {
		"ModPackDelete",
		"POST",
		"/mods/packs/{modpack}/delete",
		ModPackDeleteHandler,
		false,
	}, {
		"ModPackDownload",
		"GET",
		"/mods/packs/{modpack}/download",
		ModPackDownloadHandler,
		false,
	}, {
		"LoadModPack",
		"POST",
		"/mods/packs/{modpack}/load",
		ModPackLoadHandler,
		true,
	},
	// Mods inside Mod Packs
	{
		"ModPackListMods",
		"GET",
		"/mods/packs/{modpack}/list",
		ModPackModListHandler,
		false,
	}, {
		"ModPackToggleMod",
		"POST",
		"/mods/packs/{modpack}/mod/toggle",
		ModPackModToggleHandler,
		false,
	}, {
		"ModPackDeleteMod",
		"POST",
		"/mods/packs/{modpack}/mod/delete",
		ModPackModDeleteHandler,
		false,
	}, {
		"ModPackDeleteAllMod",
		"POST",
		"/mods/packs/{modpack}/mod/delete/all",
		ModPackModDeleteAllHandler,
		false,
	}, {
		"ModPackUpdateMod",
		"POST",
		"/mods/packs/{modpack}/mod/update",
		ModPackModUpdateHandler,
		false,
	}, {
		"ModPackUploadMod",
		"POST",
		"/mods/packs/{modpack}/mod/upload",
		ModPackModUploadHandler,
		false,
	}, {
		"ModPackModPortalInstallMod",
		"POST",
		"/mods/packs/{modpack}/portal/install",
		ModPackModPortalInstallHandler,
		false,
	}, {
		"ModPackModPortalInstallMultiple",
		"POST",
		"/mods/packs/{modpack}/portal/install/multiple",
		ModPackModPortalInstallMultipleHandler,
		false,
	},
}
