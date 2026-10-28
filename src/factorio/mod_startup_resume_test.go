package factorio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPendingLegacyModPackWithoutDestinationCompletesOnce(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	pending := filepath.Join(legacyDir, "recovered.zip.migrating")
	inner := makeTestModArchive(t, `{"name":"example","version":"1.0.0"}`)
	writeLegacyModPack(t, pending, map[string][]byte{"example_1.0.0.zip": inner})
	require.NoError(t, prepareModStartup(config))
	require.FileExists(t, filepath.Join(legacyDir, "recovered.zip.migrated"))
	require.NoFileExists(t, pending)
	require.NoError(t, prepareModStartup(config), "a recovered activation must survive another restart")
}

func TestUnrelatedMigratingFileIsNotTreatedAsLegacyArchive(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	note := filepath.Join(legacyDir, "notes.txt.migrating")
	require.NoError(t, os.WriteFile(note, []byte("operator notes"), 0600))
	require.NoError(t, prepareModStartup(config))
	contents, err := os.ReadFile(note)
	require.NoError(t, err)
	require.Equal(t, "operator notes", string(contents))
}
