package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/syndtr/goleveldb/leveldb"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
)

type User struct {
	gorm.Model
	Username string `json:"username" gorm:"uniqueIndex,not null"`
	Password string `json:"password" gorm:"not null"`
	Role     string `json:"role" gorm:"not null"`
	Email    string `json:"email"`
}

func MigrateLevelDBToSqlite(oldDBFile, newDBFile string) {
	oldDB, err := leveldb.OpenFile(oldDBFile, nil)
	if err != nil {
		log.Printf("Error opening old leveldb: %s", err)
		panic(err)
	}
	defer oldDB.Close()

	newDB, err := gorm.Open(sqlite.Open(newDBFile), nil)
	if err != nil {
		log.Printf("Error open sqlite and gorm: %s", err)
		panic(err)
	}
	defer func() {
		db, err2 := newDB.DB()
		if err2 != nil {
			log.Printf("Error getting real DB from gorm: %s", err2)
		}
		if db != nil {
			err2 = db.Close()
			if err2 != nil {
				log.Printf("Error closing real DB of gorm: %s", err2)
				panic(err2)
			}
		}
	}()

	err = newDB.AutoMigrate(&User{})
	if err != nil {
		log.Printf("Error autoMigrating sqlite database with user: %s", err)
		panic(err)
	}

	oldUserData, err := oldDB.Get([]byte("httpauth::userdata"), nil)
	if err != nil {
		log.Printf("Error getting `httpauth::userdata` from leveldb: %s", err)
		panic(err)
	}

	var migrationData map[string]struct {
		Username string
		Email    string
		Hash     string
		Role     string
	}
	err = json.Unmarshal(oldUserData, &migrationData)
	if err != nil {
		log.Printf("Error unmarshalling old ")
		panic(err)
	}

	for _, datum := range migrationData {
		// check if password is "factorio", which was the default password in the old system
		decodedHash, err := base64.StdEncoding.DecodeString(datum.Hash)
		if err != nil {
			log.Printf("Error decoding base64 hash: %s", err)
			panic(err)
		}

		err = bcrypt.CompareHashAndPassword(decodedHash, []byte("factorio"))
		if err == nil {
			// password is "factorio" .. change it
			newPassword := GenerateRandomPassword()

			bcryptPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
			if err != nil {
				log.Printf("Error generating has from password: %s", err)
				panic(err)
			}

			datum.Hash = base64.StdEncoding.EncodeToString(bcryptPassword)

			credentialPath := filepath.Join(filepath.Dir(newDBFile), "migrated-user-passwords.jsonl")
			if err := appendGeneratedCredential(credentialPath, datum.Username, newPassword); err != nil {
				log.Printf("Error storing migrated user credential: %s", err)
				panic(err)
			}
			log.Printf("Migrated user %q had the legacy default password; its one-time credential is stored in %s", datum.Username, credentialPath)
		}

		user := &User{
			Username: datum.Username,
			Password: datum.Hash,
			Role:     datum.Role,
			Email:    datum.Email,
		}

		newDB.Create(user)
	}

	oldDB.Close()

	// delete oldDB
	log.Println("Deleting old leveldb database.")
	err = os.RemoveAll(oldDBFile)
	if err != nil {
		log.Printf("Error removing leveldb: %s", err)
		panic(err)
	}
}

func appendGeneratedCredential(path, username, password string) error {
	record, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return fmt.Errorf("encode generated credential: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(append(record, '\n')); err != nil {
		return err
	}
	return nil
}

func GenerateRandomPassword() string {
	randomBytes := make([]byte, 18)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Errorf("generate random password: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}
