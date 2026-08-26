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

// ImportModsFromSave replaces the active profile's mod set with the enabled
// mods recorded in a save. The complete replacement is staged below the
// mounted mods directory and becomes visible only after every required Mod
// Portal archive and built-in feature has been validated.
func ImportModsFromSave(saveName string) (ModsResultList, error) {
	profileDataGate.RLock()
	defer profileDataGate.RUnlock()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if GetFactorioServer().IsBusy() {
		return ModsResultList{}, ErrServerActive
	}

	config := bootstrap.GetConfig()
	if _, err := FindSave(saveName); err != nil {
		return ModsResultList{}, err
	}
	savePath := filepath.Join(config.FactorioSavesDir, saveName)
	var credentials Credentials
	authenticated, err := credentials.Load()
	if err != nil {
		return ModsResultList{}, fmt.Errorf("load Factorio Mod Portal credentials: %w", err)
	}
	if !authenticated {
		return ModsResultList{}, errors.New("Factorio Mod Portal authentication is required to import mods from a save")
	}

	// Keep the executable and its built-in data stable while Factorio inspects
	// the isolated save and the manager validates the requested built-ins.
	factorioProgramFilesGate.RLock()
	defer factorioProgramFilesGate.RUnlock()
	requested, err := discoverSaveModsWithFactorio(savePath, config.FactorioDir, credentials, runSaveModSync)
	if err != nil {
		return ModsResultList{}, err
	}
	return replaceModsFromSave(config.FactorioDir, config.FactorioModsDir, requested, installSaveModFromPortal)
}

func replaceModsFromSave(factorioDirectory, destination string, requested []Mod, installPortal saveModPortalInstaller) (ModsResultList, error) {
	requested, err := validateSaveModImport(requested)
	if err != nil {
		return ModsResultList{}, err
	}
	if installPortal == nil {
		return ModsResultList{}, errors.New("save mod portal installer is not configured")
	}

	swap, err := newDirectoryEntrySwap(destination, ".mods-save-import-staging-", ".mods-save-import-backup-")
	if err != nil {
		return ModsResultList{}, fmt.Errorf("stage save mod import: %w", err)
	}
	defer swap.cleanup()

	staged, err := NewMods(swap.staging)
	if err != nil {
		return ModsResultList{}, fmt.Errorf("initialize staged save mod import: %w", err)
	}
	enabledSpaceAgeFeatures := make([]string, 0, len(spaceAgeFeatureMods))
	for _, mod := range requested {
		if mod.Name == "base" || mod.Name == "core" {
			continue
		}
		builtIn, err := installedBuiltInMod(factorioDirectory, mod.Name)
		if err != nil {
			return ModsResultList{}, err
		}
		if builtIn {
			if err := staged.ModSimpleList.SetModEnabled(mod.Name, true); err != nil {
				return ModsResultList{}, fmt.Errorf("enable built-in mod %s: %w", mod.Name, err)
			}
			if isSpaceAgeFeatureMod(mod.Name) {
				enabledSpaceAgeFeatures = append(enabledSpaceAgeFeatures, mod.Name)
			}
			continue
		}
		if isSpaceAgeFeatureMod(mod.Name) {
			return ModsResultList{}, fmt.Errorf("Space Age feature %s is not available in this Factorio installation", mod.Name)
		}
		if err := installPortal(&staged, mod); err != nil {
			return ModsResultList{}, fmt.Errorf("stage mod %s %s: %w", mod.Name, mod.Version.String(), err)
		}
	}

	// The synchronized list retains the enabled built-ins required by the save.
	// Record every expansion feature explicitly so importing a base-game save
	// cannot inherit Space Age through Factorio's built-in defaults.
	if err := setBuiltInFeatureMods(swap.staging, enabledSpaceAgeFeatures); err != nil {
		return ModsResultList{}, fmt.Errorf("stage imported game mode: %w", err)
	}
	validated, err := NewMods(swap.staging)
	if err != nil {
		return ModsResultList{}, fmt.Errorf("validate staged save mod import: %w", err)
	}
	result := validated.ListInstalledMods()
	if err := swap.activate(); err != nil {
		return ModsResultList{}, fmt.Errorf("activate staged save mod import: %w", err)
	}
	if err := swap.commit(); err != nil {
		log.Printf("Imported save mods are active but transactional backup cleanup failed: %v", err)
	}
	return result, nil
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
	return fmt.Errorf("requested version is not available on the Mod Portal")
}
