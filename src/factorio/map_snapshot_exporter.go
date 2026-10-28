package factorio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validateMapSnapshotFactorioVersion(value string) error {
	var version Version
	if err := version.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return fmt.Errorf("map snapshots require a valid Factorio version: %w", err)
	}
	minimum := Version{2, 0, 61, 0}
	if version.Less(minimum) {
		return fmt.Errorf("map snapshots require Factorio %s or newer; profile uses %s", minimum.ReleaseString(), version.ReleaseString())
	}
	return nil
}

func activeMapSnapshotProfile() (Profile, error) {
	profileMutex.Lock()
	defer profileMutex.Unlock()
	manifest, err := loadProfileManifest()
	if err != nil {
		return Profile{}, err
	}
	index := profileIndex(manifest, manifest.ActiveProfileID)
	if index < 0 {
		return Profile{}, errors.New("active profile metadata is missing")
	}
	return manifest.Profiles[index], nil
}

func findMapSnapshotSourceSave(directory, selected string, running bool) (*Save, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list map snapshot saves: %w", err)
	}
	var selectedSave *Save
	var latestSave *Save
	var latestAutosave *Save
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !isUsableSave(info) {
			continue
		}
		save := &Save{Name: info.Name(), LastMod: info.ModTime(), Size: info.Size()}
		if selected != "" && !strings.HasPrefix(selected, "Load Latest") && save.Name == selected {
			copySave := *save
			selectedSave = &copySave
		}
		if latestSave == nil || latestSave.LastMod.Before(save.LastMod) {
			copySave := *save
			latestSave = &copySave
		}
		if strings.HasPrefix(strings.ToLower(save.Name), "_autosave") && (latestAutosave == nil || latestAutosave.LastMod.Before(save.LastMod)) {
			copySave := *save
			latestAutosave = &copySave
		}
	}
	baseSave := selectedSave
	if baseSave == nil {
		baseSave = latestSave
	}
	if running && latestAutosave != nil && (baseSave == nil || baseSave.LastMod.Before(latestAutosave.LastMod)) {
		return latestAutosave, nil
	}
	if baseSave != nil {
		return baseSave, nil
	}
	return nil, errors.New("no usable save is available for a map snapshot")
}

func prepareMapSnapshotMods(source, destination, factorioVersion string, includeSpacePlatforms bool) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	enabledMods, err := readMapSnapshotEnabledMods(filepath.Join(source, "mod-list.json"))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(strings.ToLower(name), mapSnapshotExporterName) {
			continue
		}
		if !mapSnapshotModEntryRequired(name, enabledMods) {
			continue
		}
		sourcePath := filepath.Join(source, name)
		destinationPath := filepath.Join(destination, name)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symbolic link %s", name)
		}
		if info.IsDir() {
			if err := copyProfileDirectory(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse non-regular mod entry %s", name)
		}
		if strings.EqualFold(filepath.Ext(name), ".zip") {
			if err := os.Link(sourcePath, destinationPath); err == nil {
				continue
			}
		}
		if err := copyProfileFile(sourcePath, destinationPath, info); err != nil {
			return err
		}
	}
	if err := installMapSnapshotExporter(destination, factorioVersion, includeSpacePlatforms); err != nil {
		return err
	}
	return enableMapSnapshotExporter(filepath.Join(destination, "mod-list.json"))
}

func readMapSnapshotEnabledMods(path string) (map[string]struct{}, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]struct{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active mod list: %w", err)
	}
	var document struct {
		Mods []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"mods"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, fmt.Errorf("decode active mod list: %w", err)
	}
	enabled := make(map[string]struct{}, len(document.Mods))
	for _, mod := range document.Mods {
		if mod.Enabled && mod.Name != "" {
			enabled[mod.Name] = struct{}{}
		}
	}
	return enabled, nil
}

