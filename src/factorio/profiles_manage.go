package factorio

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

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
	if !filepath.IsLocal(requested) {
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	if err := ValidatePathElement(requested); err != nil {
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	requested = filepath.Base(requested)
	info, err := os.Stat(filepath.Join(directory, requested))
	if err != nil || !isUsableSave(info) {
		return "", fmt.Errorf("%w: selected save does not exist", ErrInvalidProfile)
	}
	return requested, nil
}
