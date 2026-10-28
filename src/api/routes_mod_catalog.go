package api

// Mod API routes follow the general server and user routes in apiRoutes.
var apiModRoutes = Routes{
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
