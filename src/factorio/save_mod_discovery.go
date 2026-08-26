package factorio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	saveModSyncTimeout          = 5 * time.Minute
	maximumSaveModSyncListBytes = 1 * 1024 * 1024
)

type saveModSyncRunner func(time.Duration, []string) error

var runSaveModSync saveModSyncRunner = runFactorioWorldCommand

type synchronizedSaveMod struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

type synchronizedSaveModList struct {
	Mods []synchronizedSaveMod `json:"mods"`
}

// discoverSaveModsWithFactorio asks the installed Factorio executable to read
// the save's current mod state. level-init.dat describes the world at creation
// time and is therefore stale after later game-version or mod changes.
//
// Factorio receives only an isolated save copy, empty mod directory and private
// write-data directory. It can describe required community mods without
// changing the playable save or active profile; the manager still owns all
// authenticated downloads and transactional activation.
func discoverSaveModsWithFactorio(savePath, factorioDirectory string, runner saveModSyncRunner) ([]Mod, error) {
	if runner == nil {
		return nil, errors.New("Factorio save-mod inspector is not configured")
	}
	if err := ValidateSaveArchive(savePath); err != nil {
		return nil, fmt.Errorf("validate save before mod inspection: %w", err)
	}

	workDirectory, err := os.MkdirTemp("", "fsm-save-mod-sync-")
	if err != nil {
		return nil, fmt.Errorf("create save-mod inspection workspace: %w", err)
	}
	defer os.RemoveAll(workDirectory)

	saveCopy := filepath.Join(workDirectory, "source-save.zip")
	if err := copyCheckpointAtomically(savePath, saveCopy); err != nil {
		return nil, fmt.Errorf("copy save for mod inspection: %w", err)
	}
	modsDirectory := filepath.Join(workDirectory, "mods")
	writeDataDirectory := filepath.Join(workDirectory, "write-data")
	for _, directory := range []string{modsDirectory, writeDataDirectory} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return nil, fmt.Errorf("create save-mod inspection directory: %w", err)
		}
	}
	configPath := filepath.Join(workDirectory, "config.ini")
	if err := writeIsolatedFactorioConfig(configPath, factorioDirectory, writeDataDirectory); err != nil {
		return nil, err
	}

	args := []string{
		"--config", configPath,
		"--mod-directory", modsDirectory,
		"--sync-mods", saveCopy,
	}
	if err := runner(saveModSyncTimeout, args); err != nil {
		return nil, fmt.Errorf("inspect save mods with Factorio: %w", err)
	}
	return readSynchronizedSaveMods(filepath.Join(modsDirectory, "mod-list.json"), factorioDirectory)
}

func readSynchronizedSaveMods(path, factorioDirectory string) ([]Mod, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read synchronized save mod list: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 {
		return nil, errors.New("synchronized save mod list is not a non-empty regular file")
	}
	var contents bytes.Buffer
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open synchronized save mod list: %w", err)
	}
	readErr := copyWithHardLimit(&contents, file, maximumSaveModSyncListBytes)
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read synchronized save mod list: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close synchronized save mod list: %w", closeErr)
	}

	var document synchronizedSaveModList
	if err := json.Unmarshal(contents.Bytes(), &document); err != nil {
		return nil, fmt.Errorf("decode synchronized save mod list: %w", err)
	}
	if len(document.Mods) > maximumSaveModImportItems {
		return nil, fmt.Errorf("save contains more than %d mod entries", maximumSaveModImportItems)
	}

	requested := make([]Mod, 0, len(document.Mods))
	for _, synchronized := range document.Mods {
		if err := ValidatePathElement(synchronized.Name); err != nil {
			return nil, fmt.Errorf("invalid synchronized save mod name: %w", err)
		}
		version, err := parseSynchronizedSaveModVersion(synchronized.Version)
		if err != nil {
			return nil, fmt.Errorf("invalid synchronized version for mod %s: %w", synchronized.Name, err)
		}
		builtIn, err := installedBuiltInMod(factorioDirectory, synchronized.Name)
		if err != nil {
			return nil, err
		}
		if builtIn && !synchronized.Enabled {
			continue
		}
		// The inspection directory starts empty. Factorio therefore records a
		// save-required community mod as disabled when its archive is not yet
		// available. Its presence, rather than this transient flag, is the
		// requirement signal; the manager downloads and enables it below staging.
		requested = append(requested, Mod{Name: synchronized.Name, Version: version})
	}
	return validateSaveModImport(requested)
}

func parseSynchronizedSaveModVersion(value string) (Version, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(parts) < 3 || len(parts) > 4 {
		return NilVersion, fmt.Errorf("expected three or four numeric components, got %q", value)
	}
	var version Version
	if err := version.UnmarshalText([]byte(value)); err != nil {
		return NilVersion, err
	}
	return version, nil
}

func writeIsolatedFactorioConfig(path, factorioDirectory, writeDataDirectory string) error {
	for _, value := range []string{factorioDirectory, writeDataDirectory} {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("Factorio path contains a line break")
		}
	}
	readData := filepath.ToSlash(filepath.Join(factorioDirectory, "data"))
	writeData := filepath.ToSlash(writeDataDirectory)
	contents := fmt.Sprintf("[path]\nread-data=%s\nwrite-data=%s\n", readData, writeData)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		return fmt.Errorf("write isolated Factorio config: %w", err)
	}
	return nil
}
