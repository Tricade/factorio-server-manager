package factorio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseFileRollbackFailureRetainsAndReportsBackup(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := filepath.Join(t.TempDir(), "factorio")
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "new.bin"), []byte("new release"), 0600))
	require.NoError(t, os.MkdirAll(destinationDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "a.txt"), []byte("old a"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "b.txt"), []byte("old b"), 0600))

	originalRename := releaseRename
	t.Cleanup(func() { releaseRename = originalRename })
	backupDir := ""
	releaseRename = func(oldPath, newPath string) error {
		if strings.Contains(newPath, ".factorio-release-backup-") {
			backupDir = filepath.Dir(newPath)
			if filepath.Base(oldPath) == "b.txt" {
				return errors.New("injected backup move failure")
			}
		}
		if strings.Contains(oldPath, ".factorio-release-backup-") && filepath.Base(oldPath) == "a.txt" && filepath.Dir(newPath) == destinationDir {
			return errors.New("injected rollback failure")
		}
		return os.Rename(oldPath, newPath)
	}

	err := replaceReleaseFiles(sourceDir, destinationDir)
	require.Error(t, err)
	require.NotEmpty(t, backupDir)
	require.DirExists(t, backupDir, "rollback failure must not delete the only recovery copy")
	require.Contains(t, err.Error(), backupDir, "error should tell the operator where recovery files remain")
	require.FileExists(t, filepath.Join(backupDir, "a.txt"))
	contents, readErr := os.ReadFile(filepath.Join(backupDir, "a.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "old a", string(contents))

	releaseRename = originalRename
	retryErr := replaceReleaseFiles(sourceDir, destinationDir)
	require.Error(t, retryErr)
	require.Contains(t, retryErr.Error(), backupDir)
	require.DirExists(t, backupDir)
	contents, readErr = os.ReadFile(filepath.Join(backupDir, "a.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "old a", string(contents), "a later replacement must not absorb or delete the retained recovery data")
}
