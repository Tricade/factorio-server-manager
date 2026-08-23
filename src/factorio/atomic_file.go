package factorio

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

var atomicFileRename = os.Rename

// writeFileAtomically writes a complete sibling temporary file before making
// it visible. On platforms that cannot rename over an existing file (notably
// Windows), the old file is moved aside and restored if activation fails.
func writeFileAtomically(path string, contents []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if path == "" || directory == "" {
		return errors.New("atomic file path is empty")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf("create atomic file directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".fsm-write-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := replaceFileWithRollback(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func replaceFileWithRollback(temporaryPath, destinationPath string) error {
	if err := atomicFileRename(temporaryPath, destinationPath); err == nil {
		return nil
	} else if _, statErr := os.Lstat(destinationPath); errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("activate temporary file: %w", err)
	} else if statErr != nil {
		return fmt.Errorf("inspect destination file: %w", statErr)
	}

	backup, err := os.CreateTemp(filepath.Dir(destinationPath), ".fsm-backup-*.tmp")
	if err != nil {
		return fmt.Errorf("prepare file backup: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close file backup: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare file backup path: %w", err)
	}
	defer os.Remove(backupPath)

	if err := atomicFileRename(destinationPath, backupPath); err != nil {
		return fmt.Errorf("back up destination file: %w", err)
	}
	if err := atomicFileRename(temporaryPath, destinationPath); err != nil {
		if restoreErr := atomicFileRename(backupPath, destinationPath); restoreErr != nil {
			return fmt.Errorf("activate temporary file: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("activate temporary file: %w", err)
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		// The new file is already committed. A stale backup is safer than
		// reporting failure and letting callers roll in-memory state backward.
		log.Printf("Atomic file replacement committed but backup cleanup failed: %v", err)
	}
	return nil
}

func copyWithHardLimit(destination io.Writer, source io.Reader, maximum int64) error {
	if maximum <= 0 {
		_, err := io.Copy(destination, source)
		return err
	}
	limited := &io.LimitedReader{R: source, N: maximum + 1}
	written, err := io.Copy(destination, limited)
	if err != nil {
		return err
	}
	if written > maximum {
		return fmt.Errorf("payload exceeds %d bytes", maximum)
	}
	return nil
}
