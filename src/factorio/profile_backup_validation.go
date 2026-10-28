package factorio

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func profileBackupEntryAllowed(name string) bool {
	if name == "backup.json" {
		return true
	}
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "profiles" || validateProfileID(parts[1]) != nil {
		return false
	}
	switch parts[2] {
	case "saves":
		return backupSaveFile(parts[3])
	case "mods":
		return backupModFile(parts[3])
	case "config":
		return parts[3] == "server-settings.json" || parts[3] == "server-adminlist.json" || parts[3] == "server-banlist.json" || parts[3] == "server-whitelist.json"
	case "checkpoints":
		return strings.HasSuffix(parts[3], ".zip") && checkpointIDPattern.MatchString(strings.TrimSuffix(parts[3], ".zip"))
	}
	return false
}

func stageProfileBackup(input io.ReaderAt, size int64) (ProfileBackup, string, error) {
	var backup ProfileBackup
	reader, err := zip.NewReader(input, size)
	if err != nil {
		return backup, "", ErrInvalidBackup
	}
	staging, err := os.MkdirTemp("", "fsm-profile-import-")
	if err != nil {
		return backup, "", err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(staging)
		}
	}()
	if err := extractBackup(reader, staging, profileBackupEntryAllowed); err != nil {
		return backup, "", err
	}
	if err := readBackupJSON(filepath.Join(staging, "backup.json"), &backup); err != nil {
		return backup, "", ErrInvalidBackup
	}
	if backup.Kind != profileBackupKind || backup.FormatVersion != 1 || len(backup.Profiles) == 0 || len(backup.Profiles) > 256 {
		return backup, "", ErrInvalidBackup
	}
	ids := map[string]bool{}
	for index := range backup.Profiles {
		item := &backup.Profiles[index]
		if validateProfileID(item.ID) != nil || ids[item.ID] {
			return backup, "", ErrInvalidBackup
		}
		item.ID = filepath.Base(item.ID)
		ids[item.ID] = true
		if err := validateBackupProfile(filepath.Join(staging, "profiles", filepath.Base(item.ID)), item); err != nil {
			return backup, "", err
		}
	}
	for _, entry := range reader.File {
		parts := strings.Split(entry.Name, "/")
		if len(parts) > 2 && parts[0] == "profiles" && !ids[parts[1]] {
			return backup, "", ErrInvalidBackup
		}
	}
	success = true
	return backup, staging, nil
}

func validateBackupProfile(root string, item *BackupProfile) error {
	if _, _, err := validateProfileText(item.Name, item.Description); err != nil {
		return ErrInvalidBackup
	}
	if _, err := NormalizeExactReleaseVersion(item.InstalledVersion); err != nil {
		return ErrInvalidBackup
	}
	if _, err := NormalizeReleaseTarget(item.ReleaseTarget); err != nil {
		return ErrInvalidBackup
	}
	if validateCheckpointSettings(item.CheckpointSettings) != nil {
		return ErrInvalidBackup
	}
	if err := sanitizeBackupServerSettings(filepath.Join(root, "config", "server-settings.json"), false); err != nil {
		return err
	}
	for _, name := range []string{"server-adminlist.json", "server-banlist.json", "server-whitelist.json"} {
		path := filepath.Join(root, "config", name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		var value interface{}
		if readBackupJSON(path, &value) != nil {
			return ErrInvalidBackup
		}
	}
	mods, err := validateBackupMods(filepath.Join(root, "mods"))
	if err != nil {
		return err
	}
	item.ModCount = len(mods.Mods)
	item.GameMode = mods.GameMode
	item.EnabledBuiltInMods = mods.EnabledBuiltInMods
	item.SaveCount = 0
	entries, err := os.ReadDir(filepath.Join(root, "saves"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	selectedFound := false
	for _, entry := range entries {
		if entry.IsDir() || !backupSaveFile(entry.Name()) || verifyFactorioSaveZip(filepath.Join(root, "saves", entry.Name())) != nil {
			return ErrInvalidBackup
		}
		item.SaveCount++
		selectedFound = selectedFound || entry.Name() == item.SelectedSave
	}
	if item.SelectedSave != "" && !strings.HasPrefix(item.SelectedSave, "Load Latest") && !selectedFound {
		return ErrInvalidBackup
	}
	seen := map[string]bool{}
	for _, checkpoint := range item.Checkpoints {
		if !checkpointIDPattern.MatchString(checkpoint.ID) || seen[checkpoint.ID] || checkpoint.FileName != checkpoint.ID+".zip" || !validCheckpointTrigger(checkpoint.Trigger) {
			return ErrInvalidBackup
		}
		seen[checkpoint.ID] = true
		if checkpoint.SourceSave != "" && ValidatePathElement(checkpoint.SourceSave) != nil {
			return ErrInvalidBackup
		}
		if verifyFactorioSaveZip(filepath.Join(root, "checkpoints", filepath.Base(checkpoint.FileName))) != nil {
			return ErrInvalidBackup
		}
	}
	checkpointFiles, err := os.ReadDir(filepath.Join(root, "checkpoints"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(checkpointFiles) != len(item.Checkpoints) {
		return ErrInvalidBackup
	}
	item.Active = false
	item.BindIP = ""
	return nil
}

func PreviewProfileBackup(input io.ReaderAt, size int64) (ProfileBackup, error) {
	backup, staging, err := stageProfileBackup(input, size)
	if err != nil {
		return ProfileBackup{}, err
	}
	defer os.RemoveAll(staging)
	profileMutex.Lock()
	defer profileMutex.Unlock()
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileBackup{}, err
	}
	for index := range backup.Profiles {
		item := &backup.Profiles[index]
		base := item.Name
		for suffix := 2; profileNameExists(manifest, item.Name, ""); suffix++ {
			runes := []rune(base)
			if len(runes) > 48 {
				runes = runes[:48]
			}
			item.Name = fmt.Sprintf("%s (import %d)", string(runes), suffix)
		}
		manifest.Profiles = append(manifest.Profiles, item.Profile)
	}
	return backup, nil
}
