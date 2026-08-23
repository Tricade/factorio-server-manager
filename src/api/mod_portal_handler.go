package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

const maximumPortalBatchInstallItems = 128

var loadModPortalAuthenticationStatus = func() (bool, error) {
	var credentials factorio.Credentials
	return credentials.Load()
}

func ModPortalListModsHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var statusCode int
	resp, err, statusCode = factorio.ModPortalList()
	w.WriteHeader(statusCode)
	if err != nil {
		resp = fmt.Sprintf("Error in listing mods from mod portal: %s\nresponse: %+v", err, resp)
		log.Println(resp)
		return
	}
}

// ModPortalModInfoHandler returns JSON response with the mod details
func ModPortalModInfoHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	vars := mux.Vars(r)
	modId := vars["mod"]

	var statusCode int
	resp, err, statusCode = factorio.ModPortalModDetails(modId)

	if err != nil {
		resp = fmt.Sprintf("Error in getting mod details from mod portal: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode)
}

func ModPortalInstallHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	// Get Data out of the request
	var data struct {
		DownloadURL string `json:"downloadUrl"`
		Filename    string `json:"fileName"`
		ModName     string `json:"modName"`
	}
	resp, err = ReadFromRequestBody(w, r, &data)
	if err != nil {
		return
	}

	mods, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	err = mods.DownloadMod(data.DownloadURL, data.Filename, data.ModName)
	if err != nil {
		resp = fmt.Sprintf("Error downloading a mod: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = mods.ListInstalledMods()
}

// ModPortalInstallPlanHandler resolves required and optional dependencies for
// one exact mod release. The server chooses dependency versions so the browser
// never has to trust or construct portal download URLs.
func ModPortalInstallPlanHandler(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() { WriteResponse(w, resp) }()
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var request factorio.ModInstallPlanRequest
	var err error
	resp, err = ReadFromRequestBody(w, r, &request)
	if err != nil {
		return
	}

	plan, err := factorio.PlanModInstallation(request)
	if err != nil {
		resp = fmt.Sprintf("Error resolving mod dependencies: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	resp = plan
}

func ModPortalInstallResolvedHandler(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() { WriteResponse(w, resp) }()
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var request factorio.ModInstallPlanRequest
	var err error
	resp, err = ReadFromRequestBody(w, r, &request)
	if err != nil {
		return
	}

	installed, plan, err := factorio.InstallModPlan(request)
	if err != nil {
		resp = map[string]interface{}{"error": err.Error(), "plan": plan}
		log.Printf("Error installing resolved mod set: %s", err)
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	resp = installed
}

func ModPortalLoginHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var data struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	resp, err = ReadFromRequestBody(w, r, &data)
	if err != nil {
		return
	}

	err, statusCode := factorio.FactorioLogin(data.Username, data.Password)
	w.WriteHeader(statusCode)
	if err != nil {
		resp = fmt.Sprintf("Error trying to login into Factorio: %s", err)
		log.Println(resp)
		return
	}
}

func ModPortalLoginStatusHandler(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	authenticated, err := loadModPortalAuthenticationStatus()
	if err != nil {
		log.Printf("Error getting Factorio credential status: %s", err)
		resp = "Unable to read Factorio mod portal login status"
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp = authenticated
}

func ModPortalLogoutHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	var credentials factorio.Credentials
	err = credentials.Del()

	if err != nil {
		resp = fmt.Sprintf("Error on logging out of factorio: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = false
}

func ModPortalInstallMultipleHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var data []struct {
		Name    string           `json:"name"`
		Version factorio.Version `json:"version"`
	}
	resp, err = ReadFromRequestBody(w, r, &data)
	if err != nil {
		return
	}
	if len(data) > maximumPortalBatchInstallItems {
		resp = fmt.Sprintf("At most %d mods can be installed in one request", maximumPortalBatchInstallItems)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	modList, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	for _, datum := range data {
		// skip base mod because it is already included in factorio
		if datum.Name == "base" {
			continue
		}
		details, err, statusCode := factorio.ModPortalModDetails(datum.Name)
		if err != nil || statusCode != http.StatusOK {
			resp = fmt.Sprintf("Error in getting mod details from mod portal: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		//find correct mod-version
		var found = false
		for _, release := range details.Releases {
			if release.Version.Equals(datum.Version) {
				found = true

				err := modList.DownloadMod(release.DownloadURL, release.FileName, details.Name)
				if err != nil {
					resp = fmt.Sprintf("Error downloading mod {%s}, error: %s", details.Name, err)
					log.Println(resp)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				break
			}
		}
		if !found {
			log.Printf("Error downloading mod {%s}, error: %s", details.Name, "version not found")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}

	resp = modList.ListInstalledMods()
}
