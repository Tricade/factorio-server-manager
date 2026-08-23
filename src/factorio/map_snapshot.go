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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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
	maximumMapSnapshotImageBytes  = 128 * 1024 * 1024
	maximumMapSnapshotEntityCount = 100000
	maximumMapSnapshotEntityTotal = 1_000_000_000
	maximumMapSnapshotEntityBytes = 32 * 1024 * 1024
	maximumMapSnapshotEntityLine  = 16 * 1024
	maximumMapSnapshotCoordinate  = 10_000_000
	mapSnapshotChunkPixels        = 32
)

var (
	ErrMapSnapshotBusy            = errors.New("a map snapshot is already being generated")
	ErrMapSnapshotNotFound        = errors.New("map snapshot not found")
	ErrMapSnapshotSurfaceNotFound = errors.New("map snapshot surface not found")
	ErrMapSnapshotDetailsNotFound = errors.New("map snapshot entity details not found")
	ErrInvalidMapSnapshotSettings = errors.New("invalid map snapshot settings")
)

type MapSnapshotSettings struct {
	IntervalMinutes int `json:"interval_minutes"`
}

type MapSnapshotSurface struct {
	ID                  string  `json:"id"`
	Index               uint32  `json:"index"`
	Name                string  `json:"name"`
	SurfaceName         string  `json:"surface_name,omitempty"`
	Kind                string  `json:"kind,omitempty"`
	ChunkCount          int     `json:"chunk_count"`
	Width               int     `json:"width"`
	Height              int     `json:"height"`
	ViewBoundsAvailable bool    `json:"view_bounds_available"`
	ViewMinTileX        int     `json:"view_min_tile_x"`
	ViewMinTileY        int     `json:"view_min_tile_y"`
	ViewMaxTileX        int     `json:"view_max_tile_x"`
	ViewMaxTileY        int     `json:"view_max_tile_y"`
	PixelsPerTile       float64 `json:"pixels_per_tile"`
	EntitiesAvailable   bool    `json:"entities_available"`
	EntityCount         int     `json:"entity_count"`
	EntityTotalCount    int     `json:"entity_total_count"`
	EntityTruncated     bool    `json:"entity_truncated"`
	File                string  `json:"-"`
	EntityFile          string  `json:"-"`
}

// MapSnapshotEntity describes the footprint of a player-force building in the
// isolated snapshot save. It deliberately contains geometry and prototype
// identifiers only; chart images do not contain client sprites.
type MapSnapshotEntity struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Direction   uint8                  `json:"direction"`
	BoundingBox MapSnapshotBoundingBox `json:"bounding_box"`
}

type MapSnapshotBoundingBox struct {
	LeftTop     MapSnapshotPosition `json:"left_top"`
	RightBottom MapSnapshotPosition `json:"right_bottom"`
}

type MapSnapshotPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MapSnapshotPlayer is read only from the isolated snapshot save. OnlineTime
// is Factorio's persisted LuaPlayer.online_time value in game ticks; no Lua is
// executed against the live server to collect it.
type MapSnapshotPlayer struct {
	Name              string `json:"name"`
	OnlineTimeTicks   uint64 `json:"online_time_ticks"`
	OnlineTimeSeconds uint64 `json:"online_time_seconds"`
	Rank              int    `json:"rank"`
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
	Players         []MapSnapshotPlayer  `json:"players"`
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
	Players       []mapSnapshotExporterPlayer  `json:"players"`
	Surfaces      []mapSnapshotExporterSurface `json:"surfaces"`
}

