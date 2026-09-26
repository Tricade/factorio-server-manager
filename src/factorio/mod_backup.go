package factorio

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type BackupMod struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	FactorioVersion string `json:"factorio_version"`
	Enabled         bool   `json:"enabled"`
}

type ModBackupPreview struct {
	Mods               []BackupMod `json:"mods"`
	GameMode           GameMode    `json:"game_mode"`
	EnabledBuiltInMods []string    `json:"enabled_built_in_mods"`
	HasSettings        bool        `json:"has_settings"`
}

func validateBackupMods(root string) (ModBackupPreview, error) {
	result := ModBackupPreview{Mods: []BackupMod{}, EnabledBuiltInMods: []string{}, GameMode: GameModeFactorio}
	entries, err := os.ReadDir(root)
	if err != nil {
		return result, err
	}
	installed := map[string]bool{"base": true, "quality": true, "elevated-rails": true, "space-age": true}
	versions := map[string]bool{}
	for _, entry := range entries {
		if !backupModFile(entry.Name()) || entry.IsDir() {
			return result, ErrInvalidBackup
		}
		path := filepath.Join(root, entry.Name())
		if entry.Name() == "mod-settings.dat" {
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > backupMaxSettingsBytes {
				return result, ErrInvalidBackup
			}
			result.HasSettings = true
		}
		if !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		reader, err := zip.OpenReader(path)
		if err != nil {
			return result, ErrInvalidBackup
		}
		if err := inspectBackupZip(&reader.Reader, nil); err != nil {
			reader.Close()
			return result, err
		}
		// Read info.json ourselves with a bound and CRC verification. Never
		// execute mod code while previewing/restoring a backup.
		var info ModInfo
		infoCount := 0
		for _, file := range reader.File {
			if len(strings.Split(file.Name, "/")) == 2 && strings.HasSuffix(file.Name, "/info.json") {
				infoCount++
				if file.UncompressedSize64 > backupMaxJSONBytes {
					reader.Close()
					return result, ErrInvalidBackup
				}
				input, openErr := file.Open()
				if openErr != nil {
					reader.Close()
					return result, ErrInvalidBackup
				}
				data, readErr := io.ReadAll(io.LimitReader(input, backupMaxJSONBytes+1))
				input.Close()
				if readErr != nil || len(data) > backupMaxJSONBytes || json.Unmarshal(data, &info) != nil {
					reader.Close()
					return result, ErrInvalidBackup
				}
			}
		}
		if infoCount != 1 {
			reader.Close()
			return result, ErrInvalidBackup
		}
		reader.Close()
		if ValidatePathElement(info.Name) != nil || ValidatePathElement(info.Version) != nil || info.FactorioVersion == NilVersion {
			return result, ErrInvalidBackup
		}
		if info.Name == "base" || info.Name == "quality" || info.Name == "elevated-rails" || info.Name == "space-age" || versions[info.Name+"@"+info.Version] {
			return result, ErrInvalidBackup
		}
		versions[info.Name+"@"+info.Version] = true
		installed[info.Name] = true
		result.Mods = append(result.Mods, BackupMod{Name: info.Name, Version: info.Version, FactorioVersion: info.FactorioVersion.FactorioLine()})
	}
	var list ModSimpleList
	path := filepath.Join(root, "mod-list.json")
	if err := readBackupJSON(path, &list); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		// Older Download all archives may not contain a mod-list.json yet.
		list.Mods = []ModSimple{{Name: "base", Enabled: true}}
		names := []string{}
		for name := range installed {
			if name != "base" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			list.Mods = append(list.Mods, ModSimple{Name: name, Enabled: name != "quality" && name != "elevated-rails" && name != "space-age"})
		}
		if err := writeBackupJSON(path, list); err != nil {
			return result, err
		}
	}
	enabled := map[string]bool{}
	seen := map[string]bool{}
	for _, mod := range list.Mods {
		if ValidatePathElement(mod.Name) != nil || seen[mod.Name] || (mod.Enabled && !installed[mod.Name]) {
			return result, ErrInvalidBackup
		}
		seen[mod.Name] = true
		enabled[mod.Name] = mod.Enabled
	}
	if !enabled["base"] {
		return result, ErrInvalidBackup
	}
	for index := range result.Mods {
		result.Mods[index].Enabled = enabled[result.Mods[index].Name]
	}
	for _, name := range spaceAgeFeatureMods {
		if enabled[name] {
			result.EnabledBuiltInMods = append(result.EnabledBuiltInMods, name)
		}
	}
	if len(result.EnabledBuiltInMods) == len(spaceAgeFeatureMods) {
		result.GameMode = GameModeSpaceAge
	} else if len(result.EnabledBuiltInMods) > 0 {
		result.GameMode = GameModeCustom
	}
	// Add only missing DLC defaults. Preserve existing/unknown mod-list fields
	// (including future per-mod metadata) instead of round-tripping a narrow struct.
	var missing []ModSimple
	for _, name := range spaceAgeFeatureMods {
		if !seen[name] {
			missing = append(missing, ModSimple{Name: name, Enabled: false})
		}
	}
	if len(missing) > 0 {
		var raw map[string]json.RawMessage
		if err := readBackupJSON(path, &raw); err != nil {
			return result, err
		}
		var rawMods []json.RawMessage
		if json.Unmarshal(raw["mods"], &rawMods) != nil {
			return result, ErrInvalidBackup
		}
		for _, mod := range missing {
			data, err := json.Marshal(mod)
			if err != nil {
				return result, err
			}
			rawMods = append(rawMods, data)
		}
		raw["mods"], err = json.Marshal(rawMods)
		if err != nil {
			return result, err
		}
		if err := writeBackupJSON(path, raw); err != nil {
			return result, err
		}
	}
	return result, nil
}

