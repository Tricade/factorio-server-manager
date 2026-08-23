package api

import (
	"archive/zip"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

// serveDirectoryZip finishes the archive on disk before touching the HTTP
// response, so a walk/read failure can still return an honest error status.
func serveDirectoryZip(w http.ResponseWriter, directory, downloadName string, lockFiles bool) error {
	archive, err := os.CreateTemp("", "fsm-directory-download-*.zip")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	defer archive.Close()

	zipWriter := zip.NewWriter(archive)
	err = filepath.Walk(directory, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to archive non-regular file %s", info.Name())
		}
		if lockFiles {
			if err := factorio.FileLock.RLock(path); err != nil {
				return err
			}
			defer factorio.FileLock.RUnlock(path)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		writer, createErr := zipWriter.Create(info.Name())
		if createErr != nil {
			_ = file.Close()
			return createErr
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = zipWriter.Close()
		return err
	}
	if err := zipWriter.Close(); err != nil {
		return err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName}))
	_, err = io.Copy(w, archive)
	return err
}
