package api

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

func ListSaves(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	latestParam := r.URL.Query().Get("latest")

	var withLatest bool

	if latestParam != "" {
		var err error
		withLatest, err = strconv.ParseBool(latestParam)
		if err != nil {
			resp = fmt.Sprintf("Error parsing latestParam: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	savesList, err := factorio.ListSaves()
	if err != nil {
		resp = fmt.Sprintf("Error listing save files: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// get actual latest and add name
	// but only if requested
	if withLatest && len(savesList) != 0 {
		latestSave, err := factorio.GetLatestSave()
		if err != nil {
			resp = fmt.Sprintf("Error getting latest save: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		latestSave.Name = fmt.Sprintf("Load Latest (%s)", latestSave.Name)
		savesList = append(savesList, latestSave)
	}

	resp = savesList
}

func DLSave(w http.ResponseWriter, r *http.Request) {
	config := bootstrap.GetConfig()
	save, err := factorio.FindSave(mux.Vars(r)["save"])
	if err != nil {
		http.Error(w, "save not found", http.StatusNotFound)
		return
	}
	saveName := filepath.Join(config.FactorioSavesDir, save.Name)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": save.Name}))
	log.Printf("%s downloading: %s", r.Host, saveName)

	http.ServeFile(w, r, saveName)
}

func UploadSave(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	log.Println("Uploading save file")

	config := bootstrap.GetConfig()
	if config.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maximumMultipartRequestSize(config.MaxUploadSize))
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			resp = uploadLimitMessage("Save file", config.MaxUploadSize)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			resp = fmt.Sprintf("Unable to parse save upload: %s", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		log.Println(resp)
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["savefile"]
	if len(files) != 1 {
		resp = "Exactly one save file must be provided"
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, saveFile := range files {
		if config.MaxUploadSize > 0 && saveFile.Size > config.MaxUploadSize {
			resp = uploadLimitMessage("Save file", config.MaxUploadSize)
			log.Println(resp)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if err := factorio.ValidatePathElement(saveFile.Filename); err != nil {
			resp = fmt.Sprintf("Invalid save filename: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ext := filepath.Ext(saveFile.Filename)
		if !strings.EqualFold(ext, ".zip") || strings.HasSuffix(strings.ToLower(saveFile.Filename), ".tmp.zip") {
			// Only zip-files allowed
			resp = fmt.Sprintf("Fileformat {%s} is not allowed", ext)
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		file, err := saveFile.Open()
		if err != nil {
			resp = fmt.Sprintf("Error opening uploaded saveFile: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer file.Close()

		temporary, err := os.CreateTemp(config.FactorioSavesDir, ".save-upload-*.zip")
		if err != nil {
			resp = fmt.Sprintf("Error creating temporary save upload: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)

		_, err = io.Copy(temporary, file)
		if err != nil {
			temporary.Close()
			resp = fmt.Sprintf("Error copying uploaded save to disk: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := temporary.Sync(); err != nil {
			temporary.Close()
			resp = fmt.Sprintf("Error syncing uploaded save: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := temporary.Close(); err != nil {
			resp = fmt.Sprintf("Error closing uploaded save: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := os.Chmod(temporaryPath, 0644); err != nil {
			resp = fmt.Sprintf("Error setting save permissions: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := factorio.ActivateSaveUpload(temporaryPath, saveFile.Filename); err != nil {
			resp = fmt.Sprintf("Error activating uploaded save: %s", err)
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
	}

	resp = "Uploading files successful"
}

func maximumMultipartRequestSize(maximumFileBytes int64) int64 {
	const maximumInt64 = int64(1<<63 - 1)
	if maximumFileBytes > maximumInt64-multipartRequestOverheadBytes {
		return maximumInt64
	}
	return maximumFileBytes + multipartRequestOverheadBytes
}

func uploadLimitMessage(subject string, maximumFileBytes int64) string {
	if maximumFileBytes > 0 && maximumFileBytes%(1024*1024) == 0 {
		return fmt.Sprintf("%s exceeds the configured upload limit of %d MiB", subject, maximumFileBytes/(1024*1024))
	}
	return fmt.Sprintf("%s exceeds the configured upload limit of %d bytes", subject, maximumFileBytes)
}

// Deletes provided save

func RemoveSave(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	vars := mux.Vars(r)
	name := vars["save"]

	save, err := factorio.FindSave(name)
	if err != nil {
		resp = fmt.Sprintf("Error finding save {%s}: %s", name, err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = save.Remove()
	if err != nil {
		resp = fmt.Sprintf("Error removing save {%s}: %s", name, err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// save was removed
	resp = fmt.Sprintf("Removed save: %s", save.Name)
}

// Launches Factorio server binary with --create flag to create save
// Url must include save name for creation of savefile

func CreateSaveHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	vars := mux.Vars(r)
	saveName := vars["save"]

	if saveName == "" {
		resp = "Error creating save: no save name provided"
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if filepath.Ext(saveName) == "" {
		saveName += ".zip"
	}
	if err := factorio.ValidatePathElement(saveName); err != nil || !strings.EqualFold(filepath.Ext(saveName), ".zip") {
		resp = "Error creating save: invalid save name"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	config := bootstrap.GetConfig()
	saveFile := filepath.Join(config.FactorioSavesDir, saveName)
	_, err = factorio.CreateSave(saveFile)
	if err != nil {
		resp = fmt.Sprintf("Error creating save {%s}: %s", saveName, err)
		log.Println(resp)
		status := http.StatusInternalServerError
		if errors.Is(err, factorio.ErrSaveAlreadyExists) || errors.Is(err, factorio.ErrWorldGenerationBusy) {
			status = http.StatusConflict
		} else if errors.Is(err, factorio.ErrFactorioMustBeStopped) {
			status = http.StatusLocked
		}
		w.WriteHeader(status)
		return
	}

	resp = fmt.Sprintf("Save %s created successfully.", saveName)
}

// LogTail returns last lines of the factorio-current.log file
