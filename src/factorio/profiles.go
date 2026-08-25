package factorio

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

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

func CreateProfile(name, description, source string) (ProfileState, error) {
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := ensureProfileServerStopped(); err != nil {
		return ProfileState{}, err
	}
	name, description, err := validateProfileText(name, description)
	if err != nil {
		return ProfileState{}, err
	}
	if source == "" {
		source = ProfileSourceClone
	}
	if source != ProfileSourceClone && source != ProfileSourceEmpty {
		return ProfileState{}, fmt.Errorf("%w: unsupported profile source %q", ErrInvalidProfile, source)
	}

	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	if profileNameExists(manifest, name, "") {
		return ProfileState{}, ErrProfileNameConflict
	}

	id, err := newProfileID()
	if err != nil {
		return ProfileState{}, err
	}
	now := profileNow().UTC()
	profile := Profile{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	profile, err = captureActiveProfile(profile)
	if err != nil {
		return ProfileState{}, err
	}
	profile.Active = false

	switch source {
	case ProfileSourceClone:
		if err := snapshotActiveProfile(id); err != nil {
			return ProfileState{}, fmt.Errorf("clone current setup: %w", err)
		}
	case ProfileSourceEmpty:
		if err := createEmptyProfileData(id); err != nil {
			return ProfileState{}, fmt.Errorf("create empty profile data: %w", err)
		}
		profile.GameMode = GameModeFactorio
		profile.EnabledBuiltInMods = []string{}
		profile.SelectedSave = ""
		profile.SaveCount = 0
		profile.ModCount = 0
	}

	manifest.Profiles = append(manifest.Profiles, profile)
	if err := saveProfileManifest(manifest); err != nil {
		_ = os.RemoveAll(profileDirectory(id))
		return ProfileState{}, err
	}
	return stateFromManifest(manifest), nil
}

func UpdateProfile(id, name, description string) (ProfileState, error) {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := validateProfileID(id); err != nil {
		return ProfileState{}, ErrProfileNotFound
	}
	name, description, err := validateProfileText(name, description)
	if err != nil {
		return ProfileState{}, err
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	index := profileIndex(manifest, id)
	if index < 0 {
		return ProfileState{}, ErrProfileNotFound
	}
	if profileNameExists(manifest, name, id) {
		return ProfileState{}, ErrProfileNameConflict
	}
	manifest.Profiles[index].Name = name
	manifest.Profiles[index].Description = description
	manifest.Profiles[index].UpdatedAt = profileNow().UTC()
	if err := saveProfileManifest(manifest); err != nil {
		return ProfileState{}, err
	}
	return stateFromManifest(manifest), nil
}

// UpdateProfileStartup persists the network binding and save that will be used
// the next time the active profile is started. Factorio must be stopped so the
// stored profile state and the in-memory server configuration cannot diverge.
func UpdateProfileStartup(id, bindIP string, port int, selectedSave string) (ProfileState, error) {
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := ensureProfileServerStopped(); err != nil {
		return ProfileState{}, err
	}
	if err := validateProfileID(id); err != nil {
		return ProfileState{}, ErrProfileNotFound
	}

	bindIP = strings.TrimSpace(bindIP)
	parsedIP := net.ParseIP(bindIP)
	if parsedIP == nil || parsedIP.To4() == nil {
		return ProfileState{}, fmt.Errorf("%w: invalid IPv4 bind address", ErrInvalidProfile)
	}
	if port < 1 || port > 65535 {
		return ProfileState{}, fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidProfile)
	}

	normalizedSelectedSave, err := normalizeProfileStartupSave(profileActiveDirectories()["saves"], selectedSave)
	if err != nil {
		return ProfileState{}, err
	}
	selectedSave = normalizedSelectedSave

	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	if manifest.ActiveProfileID != id {
		return ProfileState{}, fmt.Errorf("%w: startup settings belong to the active profile", ErrInvalidProfile)
	}
	index := profileIndex(manifest, id)
	if index < 0 {
		return ProfileState{}, ErrProfileNotFound
	}

	server := GetFactorioServer()
	previous := server.Snapshot()
	if err := server.ConfigureStart(bindIP, port, selectedSave); err != nil {
		return ProfileState{}, err
	}
	refreshed, err := captureActiveProfile(manifest.Profiles[index])
	if err != nil {
		_ = server.ConfigureStart(previous.BindIP, previous.Port, previous.Savefile)
		return ProfileState{}, fmt.Errorf("capture startup settings: %w", err)
	}
	manifest.Profiles[index] = refreshed
	if err := saveProfileManifest(manifest); err != nil {
		_ = server.ConfigureStart(previous.BindIP, previous.Port, previous.Savefile)
		return ProfileState{}, err
	}
	return stateFromManifest(manifest), nil
}

func normalizeProfileStartupSave(directory, requested string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("list profile saves: %w", err)
	}
	requested = strings.TrimSpace(requested)
	var latest os.FileInfo
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return "", fmt.Errorf("inspect profile save: %w", infoErr)
		}
		if !isUsableSave(info) {
			continue
		}
		if latest == nil || latest.ModTime().Before(info.ModTime()) {
			latest = info
		}
	}
	if latest == nil {
		if requested == "" {
			return "", nil
		}
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	if strings.HasPrefix(requested, "Load Latest") {
		return latest.Name(), nil
	}
	if requested == "" {
		return "", fmt.Errorf("%w: select a save", ErrInvalidProfile)
	}
	if err := ValidatePathElement(requested); err != nil {
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	info, err := os.Stat(filepath.Join(directory, requested))
	if err != nil || !isUsableSave(info) {
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	return requested, nil
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

func ActivateProfile(id string) (ProfileState, error) {
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := ensureProfileServerStopped(); err != nil {
		return ProfileState{}, err
	}
	if err := validateProfileID(id); err != nil {
		return ProfileState{}, ErrProfileNotFound
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	targetIndex := profileIndex(manifest, id)
	if targetIndex < 0 {
		return ProfileState{}, ErrProfileNotFound
	}
	if manifest.ActiveProfileID == id {
		return stateFromManifest(manifest), nil
	}
	currentIndex := profileIndex(manifest, manifest.ActiveProfileID)
	if currentIndex < 0 {
		return ProfileState{}, errors.New("active profile metadata is missing")
	}

	current, err := captureActiveProfile(manifest.Profiles[currentIndex])
	if err != nil {
		return ProfileState{}, fmt.Errorf("capture active profile metadata: %w", err)
	}
	if err := snapshotActiveProfile(current.ID); err != nil {
		return ProfileState{}, fmt.Errorf("snapshot active profile before switch: %w", err)
	}
	manifest.Profiles[currentIndex] = current
	target := manifest.Profiles[targetIndex]
	if err := validateStoredProfileData(target); err != nil {
		return ProfileState{}, fmt.Errorf("validate target profile: %w", err)
	}

	swaps, err := prepareProfileSwaps(target.ID)
	if err != nil {
		return ProfileState{}, err
	}
	defer cleanupProfileSwaps(swaps)

	releaseChanged := !versionsEqual(current.InstalledVersion, target.InstalledVersion)
	if releaseChanged {
		if err := profileInstallRelease(target.InstalledVersion, target.ReleaseTarget, target.EnabledBuiltInMods); err != nil {
			return ProfileState{}, fmt.Errorf("restore Factorio %s for profile %q: %w", target.InstalledVersion, target.Name, err)
		}
	}

	activated := make([]*directoryEntrySwap, 0, len(swaps))
	for _, swap := range swaps {
		if err := swap.activate(); err != nil {
			rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
			if rollbackErr != nil {
				return ProfileState{}, fmt.Errorf("activate profile data: %w (rollback failed: %v)", err, rollbackErr)
			}
			return ProfileState{}, fmt.Errorf("activate profile data: %w (previous profile restored)", err)
		}
		activated = append(activated, swap)
	}

	if err := setBuiltInFeatureMods(profileActiveDirectories()["mods"], target.EnabledBuiltInMods); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("restore profile game mode: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("restore profile game mode: %w (previous profile restored)", err)
	}

	if err := profileReloadServer(target.BindIP, target.Port, target.SelectedSave); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("reload target profile: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("reload target profile: %w (previous profile restored)", err)
	}
	if err := profilePersistRuntimeState(target.ReleaseTarget, target.InstalledVersion); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("persist target runtime: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("persist target runtime: %w (previous profile restored)", err)
	}

	now := profileNow().UTC()
	manifest.ActiveProfileID = target.ID
	manifest.Profiles[currentIndex] = current
	manifest.Profiles[targetIndex].UpdatedAt = now
	if err := saveProfileManifest(manifest); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("commit profile selection: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("commit profile selection: %w (previous profile restored)", err)
	}

	for _, swap := range swaps {
		if err := swap.commit(); err != nil {
			log.Printf("Profile %s is active but temporary switch data could not be removed: %v", target.ID, err)
		}
	}
	log.Printf("Activated Factorio profile %q (%s); Factorio remains stopped", target.Name, target.ID)
	return stateFromManifest(manifest), nil
}

func rollbackProfileSwitch(activated []*directoryEntrySwap, current Profile, releaseChanged bool) error {
	var rollbackErrors []error
	for index := len(activated) - 1; index >= 0; index-- {
		if err := activated[index].rollback(); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if releaseChanged {
		if err := profileInstallRelease(current.InstalledVersion, current.ReleaseTarget, current.EnabledBuiltInMods); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous Factorio release: %w", err))
		}
	} else if err := profilePersistRuntimeState(current.ReleaseTarget, current.InstalledVersion); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous runtime metadata: %w", err))
	}
	if err := profileReloadServer(current.BindIP, current.Port, current.SelectedSave); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("reload previous profile: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

func ensureProfileServerStopped() error {
	server := GetFactorioServer()
	if server.IsBusy() || WorldGenerationBusy() {
		return ErrProfileServerActive
	}
	return nil
}

func captureActiveProfile(profile Profile) (Profile, error) {
	runtimeState, err := profileLoadRuntimeState()
	if err != nil {
		return Profile{}, err
	}
	snapshot := GetFactorioServer().Snapshot()
	installedVersion := runtimeState.InstalledVersion
	if installedVersion == "" {
		installedVersion = snapshot.BaseModVersion
	}
	if installedVersion == "" {
		installedVersion = snapshot.Version.ReleaseString()
	}
	installedVersion, err = NormalizeExactReleaseVersion(installedVersion)
	if err != nil {
		return Profile{}, err
	}
	releaseTarget := runtimeState.ReleaseTarget
	if releaseTarget == "" {
		releaseTarget = installedVersion
	}
	releaseTarget, err = NormalizeReleaseTarget(releaseTarget)
	if err != nil {
		return Profile{}, err
	}

	mode, err := profileGetGameModeStatus()
	if err != nil {
		return Profile{}, err
	}
	enabledBuiltIns := make([]string, 0, len(mode.Features))
	for _, feature := range mode.Features {
		if feature.Enabled {
			enabledBuiltIns = append(enabledBuiltIns, feature.Name)
		}
	}
	sort.Strings(enabledBuiltIns)

	directories := profileActiveDirectories()
	saveCount, err := countUsableSaves(directories["saves"])
	if err != nil {
		return Profile{}, err
	}
	modCount, err := countInstalledModArchives(directories["mods"])
	if err != nil {
		return Profile{}, err
	}
	selectedSave := snapshot.Savefile
	if saveCount == 0 {
		selectedSave = ""
	} else if selectedSave == "" {
		if latest, latestErr := GetLatestSave(); latestErr == nil {
			selectedSave = latest.Name
		}
	}
	bindIP := snapshot.BindIP
	if bindIP == "" {
		bindIP = bootstrap.GetConfig().FactorioIP
	}
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	port := snapshot.Port
	if port == 0 {
		port = 34197
	}

	profile.Active = true
	profile.ReleaseTarget = releaseTarget
	profile.InstalledVersion = installedVersion
	profile.GameMode = mode.Mode
	profile.EnabledBuiltInMods = enabledBuiltIns
	profile.SelectedSave = selectedSave
	profile.BindIP = bindIP
	profile.Port = port
	profile.SaveCount = saveCount
	profile.ModCount = modCount
	profile.UpdatedAt = profileNow().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = profile.UpdatedAt
	}
	return profile, nil
}

func ensureActiveProfileDirectories() error {
	for name, directory := range profileActiveDirectories() {
		if directory == "" {
			return fmt.Errorf("active %s directory is not configured", name)
		}
		if err := os.MkdirAll(directory, 0755); err != nil {
			return fmt.Errorf("create active %s directory: %w", name, err)
		}
	}
	return nil
}

func countUsableSaves(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if isUsableSave(info) {
			count++
		}
	}
	return count, nil
}

func countInstalledModArchives(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			continue
		}
		count++
	}
	return count, nil
}

func snapshotActiveProfile(id string) error {
	return writeProfileData(id, profileActiveDirectories(), nil)
}

func createEmptyProfileData(id string) error {
	directories := profileActiveDirectories()
	return writeProfileData(id, map[string]string{"config": directories["config"]}, func(root string) error {
		if err := os.MkdirAll(filepath.Join(root, "saves"), 0755); err != nil {
			return err
		}
		modsDirectory := filepath.Join(root, "mods")
		if err := os.MkdirAll(modsDirectory, 0755); err != nil {
			return err
		}
		return setBuiltInFeatureMods(modsDirectory, nil)
	})
}

func writeProfileData(id string, sources map[string]string, initialize func(string) error) error {
	if err := validateProfileID(id); err != nil {
		return err
	}
	root := profileRootPath()
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("create profiles directory: %w", err)
	}
	temporary, err := os.MkdirTemp(root, ".profile-write-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)

	for _, name := range []string{"saves", "mods", "config"} {
		source, ok := sources[name]
		if !ok {
			continue
		}
		if err := copyProfileDirectory(source, filepath.Join(temporary, name)); err != nil {
			return fmt.Errorf("copy profile %s: %w", name, err)
		}
	}
	if initialize != nil {
		if err := initialize(temporary); err != nil {
			return err
		}
	}
	for _, name := range []string{"saves", "mods", "config"} {
		if err := os.MkdirAll(filepath.Join(temporary, name), 0755); err != nil {
			return err
		}
	}

	destination := profileDirectory(id)
	backup := temporary + ".previous"
	hasPrevious := false
	if _, err := os.Stat(destination); err == nil {
		if err := profileRename(destination, backup); err != nil {
			return fmt.Errorf("back up previous profile snapshot: %w", err)
		}
		hasPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := profileRename(temporary, destination); err != nil {
		if hasPrevious {
			_ = profileRename(backup, destination)
		}
		return fmt.Errorf("activate profile snapshot: %w", err)
	}
	if hasPrevious {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous profile snapshot: %w", err)
		}
	}
	return nil
}

func copyProfileDirectory(source, destination string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(destination, 0755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to copy symbolic link %s", source)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", source)
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse to copy symbolic link %s", path)
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to copy non-regular file %s", path)
		}
		return copyProfileFile(path, target, info)
	})
}

