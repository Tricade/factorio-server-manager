package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	checkpointSchemaVersion = 1
	checkpointStateFile     = "state.json"
	checkpointFilesDir      = "files"

	CheckpointTriggerManual     = "manual"
	CheckpointTriggerInterval   = "interval"
	CheckpointTriggerLastPlayer = "last-player"
	CheckpointTriggerCleanStop  = "clean-stop"

	defaultCheckpointIntervalMinutes = 30
	defaultCheckpointRetention       = 10
	minimumCheckpointIntervalMinutes = 5
	maximumCheckpointIntervalMinutes = 7 * 24 * 60
	maximumCheckpointRetention       = 1000
)

var (
	ErrCheckpointNotFound       = errors.New("checkpoint not found")
	ErrInvalidCheckpointSetting = errors.New("invalid checkpoint settings")
	ErrCheckpointServerActive   = errors.New("stop Factorio before restoring a checkpoint")
	ErrCheckpointCooldown       = errors.New("an automated checkpoint was created recently")
)

type CheckpointSettings struct {
	IntervalEnabled   bool `json:"interval_enabled"`
	IntervalMinutes   int  `json:"interval_minutes"`
	LastPlayerEnabled bool `json:"last_player_enabled"`
	CleanStopEnabled  bool `json:"clean_stop_enabled"`
	RetentionCount    int  `json:"retention_count"`
}

type Checkpoint struct {
	ID         string    `json:"id"`
	FileName   string    `json:"file_name"`
	CreatedAt  time.Time `json:"created_at"`
	Trigger    string    `json:"trigger"`
	Size       int64     `json:"size"`
	SourceSave string    `json:"source_save,omitempty"`
}

type CheckpointState struct {
	ProfileID        string             `json:"profile_id"`
	Settings         CheckpointSettings `json:"settings"`
	Checkpoints      []Checkpoint       `json:"checkpoints"`
	LastError        string             `json:"last_error,omitempty"`
	LastErrorAt      *time.Time         `json:"last_error_at,omitempty"`
	LastCheckpointAt *time.Time         `json:"last_checkpoint_at,omitempty"`
}

type checkpointStore struct {
	SchemaVersion    int                `json:"schema_version"`
	Settings         CheckpointSettings `json:"settings"`
	LastError        string             `json:"last_error,omitempty"`
	LastErrorAt      *time.Time         `json:"last_error_at,omitempty"`
	LastCheckpointAt *time.Time         `json:"last_checkpoint_at,omitempty"`
}

type checkpointMetadata struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	Trigger       string    `json:"trigger"`
	SourceSave    string    `json:"source_save,omitempty"`
}

