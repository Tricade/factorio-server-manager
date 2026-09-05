package factorio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	maximumSaveModImportItems  = 256
	maximumBuiltInModInfoBytes = 1 * 1024 * 1024
)

type saveModPortalInstaller func(*Mods, Mod) error

const (
	saveModImportSkipReleaseUnavailable      = "release-unavailable"
	saveModImportSkipArchiveIdentityMismatch = "archive-identity-mismatch"
)

type SaveModImportSkipped struct {
	Name    string  `json:"name"`
	Version Version `json:"version"`
	Reason  string  `json:"reason"`
}

type SaveModImportResult struct {
	ModsResult []ModsResult           `json:"mods"`
	Skipped    []SaveModImportSkipped `json:"skipped"`
}

// ImportModsFromSave replaces the active profile's mod set with the enabled
// mods recorded in a save. The replacement is staged below the mounted mods
// directory and becomes visible only after every available Mod Portal archive
// and built-in feature has been validated. Permanently unavailable releases
// and mismatched portal archives are omitted and reported in the result.
func ImportModsFromSave(saveName string) (SaveModImportResult, error) {
	profileDataGate.RLock()
	defer profileDataGate.RUnlock()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if GetFactorioServer().IsBusy() {
		return SaveModImportResult{}, ErrServerActive
	}

	config := bootstrap.GetConfig()
	save, err := FindSave(saveName)
	if err != nil {
		return SaveModImportResult{}, err
	}
	saveName = filepath.Base(save.Name)
	savePath := filepath.Join(config.FactorioSavesDir, saveName)
	var credentials Credentials
	authenticated, err := credentials.Load()
	if err != nil {
		return SaveModImportResult{}, fmt.Errorf("load Factorio Mod Portal credentials: %w", err)
	}
	if !authenticated {
		return SaveModImportResult{}, errors.New("Factorio Mod Portal authentication is required to import mods from a save")
	}

	// Keep the executable and its built-in data stable while Factorio inspects
	// the isolated save and the manager validates the requested built-ins.
	factorioProgramFilesGate.RLock()
	defer factorioProgramFilesGate.RUnlock()
	requested, err := discoverSaveModsWithFactorio(savePath, config.FactorioDir, credentials, runSaveModSync)
	if err != nil {
		return SaveModImportResult{}, err
	}
	return replaceModsFromSave(config.FactorioDir, config.FactorioModsDir, requested, installSaveModFromPortal)
}

func replaceModsFromSave(factorioDirectory, destination string, requested []Mod, installPortal saveModPortalInstaller) (SaveModImportResult, error) {
	requested, err := validateSaveModImport(requested)
	if err != nil {
		return SaveModImportResult{}, err
	}
	if installPortal == nil {
		return SaveModImportResult{}, errors.New("save mod portal installer is not configured")
	}
	// Save import selects a mod set; it must not silently replace the active
	// profile's independent startup/runtime setting values. Settings for newly
	// imported mods remain absent and therefore use Factorio's defaults.
	existingSettings, err := readBoundedRegularFile(filepath.Join(destination, "mod-settings.dat"), maximumModSettingsFileBytes, true)
	if err != nil {
		return SaveModImportResult{}, fmt.Errorf("preserve profile mod settings: %w", err)
	}

	swap, err := newDirectoryEntrySwap(destination, ".mods-save-import-staging-", ".mods-save-import-backup-")
	if err != nil {
		return SaveModImportResult{}, fmt.Errorf("stage save mod import: %w", err)
	}
	defer swap.cleanup()

	staged, err := NewMods(swap.staging)
	if err != nil {
		return SaveModImportResult{}, fmt.Errorf("initialize staged save mod import: %w", err)
	}
	if len(existingSettings) > 0 {
		if err := os.WriteFile(filepath.Join(swap.staging, "mod-settings.dat"), existingSettings, 0600); err != nil {
			return SaveModImportResult{}, fmt.Errorf("stage profile mod settings: %w", err)
		}
	}
	enabledSpaceAgeFeatures := make([]string, 0, len(spaceAgeFeatureMods))
	skipped := make([]SaveModImportSkipped, 0)
	for _, mod := range requested {
		if mod.Name == "base" || mod.Name == "core" {
			continue
		}
		builtIn, err := installedBuiltInMod(factorioDirectory, mod.Name)
		if err != nil {
			return SaveModImportResult{}, err
		}
		if builtIn {
			if err := staged.ModSimpleList.SetModEnabled(mod.Name, true); err != nil {
				return SaveModImportResult{}, fmt.Errorf("enable built-in mod %s: %w", mod.Name, err)
			}
			if isSpaceAgeFeatureMod(mod.Name) {
				enabledSpaceAgeFeatures = append(enabledSpaceAgeFeatures, mod.Name)
			}
			continue
		}
		if isSpaceAgeFeatureMod(mod.Name) {
			return SaveModImportResult{}, fmt.Errorf("Space Age feature %s is not available in this Factorio installation", mod.Name)
		}
		if err := installPortal(&staged, mod); err != nil {
			reason, canSkip := saveModImportSkipReason(err)
			if !canSkip {
				return SaveModImportResult{}, fmt.Errorf("stage mod %s %s: %w", mod.Name, mod.Version.String(), err)
			}
			log.Printf("Skipping save-required mod %s %s during import: %v", mod.Name, mod.Version.String(), err)
			skipped = append(skipped, SaveModImportSkipped{Name: mod.Name, Version: mod.Version, Reason: reason})
			continue
		}
	}

	// The synchronized list retains the enabled built-ins required by the save.
	// Record every expansion feature explicitly so importing a base-game save
	// cannot inherit Space Age through Factorio's built-in defaults.
	if err := setBuiltInFeatureMods(swap.staging, enabledSpaceAgeFeatures); err != nil {
		return SaveModImportResult{}, fmt.Errorf("stage imported game mode: %w", err)
	}
	validated, err := NewMods(swap.staging)
	if err != nil {
		return SaveModImportResult{}, fmt.Errorf("validate staged save mod import: %w", err)
	}
	installed := validated.ListInstalledMods()
	if err := swap.activate(); err != nil {
		return SaveModImportResult{}, fmt.Errorf("activate staged save mod import: %w", err)
	}
	if err := swap.commit(); err != nil {
		log.Printf("Imported save mods are active but transactional backup cleanup failed: %v", err)
	}
	return SaveModImportResult{ModsResult: installed.ModsResult, Skipped: skipped}, nil
}

func saveModImportSkipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, errModPortalReleaseUnavailable):
		return saveModImportSkipReleaseUnavailable, true
	case errors.Is(err, errModArchiveIdentityMismatch):
		return saveModImportSkipArchiveIdentityMismatch, true
	default:
		return "", false
	}
}

func validateSaveModImport(requested []Mod) ([]Mod, error) {
	if len(requested) == 0 {
		return nil, errors.New("save does not contain a usable mod list")
	}
	if len(requested) > maximumSaveModImportItems {
		return nil, fmt.Errorf("save contains more than %d mods", maximumSaveModImportItems)
	}

	validated := make([]Mod, 0, len(requested))
	seen := make(map[string]Version, len(requested))
	hasBase := false
	for _, mod := range requested {
		if len(mod.Name) > 255 {
			return nil, errors.New("save contains a mod name longer than 255 bytes")
		}
		if err := ValidatePathElement(mod.Name); err != nil {
			return nil, fmt.Errorf("invalid save mod name: %w", err)
		}
		if previous, exists := seen[mod.Name]; exists {
			if !previous.Equals(mod.Version) {
				return nil, fmt.Errorf("save contains conflicting versions of mod %s", mod.Name)
			}
			continue
		}
		seen[mod.Name] = mod.Version
		validated = append(validated, mod)
		if mod.Name == "base" {
			hasBase = true
		}
	}
	if !hasBase {
		return nil, errors.New("save mod list does not contain the base mod")
	}
	return validated, nil
}

func installedBuiltInMod(factorioDirectory, name string) (bool, error) {
	infoPath := filepath.Join(factorioDirectory, "data", name, "info.json")
	info, err := os.Lstat(infoPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect built-in mod %s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("built-in mod %s has an invalid info.json", name)
	}
	file, err := os.Open(infoPath)
	if err != nil {
		return false, fmt.Errorf("open built-in mod %s: %w", name, err)
	}
	defer file.Close()
	var contents bytes.Buffer
	if err := copyWithHardLimit(&contents, file, maximumBuiltInModInfoBytes); err != nil {
		return false, fmt.Errorf("read built-in mod %s: %w", name, err)
	}
	var metadata builtInModInfo
	if err := json.Unmarshal(contents.Bytes(), &metadata); err != nil {
		return false, fmt.Errorf("decode built-in mod %s: %w", name, err)
	}
	if metadata.Name != name {
		return false, fmt.Errorf("built-in mod directory %s identifies itself as %q", name, metadata.Name)
	}
	return true, nil
}

func isSpaceAgeFeatureMod(name string) bool {
	for _, feature := range spaceAgeFeatureMods {
		if name == feature {
			return true
		}
	}
	return false
}

func installSaveModFromPortal(mods *Mods, requested Mod) error {
	details, err, statusCode := ModPortalModDetails(requested.Name)
	if statusCode == http.StatusNotFound || statusCode == http.StatusGone {
		return fmt.Errorf("%w: Mod Portal metadata returned HTTP %d", errModPortalReleaseUnavailable, statusCode)
	}
	if err != nil {
		return fmt.Errorf("load Mod Portal metadata: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("load Mod Portal metadata: HTTP %d", statusCode)
	}
	for _, release := range details.Releases {
		if !release.Version.Equals(requested.Version) {
			continue
		}
		if err := mods.DownloadMod(release.DownloadURL, release.FileName, details.Name); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("%w: version %s is not listed on the Mod Portal", errModPortalReleaseUnavailable, requested.Version.String())
}