func copyProfileFile(sourcePath, destinationPath string, info os.FileInfo) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(destinationPath, info.ModTime(), info.ModTime())
}

// directoryEntrySwap replaces the contents of destination without renaming
// destination itself. Keeping staging and backup directories below the
// destination makes the transaction safe when destination is a Docker bind
// mount or volume mount point.
type directoryEntrySwap struct {
	destination      string
	staging          string
	backup           string
	backedUp         []string
	activated        []string
	isActive         bool
	preserveRecovery bool
}

func prepareProfileSwaps(profileID string) ([]*directoryEntrySwap, error) {
	active := profileActiveDirectories()
	swaps := make([]*directoryEntrySwap, 0, 3)
	for _, name := range []string{"saves", "mods", "config"} {
		swap, err := prepareProfileDirectorySwap(filepath.Join(profileDirectory(profileID), name), active[name])
		if err != nil {
			cleanupProfileSwaps(swaps)
			return nil, fmt.Errorf("stage profile %s: %w", name, err)
		}
		swaps = append(swaps, swap)
	}
	return swaps, nil
}

func prepareProfileDirectorySwap(source, destination string) (*directoryEntrySwap, error) {
	swap, err := newDirectoryEntrySwap(destination, ".profile-staging-", ".profile-backup-")
	if err != nil {
		return nil, err
	}
	if err := copyProfileDirectory(source, swap.staging); err != nil {
		_ = swap.cleanup()
		return nil, err
	}
	return swap, nil
}

