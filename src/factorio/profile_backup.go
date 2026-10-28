package factorio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const profileBackupKind = "factorio-server-control-profile-backup"

type ProfileBackupOptions struct {
	ProfileIDs         []string `json:"profile_ids"`
	IncludeSaves       bool     `json:"include_saves"`
	IncludeCheckpoints bool     `json:"include_checkpoints"`
}

type BackupProfile struct {
	Profile
	CheckpointSettings CheckpointSettings `json:"checkpoint_settings"`
	Checkpoints        []Checkpoint       `json:"checkpoints"`
}

type ProfileBackup struct {
	Kind          string          `json:"kind"`
	FormatVersion int             `json:"format_version"`
	CreatedAt     time.Time       `json:"created_at"`
	Profiles      []BackupProfile `json:"profiles"`
}

type ProfileBackupSelection struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Lock order matches checkpoint calls (checkpoint -> profile). The exclusive
// gate also prevents starts and mutations of live saves/mods during the copy.
func lockProfileBackup() func() {
	profileDataGate.Lock()
	checkpointOperationMutex.Lock()
	profileMutex.Lock()
	return func() { profileMutex.Unlock(); checkpointOperationMutex.Unlock(); profileDataGate.Unlock() }
}

func backupConfigRelative(path string) (string, error) {
	relative, err := filepath.Rel(profileActiveDirectories()["config"], path)
	if err != nil || backupEntryName(filepath.ToSlash(relative)) != nil {
		return "", ErrInvalidBackup
	}
	return relative, nil
}

func backupConfigFiles() (map[string]string, error) {
	relative, err := backupConfigRelative(profileSettingsFilePath())
	if err != nil {
		return nil, err
	}
	files := map[string]string{"server-settings.json": relative}
	// These optional files contain player lists, not manager accounts.
	for _, name := range []string{"server-adminlist.json", "server-banlist.json", "server-whitelist.json"} {
		files[name] = name
	}
	admin := bootstrap.GetConfig().FactorioAdminFile
	if admin != "" {
		if relative, err := backupConfigRelative(admin); err == nil {
			files["server-adminlist.json"] = relative
		}
	}
	return files, nil
}

func sanitizeBackupServerSettings(path string, importing bool) error {
	var settings map[string]interface{}
	if err := readBackupJSON(path, &settings); err != nil || settings == nil {
		return ErrInvalidBackup
	}
	for key := range settings {
		lower := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if lower == "username" || lower == "user_key" || strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			delete(settings, key)
		}
	}
	if importing {
		// An omitted password must not accidentally publish a migrated server.
		settings["visibility"] = map[string]bool{"public": false, "lan": false}
	}
	return writeBackupJSON(path, settings)
}

