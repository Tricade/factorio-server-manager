package factorio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
