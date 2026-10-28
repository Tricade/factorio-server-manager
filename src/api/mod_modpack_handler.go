package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

func CheckModPackExists(modPackMap factorio.ModPackMap, modPackName string, w http.ResponseWriter) (resp interface{}, err error) {
	exists := modPackMap.CheckModPackExists(modPackName)
	if !exists {
		resp = fmt.Sprintf("requested modPack {%s} does not exist", modPackName)
		log.Println(resp)
		w.WriteHeader(http.StatusNotFound)
		err = errors.New("requested modPack does not exist")
	}
	return
}

func CreateNewModPackMap(w http.ResponseWriter) (modPackMap factorio.ModPackMap, resp interface{}, err error) {
	modPackMap, err = factorio.NewModPackMap()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error creating modpackmap aka. list of all modpacks files : %s", err)
		log.Println(resp)
	}
	return
}

func ReadModPackRequest(w http.ResponseWriter, r *http.Request) (packMap factorio.ModPackMap, modPackName string, resp interface{}, err error) {
	vars := mux.Vars(r)
	modPackName = vars["modpack"]

	packMap, resp, err = CreateNewModPackMap(w)
	if err != nil {
		return
	}

	resp, err = CheckModPackExists(packMap, modPackName, w)

	return
}

//////////////////////
// Mod Pack Handler //
//////////////////////

func ModPackListHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	modPackMap, resp, err := CreateNewModPackMap(w)
	if err != nil {
		return
	}

	resp = modPackMap.ListInstalledModPacks()
}

func ModPackCreateHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var modPackStruct struct {
		Name string `json:"name"`
	}
	resp, err = ReadFromRequestBody(w, r, &modPackStruct)
	if err != nil {
		return
	}

	modPackMap, resp, err := CreateNewModPackMap(w)
	if err != nil {
		return
	}

	err = modPackMap.CreateModPack(modPackStruct.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error creating modpack file: %s", err)
		log.Println(resp)
		return
	}

	resp = modPackMap.ListInstalledModPacks()
}

func ModPackDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var modPackMap factorio.ModPackMap
	var modPackName string
	modPackMap, modPackName, resp, err = ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	err = modPackMap.DeleteModPack(modPackName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error deleting modpack file: %s", err)
		log.Println(resp)
		return
	}

	resp = modPackName
}

func ModPackDownloadHandler(w http.ResponseWriter, r *http.Request) {
	_, modPackName, resp, err := ReadModPackRequest(w, r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json;charset=UTF-8")
		WriteResponse(w, resp)
		return
	}
	config := bootstrap.GetConfig()
	if err := serveDirectoryZip(w, filepath.Join(config.FactorioModPackDir, modPackName), modPackName+".zip", false); err != nil {
		log.Printf("error on walking over the modpack: %s", err)
		http.Error(w, "Unable to build mod pack archive", http.StatusInternalServerError)
		return
	}
}

func ModPackLoadHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var modPackMap factorio.ModPackMap
	var modPackName string
	modPackMap, modPackName, resp, err = ReadModPackRequest(w, r)
	if err != nil {
		return
	}

	err = modPackMap[modPackName].LoadModPack()
	if err != nil {
		if errors.Is(err, factorio.ErrServerActive) {
			w.WriteHeader(http.StatusLocked)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		resp = fmt.Sprintf("Error loading modpack file: %s", err)
		log.Println(resp)
		return
	}

	resp = modPackMap[modPackName].Mods.ListInstalledMods()
}
