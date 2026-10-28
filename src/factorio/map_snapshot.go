package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

func LoadMapSnapshotSettings() (MapSnapshotSettings, error) {
	mapSnapshotSettingsMutex.Lock()
	defer mapSnapshotSettingsMutex.Unlock()
	return loadMapSnapshotSettings()
}

func loadMapSnapshotSettings() (MapSnapshotSettings, error) {
	contents, err := os.ReadFile(mapSnapshotSettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return defaultMapSnapshotSettings(), nil
	}
	if err != nil {
		return MapSnapshotSettings{}, fmt.Errorf("read map snapshot settings: %w", err)
	}
	// Start from defaults so settings files written by older releases keep
	// including platforms until an administrator explicitly disables them.
	settings := defaultMapSnapshotSettings()
	if err := json.Unmarshal(contents, &settings); err != nil {
		return MapSnapshotSettings{}, fmt.Errorf("decode map snapshot settings: %w", err)
	}
	if err := validateMapSnapshotSettings(settings); err != nil {
		return MapSnapshotSettings{}, err
	}
	return settings, nil
}

func defaultMapSnapshotSettings() MapSnapshotSettings {
	return MapSnapshotSettings{
		Enabled:                    true,
		IntervalMinutes:            defaultMapSnapshotInterval,
		AutomaticOnlyWhenNoPlayers: false,
		IncludeSpacePlatforms:      true,
	}
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
	settings, err := LoadMapSnapshotSettings()
	if err != nil {
		return MapSnapshotState{}, err
	}
	if !settings.Enabled {
		return MapSnapshotState{Settings: settings}, ErrMapSnapshotsDisabled
	}
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
	if err != nil || !settings.Enabled || settings.IntervalMinutes == 0 {
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
	allowed, err := mapSnapshotAutomaticRunAllowed(settings)
	if err != nil {
		log.Printf("Unable to verify that the Factorio server is empty before a scheduled map snapshot: %v", err)
		return
	}
	if !allowed {
		return
	}
	if _, err := TriggerMapSnapshot(); err != nil && !errors.Is(err, ErrMapSnapshotBusy) {
		log.Printf("Unable to schedule Factorio map snapshot: %v", err)
	}
}

func mapSnapshotAutomaticRunAllowed(settings MapSnapshotSettings) (bool, error) {
	if !settings.AutomaticOnlyWhenNoPlayers || !mapSnapshotServerBusy() {
		return true, nil
	}
	if !mapSnapshotServerRunning() {
		return false, nil
	}
	players, err := mapSnapshotConnectedPlayerCount()
	if err != nil {
		return false, err
	}
	return players == 0, nil
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
	settings, err := LoadMapSnapshotSettings()
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return ErrMapSnapshotsDisabled
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
	if err := prepareMapSnapshotMods(directories["mods"], modsDirectory, profile.InstalledVersion, settings.IncludeSpacePlatforms); err != nil {
		return fmt.Errorf("prepare isolated mods: %w", err)
	}
	writeDataDirectory := filepath.Join(workDirectory, "write-data")
	if err := os.MkdirAll(writeDataDirectory, 0700); err != nil {
		return err
	}
	configPath := filepath.Join(workDirectory, "config.ini")
	if err := writeIsolatedFactorioConfig(configPath, bootstrap.GetConfig().FactorioDir, writeDataDirectory); err != nil {
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
