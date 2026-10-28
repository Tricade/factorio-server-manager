package factorio

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