func (manifest *mapSnapshotExporterManifest) UnmarshalJSON(contents []byte) error {
	var raw struct {
		SchemaVersion int             `json:"schema_version"`
		GameTick      uint64          `json:"game_tick"`
		GameVersion   string          `json:"game_version"`
		Force         string          `json:"force"`
		Players       json.RawMessage `json:"players"`
		Surfaces      json.RawMessage `json:"surfaces"`
	}
	if err := json.Unmarshal(contents, &raw); err != nil {
		return err
	}
	manifest.SchemaVersion = raw.SchemaVersion
	manifest.GameTick = raw.GameTick
	manifest.GameVersion = raw.GameVersion
	manifest.Force = raw.Force
	trimmedPlayers := bytes.TrimSpace(raw.Players)
	if bytes.Equal(trimmedPlayers, []byte("{}")) || bytes.Equal(trimmedPlayers, []byte("null")) || len(trimmedPlayers) == 0 {
		manifest.Players = []mapSnapshotExporterPlayer{}
	} else if err := json.Unmarshal(trimmedPlayers, &manifest.Players); err != nil {
		return err
	}
	trimmedSurfaces := bytes.TrimSpace(raw.Surfaces)
	if bytes.Equal(trimmedSurfaces, []byte("{}")) || bytes.Equal(trimmedSurfaces, []byte("null")) || len(trimmedSurfaces) == 0 {
		manifest.Surfaces = []mapSnapshotExporterSurface{}
		return nil
	}
	return json.Unmarshal(trimmedSurfaces, &manifest.Surfaces)
}

type mapSnapshotExporterPlayer struct {
	Name       string `json:"name"`
	OnlineTime uint64 `json:"online_time"`
}

type mapSnapshotExporterSurface struct {
	Index            uint32 `json:"index"`
	Name             string `json:"name"`
	SurfaceName      string `json:"surface_name,omitempty"`
	Kind             string `json:"kind,omitempty"`
	ChunkCount       int    `json:"chunk_count"`
	MinX             int    `json:"min_x"`
	MinY             int    `json:"min_y"`
	MaxX             int    `json:"max_x"`
	MaxY             int    `json:"max_y"`
	ViewMinTileX     *int   `json:"view_min_tile_x,omitempty"`
	ViewMinTileY     *int   `json:"view_min_tile_y,omitempty"`
	ViewMaxTileX     *int   `json:"view_max_tile_x,omitempty"`
	ViewMaxTileY     *int   `json:"view_max_tile_y,omitempty"`
	File             string `json:"file"`
	EntityFile       string `json:"entity_file"`
	EntityCount      int    `json:"entity_count"`
	EntityTotalCount int    `json:"entity_total_count"`
	EntityTruncated  bool   `json:"entity_truncated"`
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
		geometry, err := calculateMapSnapshotRenderGeometry(exportedSurface)
		if err != nil {
			return fmt.Errorf("calculate surface %q geometry: %w", exportedSurface.Name, err)
		}
		width, height, err := renderMapSnapshotSurface(exportedSurface, filepath.Join(exportDirectory, exportedSurface.File), filepath.Join(stage, filename))
		if err != nil {
			return fmt.Errorf("render surface %q: %w", exportedSurface.Name, err)
		}
		entityFilename := "surface-" + strconv.FormatUint(uint64(exportedSurface.Index), 10) + "-entities.jsonl"
		if err := writeMapSnapshotEntityFile(filepath.Join(exportDirectory, exportedSurface.EntityFile), filepath.Join(stage, entityFilename), exportedSurface.EntityCount); err != nil {
			return fmt.Errorf("persist entity details for surface %q: %w", exportedSurface.Name, err)
		}
		surfaces = append(surfaces, MapSnapshotSurface{
			ID:                  strconv.FormatUint(uint64(exportedSurface.Index), 10),
			Index:               exportedSurface.Index,
			Name:                exportedSurface.Name,
			SurfaceName:         exportedSurface.SurfaceName,
			Kind:                exportedSurface.Kind,
			ChunkCount:          exportedSurface.ChunkCount,
			Width:               width,
			Height:              height,
			ViewBoundsAvailable: true,
			ViewMinTileX:        geometry.MinTileX,
			ViewMinTileY:        geometry.MinTileY,
			ViewMaxTileX:        geometry.MaxTileX,
			ViewMaxTileY:        geometry.MaxTileY,
			PixelsPerTile:       geometry.PixelsPerTile,
			EntitiesAvailable:   true,
			EntityCount:         exportedSurface.EntityCount,
			EntityTotalCount:    exportedSurface.EntityTotalCount,
			EntityTruncated:     exportedSurface.EntityTruncated,
			File:                filename,
			EntityFile:          entityFilename,
		})
	}
	sort.SliceStable(surfaces, func(left, right int) bool { return surfaces[left].Index < surfaces[right].Index })
	players := make([]MapSnapshotPlayer, 0, len(manifest.Players))
	for _, exportedPlayer := range manifest.Players {
		players = append(players, MapSnapshotPlayer{
			Name:              exportedPlayer.Name,
			OnlineTimeTicks:   exportedPlayer.OnlineTime,
			OnlineTimeSeconds: exportedPlayer.OnlineTime / 60,
		})
	}
	sort.SliceStable(players, func(left, right int) bool {
		if players[left].OnlineTimeTicks != players[right].OnlineTimeTicks {
			return players[left].OnlineTimeTicks > players[right].OnlineTimeTicks
		}
		return strings.ToLower(players[left].Name) < strings.ToLower(players[right].Name)
	})
	for index := range players {
		players[index].Rank = index + 1
		if index > 0 && players[index].OnlineTimeTicks == players[index-1].OnlineTimeTicks {
			players[index].Rank = players[index-1].Rank
		}
	}
	snapshot := MapSnapshot{
		SchemaVersion:   mapSnapshotSchemaVersion,
		ProfileID:       profile.ID,
		SaveName:        source.Name,
		SaveModifiedAt:  source.LastMod.UTC(),
		GeneratedAt:     mapSnapshotNow().UTC(),
		FactorioVersion: manifest.GameVersion,
		GameTick:        manifest.GameTick,
		Force:           manifest.Force,
		Players:         players,
		Surfaces:        surfaces,
	}
	if err := writeMapSnapshotJSONAtomically(filepath.Join(stage, mapSnapshotMetadataFileName), snapshot, 0600); err != nil {
		return err
	}
	return activateMapSnapshotDirectory(profile.ID, stage)
}

