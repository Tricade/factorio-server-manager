package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func checkpointProfileDirectory(profileID string) string {
	return filepath.Join(checkpointRootPath(), profileID)
}

func checkpointFilesDirectory(profileID string) string {
	return filepath.Join(checkpointProfileDirectory(profileID), checkpointFilesDir)
}

func checkpointMetadataPath(profileID, id string) string {
	return filepath.Join(checkpointFilesDirectory(profileID), id+".json")
}

func loadCheckpointStore(profileID string) (checkpointStore, error) {
	store := checkpointStore{SchemaVersion: checkpointSchemaVersion, Settings: defaultCheckpointSettings()}
	contents, err := os.ReadFile(filepath.Join(checkpointProfileDirectory(profileID), checkpointStateFile))
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return checkpointStore{}, err
	}
	if err := json.Unmarshal(contents, &store); err != nil {
		return checkpointStore{}, fmt.Errorf("decode checkpoint settings: %w", err)
	}
	if store.SchemaVersion != checkpointSchemaVersion {
		return checkpointStore{}, fmt.Errorf("unsupported checkpoint schema version %d", store.SchemaVersion)
	}
	if err := validateCheckpointSettings(store.Settings); err != nil {
		return checkpointStore{}, err
	}
	return store, nil
}

func saveCheckpointStore(profileID string, store checkpointStore) error {
	root := checkpointProfileDirectory(profileID)
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	store.SchemaVersion = checkpointSchemaVersion
	return writeCheckpointJSONAtomically(filepath.Join(root, checkpointStateFile), store)
}

func writeCheckpointJSONAtomically(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".checkpoint-json-*")
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
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
