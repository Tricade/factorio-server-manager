package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type User struct {
	gorm.Model
	Username string `json:"username" gorm:"uniqueIndex,not null"`
	Password string `json:"password" gorm:"not null"`
	Role     string `json:"role" gorm:"not null"`
	Email    string `json:"email"`
}

// MigrateLevelDBToSqlite commits the entire user batch or none of it. The
// legacy source remains available even after success: saving conf.json is a
// separate operation and must never make the only recoverable copy disappear.
func MigrateLevelDBToSqlite(oldDBFile, newDBFile string) (result error) {
	oldDB, err := leveldb.OpenFile(oldDBFile, &opt.Options{ReadOnly: true, ErrorIfMissing: true})
	if err != nil {
		return fmt.Errorf("open legacy users: %w", err)
	}
	defer func() { result = errors.Join(result, oldDB.Close()) }()
	contents, err := oldDB.Get([]byte("httpauth::userdata"), nil)
	if err != nil {
		return fmt.Errorf("read legacy users: %w", err)
	}
	var records map[string]legacyUser
	if err := json.Unmarshal(contents, &records); err != nil {
		return fmt.Errorf("decode legacy users: %w", err)
	}
	if records == nil {
		return errors.New("legacy users must be a JSON object")
	}
	// Disable SQL value interpolation in error logs: password hashes are data,
	// not appropriate diagnostic context.
	db, err := gorm.Open(sqlite.Open(newDBFile), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("open migrated user database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open migrated database connection: %w", err)
	}
	defer func() { result = errors.Join(result, sqlDB.Close()) }()
	if err := db.AutoMigrate(&User{}); err != nil {
		return fmt.Errorf("prepare user schema: %w", err)
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return db.Transaction(func(tx *gorm.DB) error {
		seen := make(map[string]bool, len(records))
		for _, key := range keys {
			record := records[key]
			if record.Username == "" || seen[record.Username] {
				return errors.New("legacy users contain an empty or duplicate username")
			}
			seen[record.Username] = true
			if err := migrateLegacyUser(tx, record, newDBFile); err != nil {
				return err
			}
		}
		return nil
	})
}

type legacyUser struct {
	Username string
	Email    string
	Hash     string
	Role     string
}

func migrateLegacyUser(tx *gorm.DB, record legacyUser, databasePath string) error {
	hash, err := base64.StdEncoding.DecodeString(record.Hash)
	if err != nil {
		return errors.New("legacy user password hash is not valid base64")
	}
	if _, err := bcrypt.Cost(hash); err != nil {
		return errors.New("legacy user password hash is not valid bcrypt")
	}
	defaultPassword := bcrypt.CompareHashAndPassword(hash, []byte("factorio")) == nil
	var existing User
	lookup := tx.Where("username = ?", record.Username).First(&existing).Error
	if lookup == nil {
		if existing.Role == record.Role && existing.Email == record.Email {
			if !defaultPassword && existing.Password == record.Hash {
				return nil
			}
			if defaultPassword {
				matched, err := migrationCredentialMatches(filepath.Join(filepath.Dir(databasePath), "migrated-user-passwords.jsonl"), record.Username, existing.Password)
				if err != nil {
					return err
				}
				if matched {
					return nil // Retry without rotating an already migrated password again.
				}
			}
		}
		return errors.New("legacy migration conflicts with an existing account; original users retained")
	}
	if !errors.Is(lookup, gorm.ErrRecordNotFound) {
		return fmt.Errorf("inspect existing migrated account: %w", lookup)
	}
	if defaultPassword {
		password := GenerateRandomPassword()
		replacement, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("replace legacy default password: %w", err)
		}
		record.Hash = base64.StdEncoding.EncodeToString(replacement)
		credentialPath := filepath.Join(filepath.Dir(databasePath), "migrated-user-passwords.jsonl")
		if err := appendGeneratedCredential(credentialPath, record.Username, password); err != nil {
			return fmt.Errorf("store generated migration credential: %w", err)
		}
	}
	if err := tx.Create(&User{Username: record.Username, Password: record.Hash, Role: record.Role, Email: record.Email}).Error; err != nil {
		return fmt.Errorf("insert migrated account: %w", err)
	}
	return nil
}

func migrationCredentialMatches(path, username, encodedHash string) (bool, error) {
	hash, err := base64.StdEncoding.DecodeString(encodedHash)
	if err != nil {
		return false, errors.New("existing migration password hash is invalid")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read migration credential record: %w", err)
	}
	defer file.Close()
	const maximumCredentialBytes = 1 << 20
	contents, err := io.ReadAll(io.LimitReader(file, maximumCredentialBytes+1))
	if err != nil {
		return false, fmt.Errorf("read migration credential record: %w", err)
	}
	if len(contents) > maximumCredentialBytes {
		return false, errors.New("migration credential record exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	matched := false
	for {
		var record struct{ Username, Password string }
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				return matched, nil
			}
			return false, errors.New("migration credential record is malformed")
		}
		if record.Username == username && bcrypt.CompareHashAndPassword(hash, []byte(record.Password)) == nil {
			matched = true
		}
	}
}

func appendGeneratedCredential(path, username, password string) (result error) {
	record, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return fmt.Errorf("encode generated credential: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(append(record, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func GenerateRandomPassword() string {
	randomBytes := make([]byte, 18)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Errorf("generate random password: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}