func writeMapSnapshotEntityFile(sourcePath, destinationPath string, expectedCount int) error {
	if expectedCount < 0 || expectedCount > maximumMapSnapshotEntityCount {
		return errors.New("entity count exceeds the limit")
	}
	source, info, err := openRegularMapSnapshotFile(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if info.Size() > maximumMapSnapshotEntityBytes {
		return errors.New("entity dataset exceeds the size limit")
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	processErr := processMapSnapshotEntities(io.LimitReader(source, maximumMapSnapshotEntityBytes+1), expectedCount, destination)
	if processErr == nil {
		processErr = destination.Sync()
	}
	closeErr := destination.Close()
	if processErr != nil {
		_ = os.Remove(destinationPath)
		return processErr
	}
	if closeErr != nil {
		_ = os.Remove(destinationPath)
		return closeErr
	}
	return nil
}

func processMapSnapshotEntities(reader io.Reader, expectedCount int, destination io.Writer) error {
	if expectedCount < 0 || expectedCount > maximumMapSnapshotEntityCount {
		return errors.New("entity count exceeds the limit")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maximumMapSnapshotEntityLine)
	count := 0
	var written int64
	for scanner.Scan() {
		if count >= maximumMapSnapshotEntityCount {
			return errors.New("entity dataset contains too many records")
		}
		entity, err := decodeMapSnapshotEntity(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("decode entity line %d: %w", count+1, err)
		}
		count++
		if destination == nil {
			continue
		}
		line, err := json.Marshal(entity)
		if err != nil {
			return err
		}
		line = append(line, '\n')
		if len(line) > maximumMapSnapshotEntityLine || written+int64(len(line)) > maximumMapSnapshotEntityBytes {
			return errors.New("canonical entity dataset exceeds the size limit")
		}
		amount, err := destination.Write(line)
		if err != nil {
			return err
		}
		if amount != len(line) {
			return io.ErrShortWrite
		}
		written += int64(amount)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if count != expectedCount {
		return fmt.Errorf("expected %d entities, read %d", expectedCount, count)
	}
	return nil
}

func decodeMapSnapshotEntity(line []byte) (MapSnapshotEntity, error) {
	var entity MapSnapshotEntity
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entity); err != nil {
		return MapSnapshotEntity{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("entity line contains more than one JSON value")
		}
		return MapSnapshotEntity{}, err
	}
	if !validMapSnapshotEntityLabel(entity.Name) || !validMapSnapshotEntityLabel(entity.Type) {
		return MapSnapshotEntity{}, errors.New("entity name or type is invalid")
	}
	if entity.Direction > 15 {
		return MapSnapshotEntity{}, errors.New("entity direction is invalid")
	}
	coordinates := []float64{
		entity.BoundingBox.LeftTop.X,
		entity.BoundingBox.LeftTop.Y,
		entity.BoundingBox.RightBottom.X,
		entity.BoundingBox.RightBottom.Y,
	}
	for _, coordinate := range coordinates {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) || math.Abs(coordinate) > maximumMapSnapshotCoordinate {
			return MapSnapshotEntity{}, errors.New("entity bounding box coordinate is invalid")
		}
	}
	if entity.BoundingBox.LeftTop.X > entity.BoundingBox.RightBottom.X || entity.BoundingBox.LeftTop.Y > entity.BoundingBox.RightBottom.Y {
		return MapSnapshotEntity{}, errors.New("entity bounding box is inverted")
	}
	return entity, nil
}

func validMapSnapshotEntityLabel(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func renderMapSnapshotSurface(surface mapSnapshotExporterSurface, sourcePath, destinationPath string) (int, int, error) {
	geometry, err := calculateMapSnapshotRenderGeometry(surface)
	if err != nil {
		return 0, 0, err
	}
	renderMinX, renderMinY, renderMaxX, renderMaxY := geometry.MinTileX, geometry.MinTileY, geometry.MaxTileX, geometry.MaxTileY
	scale := geometry.PixelsPerTile
	width, height := geometry.Width, geometry.Height
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

type mapSnapshotRenderGeometry struct {
	MinTileX      int
	MinTileY      int
	MaxTileX      int
	MaxTileY      int
	Width         int
	Height        int
	PixelsPerTile float64
}

func calculateMapSnapshotRenderGeometry(surface mapSnapshotExporterSurface) (mapSnapshotRenderGeometry, error) {
	renderMinX, renderMinY, renderMaxX, renderMaxY, err := mapSnapshotRenderTileBounds(surface)
	if err != nil {
		return mapSnapshotRenderGeometry{}, err
	}
	sourceWidth := renderMaxX - renderMinX + 1
	sourceHeight := renderMaxY - renderMinY + 1
	if sourceWidth < 1 || sourceHeight < 1 {
		return mapSnapshotRenderGeometry{}, errors.New("surface bounds are empty")
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
	return mapSnapshotRenderGeometry{
		MinTileX: renderMinX, MinTileY: renderMinY, MaxTileX: renderMaxX, MaxTileY: renderMaxY,
		Width: max(1, int(float64(sourceWidth)*scale)), Height: max(1, int(float64(sourceHeight)*scale)), PixelsPerTile: scale,
	}, nil
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
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profileID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
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
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, err
		}
	}
	return &snapshot, nil
}

func populateMapSnapshotSurfaceFiles(surface *MapSnapshotSurface) error {
	if surface.ID != strconv.FormatUint(uint64(surface.Index), 10) || surface.ChunkCount < 1 || surface.ChunkCount > maximumMapSnapshotChunks || surface.Width < 1 || surface.Width > maximumMapSnapshotDimension || surface.Height < 1 || surface.Height > maximumMapSnapshotDimension {
		return errors.New("map snapshot surface metadata is invalid")
	}
	surface.File = "surface-" + surface.ID + ".png"
	if !surface.ViewBoundsAvailable {
		if surface.ViewMinTileX != 0 || surface.ViewMinTileY != 0 || surface.ViewMaxTileX != 0 || surface.ViewMaxTileY != 0 || surface.PixelsPerTile != 0 || surface.EntitiesAvailable {
			return errors.New("map snapshot view metadata is invalid")
		}
	} else if err := validateStoredMapSnapshotGeometry(*surface); err != nil {
		return err
	}
	if !surface.EntitiesAvailable {
		if surface.EntityCount != 0 || surface.EntityTotalCount != 0 || surface.EntityTruncated {
			return errors.New("map snapshot entity metadata is invalid")
		}
		surface.EntityFile = ""
		return nil
	}
	if surface.EntityCount < 0 || surface.EntityCount > maximumMapSnapshotEntityCount || surface.EntityTotalCount < surface.EntityCount || surface.EntityTotalCount > maximumMapSnapshotEntityTotal || surface.EntityTruncated != (surface.EntityTotalCount > surface.EntityCount) {
		return errors.New("map snapshot entity metadata is invalid")
	}
	surface.EntityFile = "surface-" + surface.ID + "-entities.jsonl"
	return nil
}

func validateStoredMapSnapshotGeometry(surface MapSnapshotSurface) error {
	coordinates := []int{surface.ViewMinTileX, surface.ViewMinTileY, surface.ViewMaxTileX, surface.ViewMaxTileY}
	for _, coordinate := range coordinates {
		if coordinate < -maximumMapSnapshotCoordinate || coordinate > maximumMapSnapshotCoordinate {
			return errors.New("map snapshot view metadata is invalid")
		}
	}
	if surface.ViewMinTileX > surface.ViewMaxTileX || surface.ViewMinTileY > surface.ViewMaxTileY {
		return errors.New("map snapshot view metadata is invalid")
	}
	sourceWidth := surface.ViewMaxTileX - surface.ViewMinTileX + 1
	sourceHeight := surface.ViewMaxTileY - surface.ViewMinTileY + 1
	expectedScale := 1.0
	if sourceWidth > maximumMapSnapshotDimension || sourceHeight > maximumMapSnapshotDimension {
		expectedScale = math.Min(float64(maximumMapSnapshotDimension)/float64(sourceWidth), float64(maximumMapSnapshotDimension)/float64(sourceHeight))
	}
	expectedWidth := max(1, int(float64(sourceWidth)*expectedScale))
	expectedHeight := max(1, int(float64(sourceHeight)*expectedScale))
	if surface.Width != expectedWidth || surface.Height != expectedHeight || math.IsNaN(surface.PixelsPerTile) || math.IsInf(surface.PixelsPerTile, 0) || math.Abs(surface.PixelsPerTile-expectedScale) > 1e-12 {
		return errors.New("map snapshot view metadata is invalid")
	}
	return nil
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
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil || snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profile.ID {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if surface.ID != surfaceID {
			continue
		}
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, time.Time{}, ErrMapSnapshotNotFound
		}
		imageBytes, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, surface.File), maximumMapSnapshotImageBytes)
		if err != nil {
			return nil, time.Time{}, err
		}
		return imageBytes, snapshot.GeneratedAt, nil
	}
	return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
}

// ReadMapSnapshotEntities returns a bounded, validated NDJSON dataset for one
// surface. Each line is a MapSnapshotEntity from the isolated save exporter.
func ReadMapSnapshotEntities(surfaceID string) ([]byte, time.Time, error) {
	if _, err := strconv.ParseUint(surfaceID, 10, 32); err != nil {
		return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return nil, time.Time{}, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil || snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profile.ID {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if surface.ID != surfaceID {
			continue
		}
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, time.Time{}, ErrMapSnapshotNotFound
		}
		if !surface.EntitiesAvailable {
			return nil, time.Time{}, ErrMapSnapshotDetailsNotFound
		}
		entityBytes, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, surface.EntityFile), maximumMapSnapshotEntityBytes)
		if err != nil {
			return nil, time.Time{}, err
		}
		if err := processMapSnapshotEntities(bytes.NewReader(entityBytes), surface.EntityCount, nil); err != nil {
			return nil, time.Time{}, fmt.Errorf("validate stored map snapshot entities: %w", err)
		}
		return entityBytes, snapshot.GeneratedAt, nil
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