func mapSnapshotModEntryRequired(name string, enabledMods map[string]struct{}) bool {
	if name == "mod-list.json" || name == "mod-settings.dat" {
		return true
	}
	baseName := strings.TrimSuffix(name, filepath.Ext(name))
	for modName := range enabledMods {
		if baseName == modName {
			return true
		}
		prefix := modName + "_"
		if !strings.HasPrefix(baseName, prefix) {
			continue
		}
		version := strings.TrimPrefix(baseName, prefix)
		if version != "" && version[0] >= '0' && version[0] <= '9' {
			return true
		}
	}
	return false
}

func installMapSnapshotExporter(modsDirectory, factorioVersion string, includeSpacePlatforms bool) error {
	destination := filepath.Join(modsDirectory, mapSnapshotExporterName+"_"+mapSnapshotExporterVersion)
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	for _, name := range []string{"info.json", "control.lua"} {
		contents, err := mapSnapshotExporterFiles.ReadFile("map_snapshot_exporter/" + name)
		if err != nil {
			return err
		}
		if name == "info.json" {
			var info map[string]any
			if err := json.Unmarshal(contents, &info); err != nil {
				return err
			}
			compatibilityVersion, err := mapSnapshotCompatibilityVersion(factorioVersion)
			if err != nil {
				return err
			}
			info["factorio_version"] = compatibilityVersion
			contents, err = json.MarshalIndent(info, "", "  ")
			if err != nil {
				return err
			}
		} else if name == "control.lua" {
			placeholder := []byte("__FSM_INCLUDE_SPACE_PLATFORMS__")
			if bytes.Count(contents, placeholder) != 1 {
				return errors.New("map snapshot exporter platform setting placeholder is invalid")
			}
			contents = bytes.Replace(contents, placeholder, []byte(strconv.FormatBool(includeSpacePlatforms)), 1)
		}
		if err := os.WriteFile(filepath.Join(destination, name), contents, 0600); err != nil {
			return err
		}
	}
	return nil
}

func mapSnapshotCompatibilityVersion(version string) (string, error) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid Factorio version %q", version)
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return "", fmt.Errorf("invalid Factorio version %q", version)
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return "", fmt.Errorf("invalid Factorio version %q", version)
	}
	return parts[0] + "." + parts[1], nil
}

func enableMapSnapshotExporter(path string) error {
	document := map[string]json.RawMessage{}
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		document["mods"] = json.RawMessage(`[]`)
	} else if err != nil {
		return err
	} else if err := json.Unmarshal(contents, &document); err != nil {
		return fmt.Errorf("decode active mod list: %w", err)
	}
	var mods []map[string]any
	if raw := document["mods"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &mods); err != nil {
			return fmt.Errorf("decode active mods: %w", err)
		}
	}
	found := false
	for _, mod := range mods {
		name, _ := mod["name"].(string)
		if name == mapSnapshotExporterName {
			mod["enabled"] = true
			found = true
		}
	}
	if !found {
		mods = append(mods, map[string]any{"name": mapSnapshotExporterName, "enabled": true})
	}
	rawMods, err := json.Marshal(mods)
	if err != nil {
		return err
	}
	document["mods"] = rawMods
	return writeMapSnapshotJSONAtomically(path, document, 0600)
}