func newDirectoryEntrySwap(destination, stagingPattern, backupPattern string) (*directoryEntrySwap, error) {
	if err := os.MkdirAll(destination, 0755); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(destination, stagingPattern)
	if err != nil {
		return nil, err
	}
	backup, err := os.MkdirTemp(destination, backupPattern)
	if err != nil {
		_ = os.RemoveAll(staging)
		return nil, err
	}
	return &directoryEntrySwap{destination: destination, staging: staging, backup: backup}, nil
}

func (swap *directoryEntrySwap) activate() error {
	ignored := map[string]bool{filepath.Base(swap.staging): true, filepath.Base(swap.backup): true}
	backedUp, err := moveProfileEntries(swap.destination, swap.backup, ignored)
	swap.backedUp = backedUp
	if err != nil {
		if restoreErr := restoreProfileEntries(swap.backup, swap.destination, backedUp); restoreErr != nil {
			swap.preserveRecovery = true
			return fmt.Errorf("back up active directory: %w (restore failed: %v; recovery data preserved)", err, restoreErr)
		}
		return fmt.Errorf("back up active directory: %w", err)
	}
	activated, err := moveProfileEntries(swap.staging, swap.destination, nil)
	swap.activated = activated
	if err != nil {
		rollbackErr := swap.rollback()
		if rollbackErr != nil {
			return fmt.Errorf("activate staged directory: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("activate staged directory: %w", err)
	}
	swap.isActive = true
	return nil
}

func (swap *directoryEntrySwap) rollback() error {
	var rollbackErrors []error
	if err := restoreProfileEntries(swap.destination, swap.staging, swap.activated); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("remove activated entries: %w", err))
	}
	if err := restoreProfileEntries(swap.backup, swap.destination, swap.backedUp); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous entries: %w", err))
	}
	swap.isActive = false
	rollbackErr := errors.Join(rollbackErrors...)
	swap.preserveRecovery = rollbackErr != nil
	return rollbackErr
}

