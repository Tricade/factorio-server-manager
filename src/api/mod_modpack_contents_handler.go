package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

// ////////////////////////////////
// Mods inside Mod Pack Handler //
// ////////////////////////////////
func ModPackModListHandler(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	modPackMap, modPackName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	resp = modPackMap[modPackName].Mods.ListInstalledMods()
}

func ModPackModToggleHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	var modPackStruct struct {
		ModName string `json:"name"`
	}
	resp, err = ReadFromRequestBody(w, r, &modPackStruct)
	if err != nil {
		return
	}

	err, resp = packMap[packName].Mods.ModSimpleList.ToggleMod(modPackStruct.ModName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error toggling mod inside modPack: %s", err)
		log.Println(resp)
		return
	}
}

func ModPackModDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	var modPackStruct struct {
		Name string `json:"name"`
	}
	resp, err = ReadFromRequestBody(w, r, &modPackStruct)
	if err != nil {
		return
	}

	err = packMap[packName].Mods.DeleteMod(modPackStruct.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error deleting mod {%s} in modpack {%s}: %s", modPackStruct.Name, packName, err)
		log.Println(resp)
		return
	}

	resp = true
}

func ModPackModUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	//Get Data out of the request
	var modPackStruct struct {
		ModName     string `json:"modName"`
		DownloadUrl string `json:"downloadUrl"`
		Filename    string `json:"filename"`
	}
	resp, err = ReadFromRequestBody(w, r, &modPackStruct)
	if err != nil {
		return
	}

	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	err = packMap[packName].Mods.UpdateMod(modPackStruct.ModName, modPackStruct.DownloadUrl, modPackStruct.Filename)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error updating mod {%s} in modpack {%s}: %s", modPackStruct.ModName, packName, err)
		log.Println(resp)
		return
	}

	installedMods := packMap[packName].Mods.ListInstalledMods().ModsResult
	var found = false
	for _, mod := range installedMods {
		if mod.Name == modPackStruct.ModName {
			resp = mod
			found = true
			return
		}
	}

	if !found {
		resp = fmt.Sprintf(`Could not find mod %s`, modPackStruct.ModName)
		log.Println(resp)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

func ModPackModDeleteAllHandler(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	// Delete Modpack
	err = packMap.DeleteModPack(packName)
	if err != nil {
		resp = fmt.Sprintf("Error deleting modPackDir: %s", err)
		log.Println(resp)
		return
	}

	// recreate modPack without mods
	err = packMap.CreateEmptyModPack(packName)
	if err != nil {
		resp = fmt.Sprintf("Error recreating modPackDir: %s", err)
		log.Println(resp)
		return
	}

	resp = true
}

func ModPackModUploadHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	config := bootstrap.GetConfig()
	if config.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, config.MaxUploadSize)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		resp = fmt.Sprintf("error parsing mod-pack upload: %s", err)
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	formFile, fileHeader, err := r.FormFile("mod_file")
	if err != nil {
		resp = fmt.Sprintf("error getting uploaded file: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer formFile.Close()
	if err := factorio.ValidatePathElement(fileHeader.Filename); err != nil {
		resp = fmt.Sprintf("invalid mod filename: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	modPackMap, modPackName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	err = modPackMap[modPackName].Mods.UploadMod(formFile, fileHeader)
	if err != nil {
		resp = fmt.Sprintf("error saving file to modPack: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = modPackMap[modPackName].Mods.ListInstalledMods()
}

func ModPackModPortalInstallHandler(w http.ResponseWriter, r *http.Request) {
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
	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	modList := packMap[packName].Mods

	err = modList.DownloadMod(data.DownloadURL, data.Filename, data.ModName)
	if err != nil {
		resp = fmt.Sprintf("Error downloading a mod: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = modList.ListInstalledMods()
}

func ModPackModPortalInstallMultipleHandler(w http.ResponseWriter, r *http.Request) {
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

	packMap, packName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		return
	}
	modList := packMap[packName].Mods
	for _, datum := range data {
		details, statusCode, err := factorio.ModPortalModDetails(datum.Name)
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
