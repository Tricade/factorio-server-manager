package api

import (
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

func writeBackupError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "Backup operation failed. No existing profile was overwritten. Check the manager logs."
	switch {
	case errors.Is(err, factorio.ErrProfileServerActive):
		status, message = http.StatusLocked, "Save and stop Factorio before exporting or restoring a backup."
	case errors.Is(err, factorio.ErrProfileNameConflict):
		status, message = http.StatusConflict, "A profile with that name already exists. Choose another name."
	case errors.Is(err, factorio.ErrInvalidProfile):
		status, message = http.StatusConflict, "Profile selection is no longer valid. Refresh the preview and try again."
	case errors.Is(err, factorio.ErrInvalidBackup):
		status, message = http.StatusBadRequest, "Invalid or unsupported backup. Check its format, contents and size limits. Unpacked mod folders are not supported."
	default:
		log.Printf("Backup operation failed: %v", err)
	}
	http.Error(w, message, status)
}

func ExportProfileBackupHandler(w http.ResponseWriter, r *http.Request) {
	var options factorio.ProfileBackupOptions
	if !decodeProfileRequest(w, r, &options) {
		return
	}
	path, err := factorio.ExportProfileBackup(options)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	defer os.Remove(path)
	file, err := os.Open(path)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="factorio-profiles-backup.zip"`)
	http.ServeContent(w, r, "factorio-profiles-backup.zip", time.Time{}, file)
}

func backupUpload(w http.ResponseWriter, r *http.Request) (multipart.File, int64, func(), bool) {
	cleanup := func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}
	limit := bootstrap.GetConfig().MaxUploadSize
	if limit <= 0 {
		limit = 512 << 20
	}
	if limit > 16<<30 {
		limit = 16 << 30
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		cleanup()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Backup exceeds FSM_MAX_UPLOAD. Increase the upload limit or export fewer profiles without saves/checkpoints.", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid backup upload.", http.StatusBadRequest)
		}
		return nil, 0, func() {}, false
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["backup"]) != 1 || len(r.FormValue("selection")) > maxProfileRequestSize || len(r.FormValue("profile_id")) > 64 {
		cleanup()
		http.Error(w, "Choose one backup ZIP.", http.StatusBadRequest)
		return nil, 0, func() {}, false
	}
	file, header, err := r.FormFile("backup")
	if err != nil {
		cleanup()
		http.Error(w, "Choose a backup ZIP.", http.StatusBadRequest)
		return nil, 0, func() {}, false
	}
	return file, header.Size, func() { file.Close(); cleanup() }, true
}

func PreviewProfileBackupHandler(w http.ResponseWriter, r *http.Request) {
	file, size, cleanup, ok := backupUpload(w, r)
	if !ok {
		return
	}
	defer cleanup()
	preview, err := factorio.PreviewProfileBackup(file, size)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

func ImportProfileBackupHandler(w http.ResponseWriter, r *http.Request) {
	file, size, cleanup, ok := backupUpload(w, r)
	if !ok {
		return
	}
	defer cleanup()
	var selection []factorio.ProfileBackupSelection
	if json.Unmarshal([]byte(r.FormValue("selection")), &selection) != nil {
		http.Error(w, "Invalid profile selection.", http.StatusBadRequest)
		return
	}
	state, err := factorio.ImportProfileBackup(file, size, selection)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	writeProfileStateWithStatus(w, state, http.StatusCreated)
}

func PreviewModBackupHandler(w http.ResponseWriter, r *http.Request) {
	file, size, cleanup, ok := backupUpload(w, r)
	if !ok {
		return
	}
	defer cleanup()
	preview, err := factorio.PreviewModBackup(file, size)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

func RestoreModBackupHandler(w http.ResponseWriter, r *http.Request) {
	file, size, cleanup, ok := backupUpload(w, r)
	if !ok {
		return
	}
	defer cleanup()
	preview, err := factorio.RestoreModBackup(file, size, r.FormValue("profile_id"))
	if err != nil {
		writeBackupError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}