func (swap *directoryEntrySwap) commit() error {
	swap.isActive = false
	swap.preserveRecovery = false
	return swap.cleanup()
}

func (swap *directoryEntrySwap) cleanup() error {
	if swap.preserveRecovery {
		return nil
	}
	return errors.Join(os.RemoveAll(swap.staging), os.RemoveAll(swap.backup))
}

func cleanupProfileSwaps(swaps []*directoryEntrySwap) {
	for _, swap := range swaps {
		if swap.isActive || swap.preserveRecovery {
			continue
		}
		_ = swap.cleanup()
	}
}

func moveProfileEntries(source, destination string, ignored map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	moved := make([]string, 0, len(entries))
	for _, entry := range entries {
		if ignored != nil && ignored[entry.Name()] {
			continue
		}
		if err := profileRename(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return moved, err
		}
		moved = append(moved, entry.Name())
	}
	return moved, nil
}

func restoreProfileEntries(source, destination string, entries []string) error {
	var restoreErrors []error
	for index := len(entries) - 1; index >= 0; index-- {
		name := entries[index]
		if err := profileRename(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", name, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func validateStoredProfileData(profile Profile) error {
	if err := validateProfileID(profile.ID); err != nil {
		return err
	}
	root := profileDirectory(profile.ID)
	for _, name := range []string{"saves", "mods", "config"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", name)
		}
	}

	activeConfigDirectory := profileActiveDirectories()["config"]
	settingsRelative, err := filepath.Rel(activeConfigDirectory, profileSettingsFilePath())
	if err != nil || settingsRelative == "." || strings.HasPrefix(settingsRelative, ".."+string(filepath.Separator)) {
		return errors.New("server settings path is outside the active config directory")
	}
	settings, err := os.Open(filepath.Join(root, "config", settingsRelative))
	if err != nil {
		return fmt.Errorf("open stored server settings: %w", err)
	}
	defer settings.Close()
	var decodedSettings map[string]interface{}
	if err := json.NewDecoder(settings).Decode(&decodedSettings); err != nil {
		return fmt.Errorf("decode stored server settings: %w", err)
	}

	modListData, err := os.ReadFile(filepath.Join(root, "mods", "mod-list.json"))
	if err != nil {
		return fmt.Errorf("read stored mod list: %w", err)
	}
	var modList ModSimpleList
	if err := json.Unmarshal(modListData, &modList); err != nil {
		return fmt.Errorf("decode stored mod list: %w", err)
	}
	return nil
}

func loadProfileManifest() (profileManifest, error) {
	path := filepath.Join(profileRootPath(), profileManifestName)
	contents, err := os.ReadFile(path)
	if err != nil {
		return profileManifest{}, err
	}
	var manifest profileManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return profileManifest{}, fmt.Errorf("decode profile manifest: %w", err)
	}
	if manifest.SchemaVersion != profileSchemaVersion {
		return profileManifest{}, fmt.Errorf("unsupported profile schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.Profiles) == 0 {
		return profileManifest{}, errors.New("profile manifest has no profiles")
	}
	ids := make(map[string]bool, len(manifest.Profiles))
	names := make(map[string]bool, len(manifest.Profiles))
	activeFound := false
	for index := range manifest.Profiles {
		profile := &manifest.Profiles[index]
		if err := validateProfileID(profile.ID); err != nil || ids[profile.ID] {
			return profileManifest{}, fmt.Errorf("profile manifest contains an invalid or duplicate id")
		}
		ids[profile.ID] = true
		nameKey := strings.ToLower(strings.TrimSpace(profile.Name))
		if nameKey == "" || names[nameKey] {
			return profileManifest{}, errors.New("profile manifest contains an invalid or duplicate name")
		}
		names[nameKey] = true
		profile.Active = profile.ID == manifest.ActiveProfileID
		activeFound = activeFound || profile.Active
		if info, err := os.Stat(profileDirectory(profile.ID)); err != nil || !info.IsDir() {
			return profileManifest{}, fmt.Errorf("profile data for %q is missing", profile.Name)
		}
	}
	if !activeFound {
		return profileManifest{}, errors.New("profile manifest active profile is missing")
	}
	return manifest, nil
}

func saveProfileManifest(manifest profileManifest) error {
	manifest.SchemaVersion = profileSchemaVersion
	for index := range manifest.Profiles {
		manifest.Profiles[index].Active = manifest.Profiles[index].ID == manifest.ActiveProfileID
	}
	root := profileRootPath()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(root, ".profile-manifest-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(root, profileManifestName)); err != nil {
		return fmt.Errorf("activate profile manifest: %w", err)
	}
	return nil
}

func stateFromManifest(manifest profileManifest) ProfileState {
	profiles := append([]Profile(nil), manifest.Profiles...)
	for index := range profiles {
		profiles[index].Active = profiles[index].ID == manifest.ActiveProfileID
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].Active != profiles[j].Active {
			return profiles[i].Active
		}
		return profiles[i].CreatedAt.Before(profiles[j].CreatedAt)
	})
	return ProfileState{
		SchemaVersion:   manifest.SchemaVersion,
		ActiveProfileID: manifest.ActiveProfileID,
		Profiles:        profiles,
	}
}

func profileDirectory(id string) string {
	return filepath.Join(profileRootPath(), id)
}

func newProfileID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate profile id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func validateProfileID(id string) error {
	if len(id) != 16 {
		return errors.New("invalid profile id")
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 8 {
		return errors.New("invalid profile id")
	}
	return nil
}

func validateProfileText(name, description string) (string, string, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", "", fmt.Errorf("%w: profile name must contain 1 to 64 printable characters", ErrInvalidProfile)
	}
	if utf8.RuneCountInString(description) > 500 || strings.IndexFunc(description, unicode.IsControl) >= 0 {
		return "", "", fmt.Errorf("%w: profile description must contain at most 500 printable characters", ErrInvalidProfile)
	}
	return name, description, nil
}

func profileIndex(manifest profileManifest, id string) int {
	for index, profile := range manifest.Profiles {
		if profile.ID == id {
			return index
		}
	}
	return -1
}

func profileNameExists(manifest profileManifest, name, exceptID string) bool {
	for _, profile := range manifest.Profiles {
		if profile.ID != exceptID && strings.EqualFold(strings.TrimSpace(profile.Name), name) {
			return true
		}
	}
	return false
}
