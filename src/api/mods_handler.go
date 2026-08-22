package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/OpenFactorioServerManager/factorio-server-manager/lockfile"
)

func CreateNewMods(w http.ResponseWriter) (modList factorio.Mods, resp interface{}, err error) {
	config := bootstrap.GetConfig()
	modList, err = factorio.NewMods(config.FactorioModsDir)
	if err != nil {
		resp = fmt.Sprintf("Error creating mods object: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
	}
	return
}

func ReadFromRequestBody(w http.ResponseWriter, r *http.Request, data interface{}) (resp interface{}, err error) {
	//Get Data out of the request
	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, data)
	if err != nil {
		resp = fmt.Sprintf("Error unmarshalling requested struct JSON: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	return
}

// Returns JSON response of all mods installed in factorio/mods
func ListInstalledModsHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	modList, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	resp = modList.ListInstalledMods().ModsResult
}

func ModToggleHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var data struct {
		Name string `json:"name"`
	}

	resp, err = ReadFromRequestBody(w, r, &data)
	if err != nil {
		return
	}

	mods, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	err, resp = mods.ModSimpleList.ToggleMod(data.Name)
	if err != nil {
		resp = fmt.Sprintf("Error in toggling mod in simple list: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func ModDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	var data struct {
		Name string `json:"name"`
	}

	// Get Data out of the request
	resp, err = ReadFromRequestBody(w, r, &data)
	if err != nil {
		return
	}

	modList, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	err = modList.DeleteMod(data.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("Error in deleting mod {%s}: %s", data.Name, err)
		log.Println(resp)
		return
	}

	resp = data.Name
}

func ModDeleteAllHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	//delete mods folder
	err = factorio.DeleteAllMods()
	if err != nil {
		resp = fmt.Sprintf("Error deleting all mods: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = nil
}

func ModUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	//Get Data out of the request
	var modData struct {
		Name        string `json:"modName"`
		DownloadUrl string `json:"downloadUrl"`
		Filename    string `json:"fileName"`
	}

	resp, err = ReadFromRequestBody(w, r, &modData)
	if err != nil {
		return
	}

	mods, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	err = mods.UpdateMod(modData.Name, modData.DownloadUrl, modData.Filename)
	if err != nil {
		resp = fmt.Sprintf("Error updating mod {%s}: %s", modData.Name, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	installedMods := mods.ListInstalledMods().ModsResult
	for _, mod := range installedMods {
		if mod.Name == modData.Name {
			resp = mod
			return
		}
	}

	resp = fmt.Sprintf(`Could not find mod %s`, modData.Name)
	log.Println(resp)
	w.WriteHeader(http.StatusNotFound)
	return
}

func ModUploadHandler(w http.ResponseWriter, r *http.Request) {
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
		resp = fmt.Sprintf("error parsing mod upload: %s", err)
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

	mods, resp, err := CreateNewMods(w)
	if err != nil {
		return
	}

	// if the file is a zip file, we handle it as mod
	// if the file is mod-settings.dat or mod-list.json, we just replace the
	if filepath.Ext(fileHeader.Filename) == ".zip" {
		err = mods.UploadMod(formFile, fileHeader)
		if err != nil {
			resp = fmt.Sprintf("error saving file to mods: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else if fileHeader.Filename == "mod-settings.dat" || fileHeader.Filename == "mod-list.json" {
		modsDir := filepath.Join(config.FactorioModsDir, fileHeader.Filename)
		file, err := os.Create(modsDir)
		if err != nil {
			resp = fmt.Sprintf("error creating %s: %s", fileHeader.Filename, err)
			log.Println(resp)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, err = io.Copy(file, formFile)
		if err != nil {
			resp = fmt.Sprintf("error saving %s: %s", fileHeader.Filename, err)
			log.Println(resp)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		resp = fmt.Sprintf("The uploaded file wasn't a zip-file, a mod-settings. dat or a mod-info.json")
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resp = mods.ListInstalledMods()
}

func ModDownloadHandler(w http.ResponseWriter, r *http.Request) {
	var err error

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()
	config := bootstrap.GetConfig()
	//iterate over folder and create everything in the zip
	err = filepath.Walk(config.FactorioModsDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() == false {
			//Lock the file, that we are want to read
			err := factorio.FileLock.RLock(path)
			if err != nil {
				log.Printf("error locking file for reading, something else has locked it")
				return err
			}
			defer factorio.FileLock.RUnlock(path)

			writer, err := zipWriter.Create(info.Name())
			if err != nil {
				log.Printf("error on creating new file inside zip: %s", err)
				return err
			}

			file, err := os.Open(path)
			if err != nil {
				log.Printf("error on opening modfile: %s", err)
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			if err != nil {
				log.Printf("error on copying file into zip: %s", err)
				return err
			}

			err = file.Close()
			if err != nil {
				log.Printf("error closing file: %s", err)
				return err
			}
		}

		return nil
	})
	if err == lockfile.ErrorAlreadyLocked {
		w.WriteHeader(http.StatusLocked)
		return
	}
	if err != nil {
		log.Printf("error on walking over the mods: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	writerHeader := w.Header()
	writerHeader.Set("Content-Type", "application/zip;charset=UTF-8")
	writerHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", "all_installed_mods.zip"))
}

// LoadModsFromSaveHandler returns JSON response with the found mods
func LoadModsFromSaveHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	//Get Data out of the request
	var saveFileStruct struct {
		Name string `json:"saveFile"`
	}

	resp, err = ReadFromRequestBody(w, r, &saveFileStruct)
	if err != nil {
		return
	}

	config := bootstrap.GetConfig()
	if err := factorio.ValidatePathElement(saveFileStruct.Name); err != nil {
		resp = fmt.Sprintf("invalid save name: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	path := filepath.Join(config.FactorioSavesDir, saveFileStruct.Name)

	f, err := factorio.OpenArchiveFile(path, "level.dat", "level-init.dat")
	if err != nil {
		resp = fmt.Sprintf("cannot open save level file: %v", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	var header factorio.SaveHeader
	err = header.DecodeFrom(f)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		resp = fmt.Sprintf("cannot read save header: %v", err)
		log.Println(resp)
		return
	}

	resp = header
}
