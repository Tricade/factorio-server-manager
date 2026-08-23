package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const runtimeStateFileName = "runtime-state.json"
const legacyReleaseTargetFileName = "release-channel"

// RuntimeState is persisted with manager data. InstalledVersion is deliberately
// exact: recreating the manager container must reinstall the same Factorio
// binary, not silently advance or downgrade a rolling release channel.
type RuntimeState struct {
	ReleaseTarget    string `json:"release_target"`
	InstalledVersion string `json:"installed_version"`
}

type runtimeStateFileSnapshot struct {
	path     string
	contents []byte
	mode     os.FileMode
	exists   bool
}

var runtimeStatePath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), runtimeStateFileName)
}

func LoadRuntimeState() (RuntimeState, error) {
	path := runtimeStatePath()
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return loadLegacyRuntimeState(filepath.Dir(path))
	}
	if err != nil {
		return RuntimeState{}, fmt.Errorf("read runtime state: %w", err)
	}

	var state RuntimeState
	if err := json.Unmarshal(contents, &state); err != nil {
		return RuntimeState{}, fmt.Errorf("decode runtime state: %w", err)
	}
	if state.ReleaseTarget != "" {
		target, err := NormalizeReleaseTarget(state.ReleaseTarget)
		if err != nil {
			return RuntimeState{}, fmt.Errorf("decode runtime release target: %w", err)
		}
		state.ReleaseTarget = target
	}
	if state.InstalledVersion != "" {
		version, err := NormalizeExactReleaseVersion(state.InstalledVersion)
		if err != nil {
			return RuntimeState{}, fmt.Errorf("decode installed runtime version: %w", err)
		}
		state.InstalledVersion = version
	}
	return state, nil
}

func loadLegacyRuntimeState(directory string) (RuntimeState, error) {
	contents, err := os.ReadFile(filepath.Join(directory, legacyReleaseTargetFileName))
	if errors.Is(err, os.ErrNotExist) {
		return RuntimeState{}, nil
	}
	if err != nil {
		return RuntimeState{}, fmt.Errorf("read legacy release target: %w", err)
	}
	target, err := NormalizeReleaseTarget(strings.TrimSpace(string(contents)))
	if err != nil {
		return RuntimeState{}, fmt.Errorf("decode legacy release target: %w", err)
	}
	return RuntimeState{ReleaseTarget: target}, nil
}

func persistRuntimeState(target, installedVersion string) error {
	normalizedTarget, err := NormalizeReleaseTarget(target)
	if err != nil {
		return err
	}
	normalizedVersion, err := NormalizeExactReleaseVersion(installedVersion)
	if err != nil {
		return err
	}

	path := runtimeStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create runtime state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".runtime-state-*")
	if err != nil {
		return fmt.Errorf("create runtime state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(RuntimeState{ReleaseTarget: normalizedTarget, InstalledVersion: normalizedVersion}); err != nil {
		temporary.Close()
		return fmt.Errorf("write runtime state: %w", err)
	}
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("restrict runtime state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close runtime state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate runtime state: %w", err)
	}

	// Keep the old marker current so rolling back to 0.11.0 still preserves a
	// stable/latest selection. Exact targets are harmless to newer entrypoints.
	legacyPath := filepath.Join(filepath.Dir(path), legacyReleaseTargetFileName)
	if err := os.WriteFile(legacyPath, []byte(normalizedTarget+"\n"), 0600); err != nil {
		return fmt.Errorf("write legacy release target: %w", err)
	}
	return nil
}

func snapshotRuntimeStateFiles() ([]runtimeStateFileSnapshot, error) {
	statePath := runtimeStatePath()
	paths := []string{
		statePath,
		filepath.Join(filepath.Dir(statePath), legacyReleaseTargetFileName),
	}
	snapshots := make([]runtimeStateFileSnapshot, 0, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshots = append(snapshots, runtimeStateFileSnapshot{path: path})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot runtime metadata %s: %w", filepath.Base(path), err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect runtime metadata %s: %w", filepath.Base(path), err)
		}
		snapshots = append(snapshots, runtimeStateFileSnapshot{
			path:     path,
			contents: contents,
			mode:     info.Mode().Perm(),
			exists:   true,
		})
	}
	return snapshots, nil
}

func restoreRuntimeStateFiles(snapshots []runtimeStateFileSnapshot) error {
	var restoreErrors []error
	for _, snapshot := range snapshots {
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove new %s: %w", filepath.Base(snapshot.path), err))
			}
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.contents, snapshot.mode); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", filepath.Base(snapshot.path), err))
			continue
		}
		if err := os.Chmod(snapshot.path, snapshot.mode); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restrict restored %s: %w", filepath.Base(snapshot.path), err))
		}
	}
	return errors.Join(restoreErrors...)
}
