package factorio

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	mapSnapshotSchemaVersion      = 1
	mapSnapshotSettingsFileName   = "map-snapshot-settings.json"
	mapSnapshotMetadataFileName   = "snapshot.json"
	mapSnapshotExporterName       = "fsm-map-exporter"
	defaultMapSnapshotInterval    = 60
	maximumMapSnapshotInterval    = 7 * 24 * 60
	maximumMapSnapshotDimension   = 4096
	maximumMapSnapshotChunks      = 250000
	maximumMapSnapshotExportBytes = 512 * 1024 * 1024
	maximumMapSnapshotManifest    = 4 * 1024 * 1024
	maximumMapSnapshotErrorLog    = 64 * 1024
	mapSnapshotChunkPixels        = 32
)

var (
	ErrMapSnapshotBusy            = errors.New("a map snapshot is already being generated")
	ErrMapSnapshotNotFound        = errors.New("map snapshot not found")
	ErrMapSnapshotSurfaceNotFound = errors.New("map snapshot surface not found")
	ErrInvalidMapSnapshotSettings = errors.New("invalid map snapshot settings")
)

type MapSnapshotSettings struct {
	IntervalMinutes int `json:"interval_minutes"`
}

type MapSnapshotSurface struct {
	ID          string `json:"id"`
	Index       uint32 `json:"index"`
	Name        string `json:"name"`
	SurfaceName string `json:"surface_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	ChunkCount  int    `json:"chunk_count"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	File        string `json:"-"`
}

type MapSnapshot struct {
	SchemaVersion   int                  `json:"schema_version"`
	ProfileID       string               `json:"profile_id"`
	SaveName        string               `json:"save_name"`
	SaveModifiedAt  time.Time            `json:"save_modified_at"`
	GeneratedAt     time.Time            `json:"generated_at"`
	FactorioVersion string               `json:"factorio_version"`
	GameTick        uint64               `json:"game_tick"`
	Force           string               `json:"force"`
	Surfaces        []MapSnapshotSurface `json:"surfaces"`
}

type MapSnapshotState struct {
	Settings       MapSnapshotSettings `json:"settings"`
	Running        bool                `json:"running"`
	RunningProfile string              `json:"running_profile,omitempty"`
	LastAttemptAt  *time.Time          `json:"last_attempt_at,omitempty"`
	LastError      string              `json:"last_error,omitempty"`
	Snapshot       *MapSnapshot        `json:"snapshot,omitempty"`
}

type mapSnapshotExporterManifest struct {
	SchemaVersion int                          `json:"schema_version"`
	GameTick      uint64                       `json:"game_tick"`
	GameVersion   string                       `json:"game_version"`
	Force         string                       `json:"force"`
	Surfaces      []mapSnapshotExporterSurface `json:"surfaces"`
}

func (manifest *mapSnapshotExporterManifest) UnmarshalJSON(contents []byte) error {
	var raw struct {
		SchemaVersion int             `json:"schema_version"`
		GameTick      uint64          `json:"game_tick"`
		GameVersion   string          `json:"game_version"`
		Force         string          `json:"force"`
		Surfaces      json.RawMessage `json:"surfaces"`
	}
	if err := json.Unmarshal(contents, &raw); err != nil {
		return err
	}
	manifest.SchemaVersion = raw.SchemaVersion
	manifest.GameTick = raw.GameTick
	manifest.GameVersion = raw.GameVersion
	manifest.Force = raw.Force
	trimmedSurfaces := bytes.TrimSpace(raw.Surfaces)
	if bytes.Equal(trimmedSurfaces, []byte("{}")) || bytes.Equal(trimmedSurfaces, []byte("null")) || len(trimmedSurfaces) == 0 {
		manifest.Surfaces = []mapSnapshotExporterSurface{}
		return nil
	}
	return json.Unmarshal(trimmedSurfaces, &manifest.Surfaces)
}

