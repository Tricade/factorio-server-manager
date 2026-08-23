package factorio

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type Save struct {
	Name    string    `json:"name"`
	LastMod time.Time `json:"last_mod"`
	Size    int64     `json:"size"`
}

func (s *Save) String() string {
	return s.Name
}

// Lists save files in factorio/saves
func ListSaves() (saves []Save, err error) {
	config := bootstrap.GetConfig()
	saves = []Save{}
	entries, err := os.ReadDir(config.FactorioSavesDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		if !isUsableSave(info) {
			continue
		}
		saves = append(saves, Save{
			info.Name(),
			info.ModTime(),
			info.Size(),
		})
	}
	return saves, nil
}

func isUsableSave(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() == 0 {
		return false
	}
	name := strings.ToLower(info.Name())
	return strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tmp.zip")
}

// ValidateSaveArchive performs bounded structural validation before an upload
// can replace a playable save. Factorio saves contain level.dat or
// level-init.dat below their world directory.
func ValidateSaveArchive(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !isUsableSave(info) {
		return errors.New("save is not a non-empty regular .zip file")
	}
	return verifyFactorioSaveZip(path)
}

func verifyFactorioSaveZip(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() == 0 {
		return errors.New("save archive is not a non-empty regular file")
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		name := entry.FileInfo().Name()
		if entry.FileInfo().IsDir() || (name != "level.dat" && name != "level-init.dat") || entry.UncompressedSize64 == 0 {
			continue
		}
		contents, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", name, err)
		}
		var first [1]byte
		_, readErr := io.ReadFull(contents, first[:])
		closeErr := contents.Close()
		if readErr != nil {
			return fmt.Errorf("read %s: %w", name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", name, closeErr)
		}
		return nil
	}
	return errors.New("save archive does not contain a non-empty level.dat or level-init.dat")
}

// ActivateSaveUpload validates and atomically replaces one profile-scoped save.
// The temporary file must live in the same directory as the destination.
func ActivateSaveUpload(temporaryPath, name string) error {
	return activateSaveUploadInDirectory(bootstrap.GetConfig().FactorioSavesDir, temporaryPath, name)
}

func activateSaveUploadInDirectory(directory, temporaryPath, name string) error {
	if err := ValidatePathElement(name); err != nil {
		return fmt.Errorf("invalid save name: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(name), ".zip") || strings.HasSuffix(strings.ToLower(name), ".tmp.zip") {
		return errors.New("save name must end in .zip")
	}
	if err := ValidateSaveArchive(temporaryPath); err != nil {
		return fmt.Errorf("invalid Factorio save archive: %w", err)
	}
	directoryAbsolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	temporaryAbsolute, err := filepath.Abs(temporaryPath)
	if err != nil {
		return err
	}
	if filepath.Clean(filepath.Dir(temporaryAbsolute)) != filepath.Clean(directoryAbsolute) {
		return errors.New("temporary save upload is outside the saves directory")
	}
	destination := filepath.Join(directoryAbsolute, name)
	if err := os.Rename(temporaryPath, destination); err == nil {
		return nil
	} else if info, statErr := os.Lstat(destination); statErr != nil {
		return fmt.Errorf("activate save upload: %w", err)
	} else if info.IsDir() {
		return errors.New("save destination is a directory")
	}

	backup, err := os.CreateTemp(filepath.Dir(destination), ".save-backup-*")
	if err != nil {
		return fmt.Errorf("prepare save replacement: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return err
	}
	if err := os.Rename(destination, backupPath); err != nil {
		return fmt.Errorf("back up existing save: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		if restoreErr := os.Rename(backupPath, destination); restoreErr != nil {
			return fmt.Errorf("activate save upload: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("activate save upload: %w", err)
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove replaced save: %w", err)
	}
	return nil
}

func FindSave(name string) (*Save, error) {
	if err := ValidatePathElement(name); err != nil {
		return nil, fmt.Errorf("invalid save name: %w", err)
	}
	saves, err := ListSaves()
	if err != nil {
		return nil, fmt.Errorf("error listing saves: %v", err)
	}

	for _, save := range saves {
		if save.Name == name {
			return &save, nil
		}
	}

	return nil, errors.New("save not found")
}

func (s *Save) Remove() error {
	if err := ValidatePathElement(s.Name); err != nil {
		return fmt.Errorf("invalid save name: %w", err)
	}
	config := bootstrap.GetConfig()
	return os.Remove(filepath.Join(config.FactorioSavesDir, s.Name))
}

// Create savefiles for Factorio
func CreateSave(filePath string) (string, error) {
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		log.Printf("Error in creating Factorio save: %s", err)
		return "", err
	}

	args := []string{"--create", filePath}
	config := bootstrap.GetConfig()
	cmdOutput, err := exec.Command(config.FactorioBinary, args...).Output()
	if err != nil {
		log.Printf("Error in creating Factorio save: %s", err)
		log.Println(string(cmdOutput))
		return "", err
	}

	result := string(cmdOutput)

	return result, nil
}

func GetLatestSave() (save Save, err error) {
	saves, err := ListSaves()
	if err != nil {
		return Save{}, err
	}
	if len(saves) == 0 {
		return Save{}, errors.New("no usable save file found")
	}

	latest := saves[0]
	for _, candidate := range saves[1:] {
		if latest.LastMod.Before(candidate.LastMod) {
			latest = candidate
		}
	}
	return latest, nil
}
