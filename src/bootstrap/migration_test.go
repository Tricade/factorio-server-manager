package bootstrap

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func createLegacyUsers(t *testing.T, path string, duplicate bool) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("non-default-test-password"), bcrypt.MinCost)
	require.NoError(t, err)
	record := map[string]string{"Username": "legacy-user", "Email": "user@example.invalid", "Role": "admin", "Hash": base64.StdEncoding.EncodeToString(hash)}
	records := map[string]any{"first": record}
	if duplicate {
		records["second"] = record
	}
	contents, err := json.Marshal(records)
	require.NoError(t, err)
	db, err := leveldb.OpenFile(path, nil)
	require.NoError(t, err)
	require.NoError(t, db.Put([]byte("httpauth::userdata"), contents, nil))
	require.NoError(t, db.Close())
}

func TestLegacyMigrationRetainsSourceOnInsertFailure(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "legacy.leveldb"), filepath.Join(root, "users.db")
	createLegacyUsers(t, source, true)
	// The old migration silently ignored the unique-index failure, deleted the
	// only source and left an incomplete account database.
	require.Error(t, MigrateLevelDBToSqlite(source, target))
	require.DirExists(t, source, "the only source must survive a failed migration")
	db, err := gorm.Open(sqlite.Open(target), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var count int64
	require.NoError(t, db.Model(&User{}).Count(&count).Error)
	require.Zero(t, count, "a failed batch must not leave partially migrated accounts")
}

func TestLegacyMigrationRetainsSuccessfulSourceForRecovery(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "legacy.leveldb"), filepath.Join(root, "users.db")
	createLegacyUsers(t, source, false)
	require.NoError(t, MigrateLevelDBToSqlite(source, target))
	_, err := os.Stat(source)
	require.NoError(t, err, "configuration activation can fail later; retain the legacy source")
	require.NoError(t, MigrateLevelDBToSqlite(source, target), "a completed non-default account batch is retryable")
}

func TestLegacyDefaultPasswordMigrationCanRetryWithoutChangingPassword(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "legacy.leveldb"), filepath.Join(root, "users.db")
	hash, err := bcrypt.GenerateFromPassword([]byte("factorio"), bcrypt.MinCost)
	require.NoError(t, err)
	contents, err := json.Marshal(map[string]legacyUser{"first": {Username: "legacy-user", Role: "admin", Hash: base64.StdEncoding.EncodeToString(hash)}})
	require.NoError(t, err)
	legacy, err := leveldb.OpenFile(source, nil)
	require.NoError(t, err)
	require.NoError(t, legacy.Put([]byte("httpauth::userdata"), contents, nil))
	require.NoError(t, legacy.Close())
	require.NoError(t, MigrateLevelDBToSqlite(source, target))
	credentialPath := filepath.Join(root, "migrated-user-passwords.jsonl")
	before, err := os.ReadFile(credentialPath)
	require.NoError(t, err)
	require.NoError(t, MigrateLevelDBToSqlite(source, target))
	after, err := os.ReadFile(credentialPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "a retry must not create another one-time credential")
}
