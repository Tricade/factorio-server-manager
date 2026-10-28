package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func listCheckpoints(profileID string) ([]Checkpoint, error) {
	directory := checkpointFilesDirectory(profileID)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []Checkpoint{}, nil
	}
	if err != nil {
		return nil, err
	}
	checkpoints := make([]Checkpoint, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".zip")
		if !checkpointIDPattern.MatchString(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		checkpoint := Checkpoint{ID: id, FileName: entry.Name(), CreatedAt: info.ModTime().UTC(), Trigger: "unknown", Size: info.Size()}
		metadataContents, metadataErr := os.ReadFile(checkpointMetadataPath(profileID, id))
		if metadataErr == nil {
			var metadata checkpointMetadata
			if json.Unmarshal(metadataContents, &metadata) == nil && metadata.ID == id {
				checkpoint.CreatedAt = metadata.CreatedAt
				checkpoint.Trigger = metadata.Trigger
				checkpoint.SourceSave = metadata.SourceSave
			}
		}
		checkpoints = append(checkpoints, checkpoint)
	}
	sort.SliceStable(checkpoints, func(i, j int) bool { return checkpoints[i].CreatedAt.After(checkpoints[j].CreatedAt) })
	return checkpoints, nil
}

func findCheckpoint(profileID, id string) (Checkpoint, string, error) {
	if !checkpointIDPattern.MatchString(id) {
		return Checkpoint{}, "", ErrCheckpointNotFound
	}
	checkpoints, err := listCheckpoints(profileID)
	if err != nil {
		return Checkpoint{}, "", err
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.ID == id {
			return checkpoint, filepath.Join(checkpointFilesDirectory(profileID), checkpoint.FileName), nil
		}
	}
	return Checkpoint{}, "", ErrCheckpointNotFound
}

func enforceCheckpointRetention(profileID string, retention int) error {
	if retention == 0 {
		return nil
	}
	checkpoints, err := listCheckpoints(profileID)
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, checkpoint := range checkpoints[min(retention, len(checkpoints)):] {
		if err := os.Remove(filepath.Join(checkpointFilesDirectory(profileID), checkpoint.FileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		if err := os.Remove(checkpointMetadataPath(profileID, checkpoint.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func availableRestoredSaveName(id string) (string, error) {
	directory := checkpointSavesPath()
	base := "restored-" + id
	for suffix := 0; suffix < 1000; suffix++ {
		name := base + ".zip"
		if suffix > 0 {
			name = fmt.Sprintf("%s-%d.zip", base, suffix)
		}
		if _, err := os.Stat(filepath.Join(directory, name)); errors.Is(err, os.ErrNotExist) {
			return name, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("unable to allocate a restored save name")
}

func recordCheckpointErrorUnlocked(profileID string, checkpointErr error) error {
	store, err := loadCheckpointStore(profileID)
	if err != nil {
		return err
	}
	now := checkpointNow().UTC()
	store.LastError = checkpointErr.Error()
	store.LastErrorAt = &now
	return saveCheckpointStore(profileID, store)
}

func deleteProfileCheckpoints(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	return os.RemoveAll(checkpointProfileDirectory(profileID))
}