type mapSnapshotExporterSurface struct {
	Index        uint32 `json:"index"`
	Name         string `json:"name"`
	SurfaceName  string `json:"surface_name,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ChunkCount   int    `json:"chunk_count"`
	MinX         int    `json:"min_x"`
	MinY         int    `json:"min_y"`
	MaxX         int    `json:"max_x"`
	MaxY         int    `json:"max_y"`
	ViewMinTileX *int   `json:"view_min_tile_x,omitempty"`
	ViewMinTileY *int   `json:"view_min_tile_y,omitempty"`
	ViewMaxTileX *int   `json:"view_max_tile_x,omitempty"`
	ViewMaxTileY *int   `json:"view_max_tile_y,omitempty"`
	File         string `json:"file"`
}

type mapSnapshotExporterChunk struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Data string `json:"data"`
}

type mapSnapshotRuntimeState struct {
	Running            bool
	RunningProfile     string
	LastAttemptProfile string
	LastAttemptAt      *time.Time
	LastError          string
}

//go:embed map_snapshot_exporter/info.json map_snapshot_exporter/control.lua
var mapSnapshotExporterFiles embed.FS

var mapSnapshotOperationMutex sync.Mutex
var mapSnapshotSettingsMutex sync.Mutex
var mapSnapshotRuntimeMutex sync.RWMutex
var mapSnapshotStoreGate sync.RWMutex
var mapSnapshotSchedulerOnce sync.Once
var mapSnapshotSchedulerWake = make(chan struct{}, 1)
var mapSnapshotNow = time.Now
var mapSnapshotCommandTimeout = 10 * time.Minute
var mapSnapshotRootPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), "map-snapshots")
}
var mapSnapshotSettingsPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), mapSnapshotSettingsFileName)
}
var mapSnapshotRunFactorio = func(timeout time.Duration, args []string) error {
	return runFactorioWorldCommand(timeout, args)
}

func LoadMapSnapshotSettings() (MapSnapshotSettings, error) {
	mapSnapshotSettingsMutex.Lock()
	defer mapSnapshotSettingsMutex.Unlock()
	return loadMapSnapshotSettings()
}

func loadMapSnapshotSettings() (MapSnapshotSettings, error) {
	contents, err := os.ReadFile(mapSnapshotSettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return MapSnapshotSettings{IntervalMinutes: defaultMapSnapshotInterval}, nil
	}
	if err != nil {
		return MapSnapshotSettings{}, fmt.Errorf("read map snapshot settings: %w", err)
	}
	var settings MapSnapshotSettings
	if err := json.Unmarshal(contents, &settings); err != nil {
		return MapSnapshotSettings{}, fmt.Errorf("decode map snapshot settings: %w", err)
	}
	if err := validateMapSnapshotSettings(settings); err != nil {
		return MapSnapshotSettings{}, err
	}
	return settings, nil
}

func SetMapSnapshotSettings(settings MapSnapshotSettings) (MapSnapshotSettings, error) {
	if err := validateMapSnapshotSettings(settings); err != nil {
		return MapSnapshotSettings{}, err
	}
	mapSnapshotSettingsMutex.Lock()
	defer mapSnapshotSettingsMutex.Unlock()
	if err := writeMapSnapshotJSONAtomically(mapSnapshotSettingsPath(), settings, 0600); err != nil {
		return MapSnapshotSettings{}, fmt.Errorf("save map snapshot settings: %w", err)
	}
	select {
	case mapSnapshotSchedulerWake <- struct{}{}:
	default:
	}
	return settings, nil
}

func validateMapSnapshotSettings(settings MapSnapshotSettings) error {
	if settings.IntervalMinutes < 0 || settings.IntervalMinutes > maximumMapSnapshotInterval {
		return fmt.Errorf("%w: interval must be between 0 and %d minutes", ErrInvalidMapSnapshotSettings, maximumMapSnapshotInterval)
	}
	return nil
}

func GetMapSnapshotState() (MapSnapshotState, error) {
	settings, err := LoadMapSnapshotSettings()
	if err != nil {
		return MapSnapshotState{}, err
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return MapSnapshotState{}, err
	}

	mapSnapshotRuntimeMutex.RLock()
	runtimeState := mapSnapshotRuntimeState{
		Running:            mapSnapshotRuntime.Running,
		RunningProfile:     mapSnapshotRuntime.RunningProfile,
		LastAttemptProfile: mapSnapshotRuntime.LastAttemptProfile,
	}
	if mapSnapshotRuntime.LastAttemptProfile == profile.ID && mapSnapshotRuntime.LastAttemptAt != nil {
		copyTime := *mapSnapshotRuntime.LastAttemptAt
		runtimeState.LastAttemptAt = &copyTime
		runtimeState.LastError = mapSnapshotRuntime.LastError
	}
	mapSnapshotRuntimeMutex.RUnlock()

	snapshot, err := loadMapSnapshot(profile.ID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return MapSnapshotState{}, err
	}
	return MapSnapshotState{
		Settings:       settings,
		Running:        runtimeState.Running,
		RunningProfile: runtimeState.RunningProfile,
		LastAttemptAt:  runtimeState.LastAttemptAt,
		LastError:      runtimeState.LastError,
		Snapshot:       snapshot,
	}, nil
}

var mapSnapshotRuntime mapSnapshotRuntimeState

func TriggerMapSnapshot() (MapSnapshotState, error) {
	if !mapSnapshotOperationMutex.TryLock() {
		state, _ := GetMapSnapshotState()
		return state, ErrMapSnapshotBusy
	}

	profile, err := activeMapSnapshotProfile()
	if err != nil {
		mapSnapshotOperationMutex.Unlock()
		return MapSnapshotState{}, err
	}
	if err := validateMapSnapshotFactorioVersion(profile.InstalledVersion); err != nil {
		mapSnapshotOperationMutex.Unlock()
		return MapSnapshotState{}, err
	}
	now := mapSnapshotNow().UTC()
	mapSnapshotRuntimeMutex.Lock()
	mapSnapshotRuntime.Running = true
	mapSnapshotRuntime.RunningProfile = profile.ID
	mapSnapshotRuntime.LastAttemptProfile = profile.ID
	mapSnapshotRuntime.LastAttemptAt = &now
	mapSnapshotRuntime.LastError = ""
	mapSnapshotRuntimeMutex.Unlock()

	go func() {
		defer mapSnapshotOperationMutex.Unlock()
		unlockData := LockProfileDataRead()
		err := generateActiveMapSnapshot(profile.ID)
		unlockData()

		mapSnapshotRuntimeMutex.Lock()
		mapSnapshotRuntime.Running = false
		mapSnapshotRuntime.RunningProfile = ""
		if err != nil {
			mapSnapshotRuntime.LastError = err.Error()
			log.Printf("Unable to generate Factorio map snapshot: %v", err)
		} else {
			mapSnapshotRuntime.LastError = ""
		}
		mapSnapshotRuntimeMutex.Unlock()
	}()
	return GetMapSnapshotState()
}

func StartMapSnapshotScheduler() {
	mapSnapshotSchedulerOnce.Do(func() {
		go mapSnapshotSchedulerLoop()
	})
}

func mapSnapshotSchedulerLoop() {
	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()
	select {
	case <-startupTimer.C:
	case <-mapSnapshotSchedulerWake:
	}
	mapSnapshotScheduleIfDue()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-mapSnapshotSchedulerWake:
		}
		mapSnapshotScheduleIfDue()
	}
}

func mapSnapshotScheduleIfDue() {
	settings, err := LoadMapSnapshotSettings()
	if err != nil || settings.IntervalMinutes == 0 {
		return
	}
	state, err := GetMapSnapshotState()
	if err != nil || state.Running {
		return
	}
	lastActivity := time.Time{}
	if state.Snapshot != nil {
		lastActivity = state.Snapshot.GeneratedAt
	}
	if state.LastAttemptAt != nil && state.LastAttemptAt.After(lastActivity) {
		lastActivity = *state.LastAttemptAt
	}
	if !lastActivity.IsZero() && mapSnapshotNow().UTC().Before(lastActivity.Add(time.Duration(settings.IntervalMinutes)*time.Minute)) {
		return
	}
	if _, err := TriggerMapSnapshot(); err != nil && !errors.Is(err, ErrMapSnapshotBusy) {
		log.Printf("Unable to schedule Factorio map snapshot: %v", err)
	}
}

func generateActiveMapSnapshot(expectedProfileID string) error {
	// Release installation replaces the same binary and read-data tree used by
	// this isolated process. Keep that tree stable for the complete export; the
	// live game server uses the files read-only and does not need to stop.
	factorioProgramFilesGate.RLock()
	defer factorioProgramFilesGate.RUnlock()

	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return err
	}
	if profile.ID != expectedProfileID {
		return errors.New("active profile changed before map snapshot generation started")
	}
	profile, err = captureActiveProfile(profile)
	if err != nil {
		return fmt.Errorf("refresh active profile before map snapshot generation: %w", err)
	}
	if err := validateMapSnapshotFactorioVersion(profile.InstalledVersion); err != nil {
		return err
	}
	directories := profileActiveDirectories()
	serverStatus := GetFactorioServer().Snapshot()
	selectedSave := profile.SelectedSave
	if serverStatus.Running && serverStatus.Savefile != "" {
		selectedSave = serverStatus.Savefile
	}
	source, err := findMapSnapshotSourceSave(directories["saves"], selectedSave, serverStatus.Running)
	if err != nil {
		return err
	}

	workDirectory, err := os.MkdirTemp("", "fsm-map-snapshot-")
	if err != nil {
		return fmt.Errorf("create map snapshot workspace: %w", err)
	}
	defer os.RemoveAll(workDirectory)

	saveCopy := filepath.Join(workDirectory, source.Name)
	if err := copyCheckpointAtomically(filepath.Join(directories["saves"], source.Name), saveCopy); err != nil {
		return fmt.Errorf("copy source save: %w", err)
	}

	modsDirectory := filepath.Join(workDirectory, "mods")
	if err := prepareMapSnapshotMods(directories["mods"], modsDirectory, profile.InstalledVersion); err != nil {
		return fmt.Errorf("prepare isolated mods: %w", err)
	}
	writeDataDirectory := filepath.Join(workDirectory, "write-data")
	if err := os.MkdirAll(writeDataDirectory, 0700); err != nil {
		return err
	}
	configPath := filepath.Join(workDirectory, "config.ini")
	if err := writeMapSnapshotFactorioConfig(configPath, bootstrap.GetConfig().FactorioDir, writeDataDirectory); err != nil {
		return err
	}

	args := []string{
		"--config", configPath,
		"--mod-directory", modsDirectory,
		"--benchmark", saveCopy,
		"--benchmark-ticks", "8",
		"--benchmark-runs", "1",
		"--benchmark-sanitize",
	}
	if err := mapSnapshotRunFactorio(mapSnapshotCommandTimeout, args); err != nil {
		if logContents, logErr := readMapSnapshotFileTail(filepath.Join(writeDataDirectory, "factorio-current.log"), maximumMapSnapshotErrorLog); logErr == nil {
			if detail := redactWorldCommandOutput(logContents); detail != "" {
				return fmt.Errorf("run isolated Factorio exporter: %w: %s", err, detail)
			}
		}
		return fmt.Errorf("run isolated Factorio exporter: %w", err)
	}

	exportDirectory := filepath.Join(writeDataDirectory, "script-output", mapSnapshotExporterName)
	if contents, err := readMapSnapshotFile(filepath.Join(exportDirectory, "complete"), 16); err != nil || strings.TrimSpace(string(contents)) != "ok" {
		if err == nil {
			err = errors.New("completion marker is invalid")
		}
		return fmt.Errorf("map exporter did not complete: %w", err)
	}
	manifest, err := readMapSnapshotExporterManifest(exportDirectory)
	if err != nil {
		return err
	}
	return persistRenderedMapSnapshot(profile, *source, manifest, exportDirectory)
}

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
	if running && latestAutosave != nil && (selectedSave == nil || selectedSave.LastMod.Before(latestAutosave.LastMod)) {
		return latestAutosave, nil
	}
	if selectedSave != nil {
		return selectedSave, nil
	}
	if latestSave == nil {
		return nil, errors.New("no usable save is available for a map snapshot")
	}
	return latestSave, nil
}

func prepareMapSnapshotMods(source, destination, factorioVersion string) error {
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
	if err := installMapSnapshotExporter(destination, factorioVersion); err != nil {
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

func installMapSnapshotExporter(modsDirectory, factorioVersion string) error {
	destination := filepath.Join(modsDirectory, mapSnapshotExporterName+"_0.1.0")
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

func writeMapSnapshotFactorioConfig(path, factorioDirectory, writeDataDirectory string) error {
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
		info, err := os.Stat(filepath.Join(directory, surface.File))
		if err != nil || !info.Mode().IsRegular() || info.Size() > maximumMapSnapshotExportBytes {
			if err == nil {
				err = errors.New("surface export is invalid or too large")
			}
			return mapSnapshotExporterManifest{}, fmt.Errorf("validate surface %d export: %w", surface.Index, err)
		}
		seen[surface.Index] = true
	}
	return manifest, nil
}

func readMapSnapshotFile(path string, maximumBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumBytes {
		return nil, errors.New("file is not regular or exceeds the size limit")
	}
	return io.ReadAll(io.LimitReader(file, maximumBytes+1))
}

func readMapSnapshotFileTail(path string, maximumBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("file is not regular")
	}
	if info.Size() > maximumBytes {
		if _, err := file.Seek(info.Size()-maximumBytes, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(file, maximumBytes))
}

func persistRenderedMapSnapshot(profile Profile, source Save, manifest mapSnapshotExporterManifest, exportDirectory string) error {
	root := mapSnapshotRootPath()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".map-snapshot-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	surfaces := make([]MapSnapshotSurface, 0, len(manifest.Surfaces))
	for _, exportedSurface := range manifest.Surfaces {
		filename := "surface-" + strconv.FormatUint(uint64(exportedSurface.Index), 10) + ".png"
		width, height, err := renderMapSnapshotSurface(exportedSurface, filepath.Join(exportDirectory, exportedSurface.File), filepath.Join(stage, filename))
		if err != nil {
			return fmt.Errorf("render surface %q: %w", exportedSurface.Name, err)
		}
		surfaces = append(surfaces, MapSnapshotSurface{
			ID:          strconv.FormatUint(uint64(exportedSurface.Index), 10),
			Index:       exportedSurface.Index,
			Name:        exportedSurface.Name,
			SurfaceName: exportedSurface.SurfaceName,
			Kind:        exportedSurface.Kind,
			ChunkCount:  exportedSurface.ChunkCount,
			Width:       width,
			Height:      height,
			File:        filename,
		})
	}
	sort.SliceStable(surfaces, func(left, right int) bool { return surfaces[left].Index < surfaces[right].Index })
	snapshot := MapSnapshot{
		SchemaVersion:   mapSnapshotSchemaVersion,
		ProfileID:       profile.ID,
		SaveName:        source.Name,
		SaveModifiedAt:  source.LastMod.UTC(),
		GeneratedAt:     mapSnapshotNow().UTC(),
		FactorioVersion: manifest.GameVersion,
		GameTick:        manifest.GameTick,
		Force:           manifest.Force,
		Surfaces:        surfaces,
	}
	if err := writeMapSnapshotJSONAtomically(filepath.Join(stage, mapSnapshotMetadataFileName), snapshot, 0600); err != nil {
		return err
	}
	return activateMapSnapshotDirectory(profile.ID, stage)
}

func renderMapSnapshotSurface(surface mapSnapshotExporterSurface, sourcePath, destinationPath string) (int, int, error) {
	renderMinX, renderMinY, renderMaxX, renderMaxY, err := mapSnapshotRenderTileBounds(surface)
	if err != nil {
		return 0, 0, err
	}
	sourceWidth := renderMaxX - renderMinX + 1
	sourceHeight := renderMaxY - renderMinY + 1
	if sourceWidth < 1 || sourceHeight < 1 {
		return 0, 0, errors.New("surface bounds are empty")
	}
	scale := 1.0
	if sourceWidth > maximumMapSnapshotDimension || sourceHeight > maximumMapSnapshotDimension {
		widthScale := float64(maximumMapSnapshotDimension) / float64(sourceWidth)
		heightScale := float64(maximumMapSnapshotDimension) / float64(sourceHeight)
		if widthScale < heightScale {
			scale = widthScale
		} else {
			scale = heightScale
		}
	}
	width := max(1, int(float64(sourceWidth)*scale))
	height := max(1, int(float64(sourceHeight)*scale))
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 7, G: 12, B: 17, A: 255}}, image.Point{}, draw.Src)

	file, err := os.Open(sourcePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maximumMapSnapshotExportBytes+1))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	count := 0
	renderedCount := 0
	seen := make(map[[2]int]bool, surface.ChunkCount)
	for scanner.Scan() {
		var chunk mapSnapshotExporterChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			return 0, 0, fmt.Errorf("decode chunk line: %w", err)
		}
		key := [2]int{chunk.X, chunk.Y}
		if seen[key] || chunk.X < surface.MinX || chunk.X > surface.MaxX || chunk.Y < surface.MinY || chunk.Y > surface.MaxY {
			return 0, 0, fmt.Errorf("invalid or duplicate chunk %d,%d", chunk.X, chunk.Y)
		}
		seen[key] = true
		count++
		chunkMinX := chunk.X * mapSnapshotChunkPixels
		chunkMinY := chunk.Y * mapSnapshotChunkPixels
		chunkMaxX := chunkMinX + mapSnapshotChunkPixels - 1
		chunkMaxY := chunkMinY + mapSnapshotChunkPixels - 1
		if chunkMaxX < renderMinX || chunkMinX > renderMaxX || chunkMaxY < renderMinY || chunkMinY > renderMaxY {
			continue
		}
		pixels, err := decodeMapSnapshotChunk(chunk.Data)
		if err != nil {
			return 0, 0, fmt.Errorf("decode chunk %d,%d: %w", chunk.X, chunk.Y, err)
		}
		paintMapSnapshotChunk(canvas, pixels, chunk.X, chunk.Y, renderMinX, renderMinY, renderMaxX, renderMaxY, scale)
		renderedCount++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if count != surface.ChunkCount {
		return 0, 0, fmt.Errorf("expected %d chunks, read %d", surface.ChunkCount, count)
	}
	if renderedCount == 0 {
		return 0, 0, errors.New("surface view contains no charted chunks")
	}

	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, 0, err
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	encodeErr := encoder.Encode(destination, canvas)
	closeErr := destination.Close()
	if encodeErr != nil {
		return 0, 0, encodeErr
	}
	if closeErr != nil {
		return 0, 0, closeErr
	}
	return width, height, nil
}

func mapSnapshotRenderTileBounds(surface mapSnapshotExporterSurface) (int, int, int, int, error) {
	chartMinX := surface.MinX * mapSnapshotChunkPixels
	chartMinY := surface.MinY * mapSnapshotChunkPixels
	chartMaxX := (surface.MaxX+1)*mapSnapshotChunkPixels - 1
	chartMaxY := (surface.MaxY+1)*mapSnapshotChunkPixels - 1
	viewBounds := []*int{surface.ViewMinTileX, surface.ViewMinTileY, surface.ViewMaxTileX, surface.ViewMaxTileY}
	present := 0
	for _, value := range viewBounds {
		if value != nil {
			present++
		}
	}
	if present == 0 {
		return chartMinX, chartMinY, chartMaxX, chartMaxY, nil
	}
	if present != len(viewBounds) {
		return 0, 0, 0, 0, errors.New("surface view bounds are incomplete")
	}
	if *surface.ViewMinTileX < chartMinX || *surface.ViewMinTileY < chartMinY || *surface.ViewMaxTileX > chartMaxX || *surface.ViewMaxTileY > chartMaxY || *surface.ViewMinTileX > *surface.ViewMaxTileX || *surface.ViewMinTileY > *surface.ViewMaxTileY {
		return 0, 0, 0, 0, errors.New("surface view bounds exceed charted bounds")
	}
	return *surface.ViewMinTileX, *surface.ViewMinTileY, *surface.ViewMaxTileX, *surface.ViewMaxTileY, nil
}

func decodeMapSnapshotChunk(encoded string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	pixels, readErr := io.ReadAll(io.LimitReader(reader, mapSnapshotChunkPixels*mapSnapshotChunkPixels*2+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(pixels) != mapSnapshotChunkPixels*mapSnapshotChunkPixels*2 {
		return nil, fmt.Errorf("expected 2048 RGB565 bytes, got %d", len(pixels))
	}
	return pixels, nil
}

func paintMapSnapshotChunk(canvas *image.RGBA, pixels []byte, chunkX, chunkY, minX, minY, maxX, maxY int, scale float64) {
	baseX := chunkX * mapSnapshotChunkPixels
	baseY := chunkY * mapSnapshotChunkPixels
	for y := 0; y < mapSnapshotChunkPixels; y++ {
		for x := 0; x < mapSnapshotChunkPixels; x++ {
			tileX := baseX + x
			tileY := baseY + y
			if tileX < minX || tileX > maxX || tileY < minY || tileY > maxY {
				continue
			}
			offset := (y*mapSnapshotChunkPixels + x) * 2
			value := binary.LittleEndian.Uint16(pixels[offset : offset+2])
			red := uint8(((value >> 11) & 0x1f) * 255 / 31)
			green := uint8(((value >> 5) & 0x3f) * 255 / 63)
			blue := uint8((value & 0x1f) * 255 / 31)
			destinationX := int(float64(tileX-minX) * scale)
			destinationY := int(float64(tileY-minY) * scale)
			if image.Pt(destinationX, destinationY).In(canvas.Bounds()) {
				canvas.SetRGBA(destinationX, destinationY, color.RGBA{R: red, G: green, B: blue, A: 255})
			}
		}
	}
}

func activateMapSnapshotDirectory(profileID, stage string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	mapSnapshotStoreGate.Lock()
	defer mapSnapshotStoreGate.Unlock()
	destination := filepath.Join(mapSnapshotRootPath(), profileID)
	backup := destination + ".previous"
	_ = os.RemoveAll(backup)
	hasPrevious := false
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("back up previous map snapshot: %w", err)
		}
		hasPrevious = true
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		if hasPrevious {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("activate map snapshot: %w", err)
	}
	if hasPrevious {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func loadMapSnapshot(profileID string) (*MapSnapshot, error) {
	if err := validateProfileID(profileID); err != nil {
		return nil, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := os.ReadFile(filepath.Join(mapSnapshotRootPath(), profileID, mapSnapshotMetadataFileName))
	if err != nil {
		return nil, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		return nil, fmt.Errorf("decode map snapshot: %w", err)
	}
	if snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profileID {
		return nil, errors.New("map snapshot metadata is invalid")
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if surface.ID != strconv.FormatUint(uint64(surface.Index), 10) {
			return nil, errors.New("map snapshot surface metadata is invalid")
		}
		surface.File = "surface-" + surface.ID + ".png"
	}
	return &snapshot, nil
}

func ReadMapSnapshotImage(surfaceID string) ([]byte, time.Time, error) {
	if _, err := strconv.ParseUint(surfaceID, 10, 32); err != nil {
		return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return nil, time.Time{}, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := os.ReadFile(filepath.Join(mapSnapshotRootPath(), profile.ID, mapSnapshotMetadataFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil || snapshot.ProfileID != profile.ID {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	for _, surface := range snapshot.Surfaces {
		if surface.ID != surfaceID || surface.ID != strconv.FormatUint(uint64(surface.Index), 10) {
			continue
		}
		imageBytes, err := os.ReadFile(filepath.Join(mapSnapshotRootPath(), profile.ID, "surface-"+surfaceID+".png"))
		if err != nil {
			return nil, time.Time{}, err
		}
		return imageBytes, snapshot.GeneratedAt, nil
	}
	return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
}

func deleteProfileMapSnapshot(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	mapSnapshotStoreGate.Lock()
	defer mapSnapshotStoreGate.Unlock()
	return os.RemoveAll(filepath.Join(mapSnapshotRootPath(), profileID))
}

func writeMapSnapshotJSONAtomically(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".map-snapshot-json-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	backup := path + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