var checkpointOperationMutex sync.Mutex
var checkpointNow = time.Now
var checkpointAutomatedCooldown = 2 * time.Minute
var checkpointRootPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), "checkpoints")
}
var checkpointSavesPath = func() string { return profileActiveDirectories()["saves"] }
var checkpointRunRCON = runCheckpointRCONCommand
var checkpointCreateLiveSave = createLiveCheckpointSave
var checkpointIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-(manual|interval|last-player|clean-stop)(-[0-9]+)?$`)

func defaultCheckpointSettings() CheckpointSettings {
	return CheckpointSettings{
		IntervalMinutes: defaultCheckpointIntervalMinutes,
		RetentionCount:  defaultCheckpointRetention,
	}
}

func GetCheckpointState() (CheckpointState, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()
	return getCheckpointStateUnlocked()
}

func UpdateCheckpointSettings(settings CheckpointSettings) (CheckpointState, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()

	if err := validateCheckpointSettings(settings); err != nil {
		return CheckpointState{}, err
	}
	profileID, err := activeProfileID()
	if err != nil {
		return CheckpointState{}, err
	}
	store, err := loadCheckpointStore(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	store.Settings = settings
	if err := saveCheckpointStore(profileID, store); err != nil {
		return CheckpointState{}, err
	}
	return checkpointStateForProfile(profileID)
}

func CreateCheckpoint(trigger string) (CheckpointState, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()

	if !validCheckpointTrigger(trigger) {
		return CheckpointState{}, fmt.Errorf("invalid checkpoint trigger %q", trigger)
	}
	profileID, err := activeProfileID()
	if err != nil {
		return CheckpointState{}, err
	}
	store, err := loadCheckpointStore(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	now := checkpointNow().UTC()
	if trigger != CheckpointTriggerManual && store.LastCheckpointAt != nil && now.Sub(*store.LastCheckpointAt) < checkpointAutomatedCooldown {
		return checkpointStateForProfile(profileID)
	}

	id, err := newCheckpointID(profileID, trigger, now)
	if err != nil {
		return CheckpointState{}, err
	}
	sourcePath, sourceSave, cleanup, err := checkpointSource(id)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		_ = recordCheckpointErrorUnlocked(profileID, err)
		return CheckpointState{}, err
	}

	metadata := checkpointMetadata{
		SchemaVersion: checkpointSchemaVersion,
		ID:            id,
		CreatedAt:     now,
		Trigger:       trigger,
		SourceSave:    sourceSave,
	}
	if err := persistCheckpoint(profileID, metadata, sourcePath); err != nil {
		_ = recordCheckpointErrorUnlocked(profileID, err)
		return CheckpointState{}, err
	}
	store.LastCheckpointAt = &now
	store.LastError = ""
	store.LastErrorAt = nil
	if err := saveCheckpointStore(profileID, store); err != nil {
		return CheckpointState{}, err
	}
	if err := enforceCheckpointRetention(profileID, store.Settings.RetentionCount); err != nil {
		log.Printf("Checkpoint %s was created, but retention cleanup failed: %v", id, err)
		_ = recordCheckpointErrorUnlocked(profileID, err)
	}
	return checkpointStateForProfile(profileID)
}

func RestoreCheckpoint(id string) (Save, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()

	server := GetFactorioServer()
	if server.GetRunning() || server.IsStopping() {
		return Save{}, ErrCheckpointServerActive
	}
	profileID, err := activeProfileID()
	if err != nil {
		return Save{}, err
	}
	checkpoint, sourcePath, err := findCheckpoint(profileID, id)
	if err != nil {
		return Save{}, err
	}
	if err := verifyCheckpointZip(sourcePath); err != nil {
		return Save{}, fmt.Errorf("verify checkpoint: %w", err)
	}

	destinationName, err := availableRestoredSaveName(checkpoint.ID)
	if err != nil {
		return Save{}, err
	}
	destinationPath := filepath.Join(checkpointSavesPath(), destinationName)
	if err := copyCheckpointAtomically(sourcePath, destinationPath); err != nil {
		return Save{}, fmt.Errorf("restore checkpoint: %w", err)
	}
	info, err := os.Stat(destinationPath)
	if err != nil {
		return Save{}, err
	}
	return Save{Name: destinationName, LastMod: info.ModTime(), Size: info.Size()}, nil
}

func DeleteCheckpoint(id string) (CheckpointState, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()
	profileID, err := activeProfileID()
	if err != nil {
		return CheckpointState{}, err
	}
	checkpoint, path, err := findCheckpoint(profileID, id)
	if err != nil {
		return CheckpointState{}, err
	}
	if err := os.Remove(path); err != nil {
		return CheckpointState{}, err
	}
	if err := os.Remove(checkpointMetadataPath(profileID, checkpoint.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return CheckpointState{}, err
	}
	remaining, err := listCheckpoints(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	store, err := loadCheckpointStore(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	if len(remaining) == 0 {
		store.LastCheckpointAt = nil
	} else {
		latest := remaining[0].CreatedAt.UTC()
		store.LastCheckpointAt = &latest
	}
	if err := saveCheckpointStore(profileID, store); err != nil {
		return CheckpointState{}, err
	}
	return checkpointStateForProfile(profileID)
}

func FindCheckpointFile(id string) (Checkpoint, string, error) {
	checkpointOperationMutex.Lock()
	defer checkpointOperationMutex.Unlock()
	profileID, err := activeProfileID()
	if err != nil {
		return Checkpoint{}, "", err
	}
	return findCheckpoint(profileID, id)
}

func getCheckpointStateUnlocked() (CheckpointState, error) {
	profileID, err := activeProfileID()
	if err != nil {
		return CheckpointState{}, err
	}
	return checkpointStateForProfile(profileID)
}

func checkpointStateForProfile(profileID string) (CheckpointState, error) {
	store, err := loadCheckpointStore(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	checkpoints, err := listCheckpoints(profileID)
	if err != nil {
		return CheckpointState{}, err
	}
	return CheckpointState{
		ProfileID:        profileID,
		Settings:         store.Settings,
		Checkpoints:      checkpoints,
		LastError:        store.LastError,
		LastErrorAt:      store.LastErrorAt,
		LastCheckpointAt: store.LastCheckpointAt,
	}, nil
}

func activeProfileID() (string, error) {
	profileMutex.Lock()
	defer profileMutex.Unlock()
	manifest, err := loadProfileManifest()
	if err != nil {
		return "", err
	}
	if err := validateProfileID(manifest.ActiveProfileID); err != nil {
		return "", errors.New("active profile metadata is invalid")
	}
	return manifest.ActiveProfileID, nil
}

func validateCheckpointSettings(settings CheckpointSettings) error {
	if settings.IntervalMinutes < minimumCheckpointIntervalMinutes || settings.IntervalMinutes > maximumCheckpointIntervalMinutes {
		return fmt.Errorf("%w: interval must be between %d and %d minutes", ErrInvalidCheckpointSetting, minimumCheckpointIntervalMinutes, maximumCheckpointIntervalMinutes)
	}
	if settings.RetentionCount < 0 || settings.RetentionCount > maximumCheckpointRetention {
		return fmt.Errorf("%w: retention must be between 0 and %d", ErrInvalidCheckpointSetting, maximumCheckpointRetention)
	}
	return nil
}

func validCheckpointTrigger(trigger string) bool {
	switch trigger {
	case CheckpointTriggerManual, CheckpointTriggerInterval, CheckpointTriggerLastPlayer, CheckpointTriggerCleanStop:
		return true
	default:
		return false
	}
}