func stageModBackup(input io.ReaderAt, size int64) (ModBackupPreview, string, error) {
	reader, err := zip.NewReader(input, size)
	if err != nil {
		return ModBackupPreview{}, "", ErrInvalidBackup
	}
	staging, err := os.MkdirTemp("", "fsm-mod-restore-")
	if err != nil {
		return ModBackupPreview{}, "", err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(staging)
		}
	}()
	// Legacy Download all stores nested mod ZIPs plus the two configuration
	// files at the root. It is deliberately distinct from a single mod upload.
	if err := extractBackup(reader, staging, func(name string) bool { return !strings.Contains(name, "/") && backupModFile(name) }); err != nil {
		return ModBackupPreview{}, "", err
	}
	preview, err := validateBackupMods(staging)
	if err != nil {
		return ModBackupPreview{}, "", err
	}
	success = true
	return preview, staging, nil
}

func PreviewModBackup(input io.ReaderAt, size int64) (ModBackupPreview, error) {
	preview, staging, err := stageModBackup(input, size)
	if err == nil {
		os.RemoveAll(staging)
	}
	return preview, err
}

func RestoreModBackup(input io.ReaderAt, size int64, expectedProfileID string) (ModBackupPreview, error) {
	preview, staging, err := stageModBackup(input, size)
	if err != nil {
		return preview, err
	}
	defer os.RemoveAll(staging)
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if err := ensureProfileServerStopped(); err != nil {
		return preview, err
	}
	profileMutex.Lock()
	manifest, manifestErr := loadProfileManifest()
	profileMutex.Unlock()
	if manifestErr != nil {
		return preview, manifestErr
	}
	if expectedProfileID == "" || expectedProfileID != manifest.ActiveProfileID {
		return preview, ErrInvalidProfile
	}
	swap, err := prepareProfileDirectorySwap(staging, profileActiveDirectories()["mods"])
	if err != nil {
		return preview, err
	}
	defer swap.cleanup()
	if err := swap.activate(); err != nil {
		return preview, err
	}
	// Validation already completed before activation. Cleanup failure does not
	// turn a committed restore into a misleading retry/second replacement.
	if err := swap.commit(); err != nil {
		log.Print("Mod backup restored; temporary directory cleanup needs attention")
	}
	return preview, nil
}
