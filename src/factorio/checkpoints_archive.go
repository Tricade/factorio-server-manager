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
	"github.com/OpenFactorioServerManager/rcon"
)

func newCheckpointID(profileID, trigger string, now time.Time) (string, error) {
	base := now.Format("20060102T150405.000000000Z") + "-" + trigger
	for suffix := 0; suffix < 1000; suffix++ {
		id := base
		if suffix > 0 {
			id += fmt.Sprintf("-%d", suffix)
		}
		if _, err := os.Stat(filepath.Join(checkpointFilesDirectory(profileID), id+".zip")); errors.Is(err, os.ErrNotExist) {
			return id, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("unable to allocate a unique checkpoint name")
}

func checkpointSource(id string) (path, sourceSave string, cleanup func(), err error) {
	server := GetFactorioServer()
	if server.GetRunning() {
		path, err = checkpointCreateLiveSave(id)
		if err != nil {
			return "", "", nil, err
		}
		sourceSave = server.Snapshot().Savefile
		cleanup = func() { _ = os.Remove(path) }
		return path, sourceSave, cleanup, nil
	}

	snapshot := server.Snapshot()
	save, err := findCheckpointSourceSave(snapshot.Savefile)
	if err != nil {
		return "", "", nil, fmt.Errorf("find source save: %w", err)
	}
	return filepath.Join(checkpointSavesPath(), save.Name), save.Name, nil, nil
}

func findCheckpointSourceSave(selected string) (*Save, error) {
	entries, err := os.ReadDir(checkpointSavesPath())
	if err != nil {
		return nil, err
	}
	var latest *Save
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !isUsableSave(info) {
			continue
		}
		save := &Save{Name: info.Name(), LastMod: info.ModTime(), Size: info.Size()}
		if selected != "" && !strings.HasPrefix(selected, "Load Latest") && save.Name == selected {
			return save, nil
		}
		if latest == nil || latest.LastMod.Before(save.LastMod) {
			latest = save
		}
	}
	if selected != "" && !strings.HasPrefix(selected, "Load Latest") {
		return nil, errors.New("selected save not found")
	}
	if latest == nil {
		return nil, errors.New("no usable save file found")
	}
	return latest, nil
}

func createLiveCheckpointSave(id string) (string, error) {
	if err := waitForCheckpointRCONReady(GetFactorioServer(), 60*time.Second); err != nil {
		return "", err
	}
	baseName := "fsm-checkpoint-" + strings.ReplaceAll(strings.SplitN(id, "-", 2)[0], ".", "")
	path := filepath.Join(checkpointSavesPath(), baseName+".zip")
	_ = os.Remove(path)
	if _, err := checkpointRunRCON("/save " + baseName); err != nil {
		return "", fmt.Errorf("ask Factorio to save: %w", err)
	}
	if err := waitForCompleteCheckpointSave(path, 90*time.Second); err != nil {
		return "", err
	}
	return path, nil
}

func waitForCheckpointRCONReady(server *Server, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if server.isStartupReady() {
			return nil
		}
		if server.IsStopping() || !server.GetRunning() {
			return errors.New("the Factorio stopped before RCON became ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the Factorio RCON did not become ready within %s", timeout)
}

func runCheckpointRCONCommand(command string) (string, error) {
	config := bootstrap.GetConfig()
	console, err := rcon.Dial(localRCONAddress(), config.FactorioRconPass)
	if err != nil {
		return "", err
	}
	defer console.Close()
	requestID, err := console.Write(command)
	if err != nil {
		return "", err
	}
	response, responseID, err := console.Read()
	if err != nil {
		return "", err
	}
	if responseID != requestID {
		return "", fmt.Errorf("unexpected RCON response id %d for request %d", responseID, requestID)
	}
	return response, nil
}

func waitForCompleteCheckpointSave(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastSize := int64(-1)
	stableReads := 0
	for time.Now().Before(deadline) {
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Size() > 0 {
			if info.Size() == lastSize {
				stableReads++
			} else {
				lastSize = info.Size()
				stableReads = 0
			}
			if stableReads >= 2 && verifyCheckpointZip(path) == nil {
				return nil
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("the Factorio save %s did not become a complete ZIP within %s", filepath.Base(path), timeout)
}

func persistCheckpoint(profileID string, metadata checkpointMetadata, sourcePath string) error {
	if err := verifyCheckpointZip(sourcePath); err != nil {
		return fmt.Errorf("source save is incomplete: %w", err)
	}
	directory := checkpointFilesDirectory(profileID)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	destinationPath := filepath.Join(directory, metadata.ID+".zip")
	if err := copyCheckpointAtomically(sourcePath, destinationPath); err != nil {
		return err
	}
	if err := writeCheckpointJSONAtomically(checkpointMetadataPath(profileID, metadata.ID), metadata); err != nil {
		_ = os.Remove(destinationPath)
		return err
	}
	return nil
}

func copyCheckpointAtomically(sourcePath, destinationPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("checkpoint source is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destinationPath), ".checkpoint-copy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := verifyCheckpointZip(temporaryPath); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destinationPath)
}

func verifyCheckpointZip(path string) error {
	return verifyFactorioSaveZip(path)
}
