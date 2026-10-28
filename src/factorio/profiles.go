package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	profileSchemaVersion = 1
	profileManifestName  = "manifest.json"
	defaultProfileName   = "Current setup"

	ProfileSourceClone = "clone"
	ProfileSourceEmpty = "empty"
)

var (
	ErrProfileNotFound     = errors.New("profile not found")
	ErrProfileNameConflict = errors.New("a profile with this name already exists")
	ErrActiveProfileDelete = errors.New("the active profile cannot be deleted")
	ErrProfileServerActive = errors.New("stop Factorio before changing profiles")
	ErrInvalidProfile      = errors.New("invalid profile")
)

type Profile struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description,omitempty"`
	Active             bool      `json:"active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ReleaseTarget      string    `json:"release_target"`
	InstalledVersion   string    `json:"installed_version"`
	GameMode           GameMode  `json:"game_mode"`
	EnabledBuiltInMods []string  `json:"enabled_built_in_mods"`
	SelectedSave       string    `json:"selected_save,omitempty"`
	BindIP             string    `json:"bind_ip"`
	Port               int       `json:"port"`
	SaveCount          int       `json:"save_count"`
	ModCount           int       `json:"mod_count"`
}

type ProfileState struct {
	SchemaVersion   int       `json:"schema_version"`
	ActiveProfileID string    `json:"active_profile_id"`
	Profiles        []Profile `json:"profiles"`
}

type profileManifest struct {
	SchemaVersion   int       `json:"schema_version"`
	ActiveProfileID string    `json:"active_profile_id"`
	Profiles        []Profile `json:"profiles"`
}

var profileDataGate sync.RWMutex
var profileMutex sync.Mutex
var profileRename = os.Rename
var profileNow = time.Now
var profileRootPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), "profiles")
}
var profileActiveDirectories = func() map[string]string {
	config := bootstrap.GetConfig()
	return map[string]string{
		"saves":  config.FactorioSavesDir,
		"mods":   config.FactorioModsDir,
		"config": config.FactorioConfigDir,
	}
}
var profileLoadRuntimeState = LoadRuntimeState
var profilePersistRuntimeState = persistRuntimeState
var profileInstallRelease = InstallProfileRelease
var profileReloadServer = ReloadFactorioServerProfile
var profileGetGameModeStatus = GetGameModeStatus
var profileSettingsFilePath = func() string { return bootstrap.GetConfig().SettingsFile }

// LockProfileDataRead prevents a profile activation from replacing active
// directories while an API request is reading or mutating them.
func LockProfileDataRead() func() {
	profileDataGate.RLock()
	return profileDataGate.RUnlock
}

func InitializeProfiles() error {
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := ensureActiveProfileDirectories(); err != nil {
		return err
	}
	if manifest, err := loadProfileManifest(); err == nil {
		index := profileIndex(manifest, manifest.ActiveProfileID)
		if index < 0 {
			return errors.New("active profile metadata is missing")
		}
		active := manifest.Profiles[index]
		if err := profileReloadServer(active.BindIP, active.Port, active.SelectedSave); err != nil {
			return fmt.Errorf("restore active profile startup settings: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	id, err := newProfileID()
	if err != nil {
		return err
	}
	now := profileNow().UTC()
	profile := Profile{
		ID:        id,
		Name:      defaultProfileName,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	profile, err = captureActiveProfile(profile)
	if err != nil {
		return fmt.Errorf("inspect existing setup for profile migration: %w", err)
	}
	if err := snapshotActiveProfile(id); err != nil {
		return fmt.Errorf("migrate existing setup into %q profile: %w", defaultProfileName, err)
	}

	manifest := profileManifest{
		SchemaVersion:   profileSchemaVersion,
		ActiveProfileID: id,
		Profiles:        []Profile{profile},
	}
	if err := saveProfileManifest(manifest); err != nil {
		_ = os.RemoveAll(profileDirectory(id))
		return err
	}
	log.Printf("Migrated the existing Factorio setup into profile %q (%s)", defaultProfileName, id)
	return nil
}

func ListProfiles() (ProfileState, error) {
	profileDataGate.RLock()
	defer profileDataGate.RUnlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	for index := range manifest.Profiles {
		if manifest.Profiles[index].ID != manifest.ActiveProfileID {
			continue
		}
		refreshed, refreshErr := captureActiveProfile(manifest.Profiles[index])
		if refreshErr != nil {
			return ProfileState{}, refreshErr
		}
		manifest.Profiles[index] = refreshed
		break
	}
	return stateFromManifest(manifest), nil
}

func DeleteProfile(id string) (ProfileState, error) {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := validateProfileID(id); err != nil {
		return ProfileState{}, ErrProfileNotFound
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	index := profileIndex(manifest, id)
	if index < 0 {
		return ProfileState{}, ErrProfileNotFound
	}
	if manifest.ActiveProfileID == id {
		return ProfileState{}, ErrActiveProfileDelete
	}

	directory := profileDirectory(id)
	tombstone := directory + ".delete-" + fmt.Sprint(profileNow().UTC().UnixNano())
	if err := profileRename(directory, tombstone); err != nil {
		return ProfileState{}, fmt.Errorf("stage profile deletion: %w", err)
	}
	manifest.Profiles = append(manifest.Profiles[:index], manifest.Profiles[index+1:]...)
	if err := saveProfileManifest(manifest); err != nil {
		if restoreErr := profileRename(tombstone, directory); restoreErr != nil {
			return ProfileState{}, fmt.Errorf("save profile deletion: %w (restore failed: %v)", err, restoreErr)
		}
		return ProfileState{}, err
	}
	if err := os.RemoveAll(tombstone); err != nil {
		log.Printf("Profile %s was deleted but its staged directory could not be removed: %v", id, err)
	}
	if err := deleteProfileCheckpoints(id); err != nil {
		log.Printf("Profile %s was deleted but its checkpoints could not be removed: %v", id, err)
	}
	if err := deleteProfileMapSnapshot(id); err != nil {
		log.Printf("Profile %s was deleted but its map snapshot could not be removed: %v", id, err)
	}
	return stateFromManifest(manifest), nil
}

func ensureProfileServerStopped() error {
	server := GetFactorioServer()
	if server.IsBusy() || WorldGenerationBusy() {
		return ErrProfileServerActive
	}
	return nil
}