func copyBackupFlat(source, destination string, allowed func(string) bool, budget *backupCopyBudget) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalidBackup
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			return fmt.Errorf("%w: unpacked directories are not supported in backups", ErrInvalidBackup)
		}
		if !allowed(entry.Name()) {
			continue
		}
		if err := backupEntryName(entry.Name()); err != nil {
			return err
		}
		if err := budget.copy(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func backupModFile(name string) bool {
	return strings.HasSuffix(name, ".zip") || name == "mod-list.json" || name == "mod-settings.dat"
}

func backupSaveFile(name string) bool {
	return strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tmp.zip")
}

// ExportProfileBackup returns a completed private temporary ZIP owned by the
// caller. No manifest, snapshot or runtime state is changed by an export.
func ExportProfileBackup(options ProfileBackupOptions) (string, error) {
	unlock := lockProfileBackup()
	defer unlock()
	if err := ensureProfileServerStopped(); err != nil {
		return "", err
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return "", err
	}
	if len(options.ProfileIDs) == 0 || len(options.ProfileIDs) > 256 {
		return "", ErrInvalidBackup
	}
	configFiles, err := backupConfigFiles()
	if err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp("", "fsm-profile-export-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	backup := ProfileBackup{Kind: profileBackupKind, FormatVersion: 1, CreatedAt: profileNow().UTC()}
	budget := &backupCopyBudget{}
	seen := map[string]bool{}
	for _, requestedID := range options.ProfileIDs {
		index := profileIndex(manifest, requestedID)
		if index < 0 || seen[requestedID] {
			return "", ErrInvalidBackup
		}
		seen[requestedID] = true
		profile := manifest.Profiles[index]
		// Resolve request selections to validated manifest entries before using
		// IDs as storage paths; never derive a path from the request itself.
		id := profile.ID
		sources := map[string]string{}
		for _, dir := range []string{"saves", "mods", "config"} {
			sources[dir] = filepath.Join(profileDirectory(id), dir)
		}
		if id == manifest.ActiveProfileID {
			profile, err = captureActiveProfile(profile)
			if err != nil {
				return "", err
			}
			sources = profileActiveDirectories()
		}
		root := filepath.Join(staging, "profiles", id)
		if err := copyBackupFlat(sources["mods"], filepath.Join(root, "mods"), backupModFile, budget); err != nil {
			return "", err
		}
		if options.IncludeSaves {
			if err := copyBackupFlat(sources["saves"], filepath.Join(root, "saves"), backupSaveFile, budget); err != nil {
				return "", err
			}
		} else {
			profile.SelectedSave = ""
		}
		for canonical, relative := range configFiles {
			source := filepath.Join(sources["config"], relative)
			if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) && canonical != "server-settings.json" {
				continue
			}
			if err := budget.copy(source, filepath.Join(root, "config", canonical)); err != nil {
				return "", err
			}
		}
		if err := sanitizeBackupServerSettings(filepath.Join(root, "config", "server-settings.json"), false); err != nil {
			return "", err
		}
		state, err := loadCheckpointStore(id)
		if err != nil {
			return "", err
		}
		item := BackupProfile{Profile: profile, CheckpointSettings: state.Settings, Checkpoints: []Checkpoint{}}
		item.Active = false
		item.BindIP = "" // host-specific, never restore blindly
		if options.IncludeCheckpoints {
			item.Checkpoints, err = listCheckpoints(id)
			if err != nil {
				return "", err
			}
			for _, checkpoint := range item.Checkpoints {
				if err := budget.copy(filepath.Join(checkpointFilesDirectory(id), filepath.Base(checkpoint.FileName)), filepath.Join(root, "checkpoints", filepath.Base(checkpoint.FileName))); err != nil {
					return "", err
				}
			}
		}
		if err := validateBackupProfile(root, &item); err != nil {
			return "", err
		}
		backup.Profiles = append(backup.Profiles, item)
	}
	if err := writeBackupJSON(filepath.Join(staging, "backup.json"), backup); err != nil {
		return "", err
	}
	return zipBackupDirectory(staging)
}

// ImportProfileBackup commits only newly allocated directories. The existing
// schema, active ID, runtime selection and manager-wide state stay untouched.
func ImportProfileBackup(input io.ReaderAt, size int64, selections []ProfileBackupSelection) (ProfileState, error) {
	if len(selections) == 0 || len(selections) > 256 {
		return ProfileState{}, ErrInvalidBackup
	}
	backup, staging, err := stageProfileBackup(input, size)
	if err != nil {
		return ProfileState{}, err
	}
	defer os.RemoveAll(staging)
	unlock := lockProfileBackup()
	defer unlock()
	if err := ensureProfileServerStopped(); err != nil {
		return ProfileState{}, err
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	configFiles, err := backupConfigFiles()
	if err != nil {
		return ProfileState{}, err
	}
	created := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, id := range created {
				os.RemoveAll(profileDirectory(id))
				os.RemoveAll(checkpointProfileDirectory(id))
			}
		}
	}()
	seen := map[string]bool{}
	for _, selection := range selections {
		var item *BackupProfile
		for index := range backup.Profiles {
			if backup.Profiles[index].ID == selection.ID {
				item = &backup.Profiles[index]
				break
			}
		}
		if item == nil || seen[selection.ID] {
			return ProfileState{}, ErrInvalidBackup
		}
		seen[selection.ID] = true
		name, description, err := validateProfileText(selection.Name, item.Description)
		if err != nil {
			return ProfileState{}, err
		}
		if profileNameExists(manifest, name, "") {
			return ProfileState{}, ErrProfileNameConflict
		}
		profile := item.Profile
		profile.ID, err = newProfileID()
		if err != nil {
			return ProfileState{}, err
		}
		if _, err := os.Lstat(checkpointProfileDirectory(profile.ID)); !errors.Is(err, os.ErrNotExist) {
			return ProfileState{}, errors.New("new profile checkpoint destination already exists or is unavailable")
		}
		// Mkdir (not MkdirAll) guarantees even an ID collision cannot overwrite.
		if err := os.Mkdir(profileDirectory(profile.ID), 0700); err != nil {
			return ProfileState{}, err
		}
		created = append(created, profile.ID)
		root := filepath.Join(staging, "profiles", item.ID)
		profile.Name = name
		profile.Description = description
		profile.Active = false
		profile.CreatedAt = profileNow().UTC()
		profile.UpdatedAt = profile.CreatedAt
		current := GetFactorioServer().Snapshot()
		profile.BindIP = current.BindIP
		profile.Port = current.Port
		if profile.BindIP == "" {
			profile.BindIP = "0.0.0.0"
		}
		if profile.Port == 0 {
			profile.Port = 34197
		}
		if err := writeProfileData(profile.ID, map[string]string{"saves": filepath.Join(root, "saves"), "mods": filepath.Join(root, "mods")}, func(destination string) error {
			for canonical, relative := range configFiles {
				source := filepath.Join(root, "config", canonical)
				if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err := (&backupCopyBudget{}).copy(source, filepath.Join(destination, "config", relative)); err != nil {
					return err
				}
			}
			// Keep destination-local engine paths, never import a source host's INI.
			ini := filepath.Join(profileActiveDirectories()["config"], "config.ini")
			if _, err := os.Lstat(ini); err == nil {
				if err := (&backupCopyBudget{}).copy(ini, filepath.Join(destination, "config", "config.ini")); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return sanitizeBackupServerSettings(filepath.Join(destination, "config", configFiles["server-settings.json"]), true)
		}); err != nil {
			return ProfileState{}, err
		}
		if err := validateStoredProfileData(profile); err != nil {
			return ProfileState{}, err
		}
		if err := saveCheckpointStore(profile.ID, checkpointStore{SchemaVersion: checkpointSchemaVersion, Settings: item.CheckpointSettings}); err != nil {
			return ProfileState{}, err
		}
		for _, checkpoint := range item.Checkpoints {
			metadata := checkpointMetadata{SchemaVersion: checkpointSchemaVersion, ID: checkpoint.ID, CreatedAt: checkpoint.CreatedAt, Trigger: checkpoint.Trigger, SourceSave: checkpoint.SourceSave}
			if err := persistCheckpoint(profile.ID, metadata, filepath.Join(root, "checkpoints", filepath.Base(checkpoint.FileName))); err != nil {
				return ProfileState{}, err
			}
		}
		manifest.Profiles = append(manifest.Profiles, profile)
	}
	if err := saveProfileManifest(manifest); err != nil {
		return ProfileState{}, err
	}
	committed = true
	return stateFromManifest(manifest), nil
}
