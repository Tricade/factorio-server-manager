package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func releaseVersionFromDirectory(directory string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(directory, "data", "base", "info.json"))
	if err != nil {
		return "", fmt.Errorf("read downloaded Factorio base version: %w", err)
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(contents, &info); err != nil {
		return "", fmt.Errorf("decode downloaded Factorio base version: %w", err)
	}
	normalized, err := NormalizeExactReleaseVersion(info.Version)
	if err != nil {
		return "", fmt.Errorf("validate downloaded Factorio base version: %w", err)
	}
	return normalized, nil
}

func validateRequiredBuiltInMods(sourceDir string, requiredBuiltIns []string) error {
	allowed := make(map[string]bool, len(spaceAgeFeatureMods))
	for _, name := range spaceAgeFeatureMods {
		allowed[name] = true
	}
	seen := make(map[string]bool, len(requiredBuiltIns))
	for _, name := range requiredBuiltIns {
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := os.Stat(filepath.Join(sourceDir, "data", name, "info.json")); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("release does not contain enabled built-in mod %s; switch this profile to base Factorio mode before installing this version", name)
		} else if err != nil {
			return fmt.Errorf("inspect downloaded built-in mod %s: %w", name, err)
		}
	}
	return nil
}

// replaceReleaseFiles stages the new program tree inside destinationDir and swaps its
// top-level program entries into place. destinationDir itself must never be renamed:
// in container deployments it is commonly a bind mount or Docker volume mount, and
// Linux rejects renaming a mount point with EBUSY.
func replaceReleaseFiles(sourceDir, destinationDir string) error {
	return replaceReleaseFilesValidated(sourceDir, destinationDir, nil)
}

// replaceReleaseFilesValidated retains the previous program tree until the
// caller has reloaded the new binary metadata and persisted its runtime state.
// A post-activation failure therefore rolls the program files back as part of
// the same transaction.
func replaceReleaseFilesValidated(sourceDir, destinationDir string, validate func() error) error {
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		return fmt.Errorf("create Factorio destination directory: %w", err)
	}
	if err := rejectUnresolvedReleaseBackups(destinationDir); err != nil {
		return err
	}

	stagingDir, err := os.MkdirTemp(destinationDir, ".factorio-release-staging-")
	if err != nil {
		return fmt.Errorf("create staged release directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	backupDir, err := os.MkdirTemp(destinationDir, ".factorio-release-backup-")
	if err != nil {
		return fmt.Errorf("create release backup directory: %w", err)
	}
	cleanupBackup := true
	defer func() {
		if cleanupBackup {
			_ = os.RemoveAll(backupDir)
		}
	}()

	if err := copyReleaseFiles(sourceDir, stagingDir, nil, releaseDataDirectories); err != nil {
		return fmt.Errorf("stage new Factorio release: %w", err)
	}

	ignoredEntries := map[string]bool{
		filepath.Base(stagingDir): true,
		filepath.Base(backupDir):  true,
	}
	for name := range releaseDataDirectories {
		ignoredEntries[name] = true
	}

	backedUpEntries, err := moveReleaseEntries(destinationDir, backupDir, ignoredEntries)
	if err != nil {
		if rollbackErr := restoreReleaseEntries(backupDir, destinationDir, backedUpEntries); rollbackErr != nil {
			cleanupBackup = false
			return fmt.Errorf("back up current Factorio release: %w (rollback failed: %v; recovery backup retained at %s)", err, rollbackErr, backupDir)
		}
		return fmt.Errorf("back up current Factorio release: %w (previous release restored)", err)
	}

	activatedEntries, err := moveReleaseEntries(stagingDir, destinationDir, nil)
	if err != nil {
		rollbackErr := rollbackReleaseActivation(destinationDir, stagingDir, backupDir, activatedEntries, backedUpEntries)
		if rollbackErr != nil {
			cleanupBackup = false
			return fmt.Errorf("activate new Factorio release: %w (rollback failed: %v; recovery backup retained at %s)", err, rollbackErr, backupDir)
		}
		return fmt.Errorf("activate new Factorio release: %w (previous release restored)", err)
	}
	if validate != nil {
		if err := validate(); err != nil {
			rollbackErr := rollbackReleaseActivation(destinationDir, stagingDir, backupDir, activatedEntries, backedUpEntries)
			if rollbackErr != nil {
				cleanupBackup = false
				return fmt.Errorf("validate new Factorio release: %w (rollback failed: %v; recovery backup retained at %s)", err, rollbackErr, backupDir)
			}
			return fmt.Errorf("validate new Factorio release: %w (previous release restored)", err)
		}
	}

	if err := os.RemoveAll(backupDir); err != nil {
		cleanupBackup = false
		return fmt.Errorf("remove previous Factorio release backup %s: %w", backupDir, err)
	}
	cleanupBackup = false
	return nil
}

func rejectUnresolvedReleaseBackups(destinationDir string) error {
	entries, err := os.ReadDir(destinationDir)
	if err != nil {
		return fmt.Errorf("inspect Factorio release directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".factorio-release-backup-") {
			return fmt.Errorf("unresolved Factorio release backup %s exists; inspect and resolve it before installing another release", filepath.Join(destinationDir, entry.Name()))
		}
	}
	return nil
}

// moveReleaseEntries renames the top-level entries from sourceDir into
// destinationDir. Both directories live below the Factorio mount, so the operation
// remains on one filesystem even when destinationDir is itself a mount point.
func moveReleaseEntries(sourceDir, destinationDir string, ignoredEntries map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}

	movedEntries := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if ignoredEntries != nil && ignoredEntries[name] {
			continue
		}
		if err := releaseRename(filepath.Join(sourceDir, name), filepath.Join(destinationDir, name)); err != nil {
			return movedEntries, err
		}
		movedEntries = append(movedEntries, name)
	}
	return movedEntries, nil
}

func restoreReleaseEntries(sourceDir, destinationDir string, entries []string) error {
	var restoreErrors []error
	for index := len(entries) - 1; index >= 0; index-- {
		name := entries[index]
		if err := releaseRename(filepath.Join(sourceDir, name), filepath.Join(destinationDir, name)); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", name, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func rollbackReleaseActivation(destinationDir, stagingDir, backupDir string, activatedEntries, backedUpEntries []string) error {
	var rollbackErrors []error
	if err := restoreReleaseEntries(destinationDir, stagingDir, activatedEntries); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("remove partially activated release: %w", err))
	}
	if err := restoreReleaseEntries(backupDir, destinationDir, backedUpEntries); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous release: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

// copyReleaseFiles copies all files when onlyDirectories is nil. Otherwise it copies
// only the named top-level directories. skipDirectories is excluded in either case.
func copyReleaseFiles(sourceDir, destinationDir string, onlyDirectories, skipDirectories map[string]bool) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return os.MkdirAll(destinationDir, info.Mode())
		}
		firstPathElement := strings.Split(relativePath, string(filepath.Separator))[0]
		if skipDirectories != nil && skipDirectories[firstPathElement] {
			if info.IsDir() && firstPathElement == relativePath {
				return filepath.SkipDir
			}
			return nil
		}
		if onlyDirectories != nil && !onlyDirectories[firstPathElement] {
			if info.IsDir() && firstPathElement == relativePath {
				return filepath.SkipDir
			}
			return nil
		}

		destinationPath := filepath.Join(destinationDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(destinationPath, info.Mode())
		}
		return copyReleaseFile(path, destinationPath, info.Mode())
	})
}

func copyReleaseFile(sourcePath, destinationPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