func readMapSnapshotExporterManifest(directory string) (mapSnapshotExporterManifest, error) {
	contents, err := readMapSnapshotFile(filepath.Join(directory, "manifest.json"), maximumMapSnapshotManifest)
	if err != nil {
		return mapSnapshotExporterManifest{}, fmt.Errorf("read map exporter manifest: %w", err)
	}
	var manifest mapSnapshotExporterManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return mapSnapshotExporterManifest{}, fmt.Errorf("decode map exporter manifest: %w", err)
	}
	if manifest.SchemaVersion != mapSnapshotSchemaVersion {
		return mapSnapshotExporterManifest{}, fmt.Errorf("unsupported map exporter schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.Surfaces) == 0 {
		return mapSnapshotExporterManifest{}, errors.New("the save has no charted surfaces for the player force")
	}
	seen := make(map[uint32]bool, len(manifest.Surfaces))
	for _, surface := range manifest.Surfaces {
		if seen[surface.Index] || surface.ChunkCount < 1 || surface.ChunkCount > maximumMapSnapshotChunks || surface.MinX > surface.MaxX || surface.MinY > surface.MaxY {
			return mapSnapshotExporterManifest{}, fmt.Errorf("invalid map exporter surface %d", surface.Index)
		}
		if _, _, _, _, err := mapSnapshotRenderTileBounds(surface); err != nil {
			return mapSnapshotExporterManifest{}, fmt.Errorf("invalid map exporter view for surface %d: %w", surface.Index, err)
		}
		expectedFile := "surface-" + strconv.FormatUint(uint64(surface.Index), 10) + ".jsonl"
		if surface.File != expectedFile {
			return mapSnapshotExporterManifest{}, fmt.Errorf("invalid map exporter file for surface %d", surface.Index)
		}
		info, err := os.Lstat(filepath.Join(directory, surface.File))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumMapSnapshotExportBytes {
			if err == nil {
				err = errors.New("surface export is invalid or too large")
			}
			return mapSnapshotExporterManifest{}, fmt.Errorf("validate surface %d export: %w", surface.Index, err)
		}
		expectedEntityFile := "surface-" + strconv.FormatUint(uint64(surface.Index), 10) + "-entities.jsonl"
		if surface.EntityFile != expectedEntityFile || surface.EntityCount < 0 || surface.EntityCount > maximumMapSnapshotEntityCount || surface.EntityTotalCount < surface.EntityCount || surface.EntityTotalCount > maximumMapSnapshotEntityTotal || surface.EntityTruncated != (surface.EntityTotalCount > surface.EntityCount) {
			return mapSnapshotExporterManifest{}, fmt.Errorf("invalid map exporter entity metadata for surface %d", surface.Index)
		}
		entityInfo, err := os.Lstat(filepath.Join(directory, surface.EntityFile))
		if err != nil || entityInfo.Mode()&os.ModeSymlink != 0 || !entityInfo.Mode().IsRegular() || entityInfo.Size() > maximumMapSnapshotEntityBytes {
			if err == nil {
				err = errors.New("surface entity export is invalid or too large")
			}
			return mapSnapshotExporterManifest{}, fmt.Errorf("validate surface %d entity export: %w", surface.Index, err)
		}
		seen[surface.Index] = true
	}
	seenPlayers := make(map[string]bool, len(manifest.Players))
	for _, player := range manifest.Players {
		if player.Name == "" || utf8.RuneCountInString(player.Name) > 200 || strings.IndexFunc(player.Name, unicode.IsControl) >= 0 || seenPlayers[player.Name] {
			return mapSnapshotExporterManifest{}, errors.New("map exporter player metadata is invalid")
		}
		seenPlayers[player.Name] = true
	}
	return manifest, nil
}

func readMapSnapshotFile(path string, maximumBytes int64) ([]byte, error) {
	file, info, err := openRegularMapSnapshotFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > maximumBytes {
		return nil, errors.New("file is not regular or exceeds the size limit")
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > maximumBytes {
		return nil, errors.New("file exceeds the size limit")
	}
	return contents, nil
}

func readMapSnapshotFileTail(path string, maximumBytes int64) ([]byte, error) {
	file, info, err := openRegularMapSnapshotFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if info.Size() > maximumBytes {
		if _, err := file.Seek(info.Size()-maximumBytes, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, maximumBytes))
}

func openRegularMapSnapshotFile(path string) (*os.File, os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, nil, errors.New("file is not regular")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !fileInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		file.Close()
		return nil, nil, errors.New("file changed while it was opened")
	}
	return file, fileInfo, nil
}
