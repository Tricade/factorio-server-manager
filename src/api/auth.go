package api

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User bootstrap.User

type Auth struct {
	db *gorm.DB
}

var (
	sessionStore *sessions.CookieStore
	auth         Auth
)

func SetupAuth() {
	var err error

	config := bootstrap.GetConfig()

	cookieEncryptionKey, err := base64.StdEncoding.DecodeString(config.CookieEncryptionKey)
	if err != nil {
		log.Printf("Error decoding base64 cookie encryption key: %s", err)
		panic(err)
	}
	sessionStore = sessions.NewCookieStore(cookieEncryptionKey)
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		Secure:   config.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	auth.db, err = gorm.Open(sqlite.Open(config.SQLiteDatabaseFile), nil)
	if err != nil {
		log.Printf("Error opening sqlite or gorm database: %s", err)
		panic(err)
	}
	if err := os.Chmod(config.SQLiteDatabaseFile, 0600); err != nil {
		log.Printf("Error restricting sqlite database permissions: %s", err)
		panic(err)
	}

	err = auth.db.AutoMigrate(&User{})
	if err != nil {
		log.Printf("Error AutoMigrating gorm database: %s", err)
		panic(err)
	}

	var userCount int64
	auth.db.Model(&User{}).Count(&userCount)

	if userCount == 0 {
		// no user created yet, create a default one
		var password = bootstrap.GenerateRandomPassword()

		var user User
		user.Username = "admin"
		user.Password = password
		user.Role = "admin"

		credentialPath, err := writeInitialAdminCredential(config.SQLiteDatabaseFile, user.Username, password)
		if err != nil {
			log.Printf("Error storing initial admin credential: %s", err)
			panic(err)
		}
		err = auth.addUser(user)
		if err != nil {
			_ = os.Remove(credentialPath)
			log.Printf("Error adding admin user to db: %s", err)
			panic(err)
		}
		log.Printf("Created default admin user. Read the one-time credential from %s and change it immediately.", credentialPath)
	}
}

func writeInitialAdminCredential(databasePath, username, password string) (string, error) {
	credentialPath := filepath.Join(filepath.Dir(databasePath), "initial-admin-password.txt")
	contents := []byte(fmt.Sprintf("username=%s\npassword=%s\n", username, password))
	if err := os.WriteFile(credentialPath, contents, 0600); err != nil {
		return "", err
	}
	if err := os.Chmod(credentialPath, 0600); err != nil {
		return "", err
	}
	return credentialPath, nil
}

func (a *Auth) checkPassword(username, password string) error {
	var user User
	result := a.db.Where(&User{Username: username}).Take(&user)
	if result.Error != nil {
		log.Printf("Error reading user from database: %s", result.Error)
		return result.Error
	}

	decodedHashPw, err := base64.StdEncoding.DecodeString(user.Password)
	if err != nil {
		log.Printf("Error decoding base64 password: %s", err)
		return err
	}

	err = bcrypt.CompareHashAndPassword(decodedHashPw, []byte(password))
	if err != nil {
		if err != bcrypt.ErrMismatchedHashAndPassword {
			log.Printf("Unexpected error comparing hash and pw: %s", err)
		}
		return err
	}

	// Password correct
	return nil
}

func (a *Auth) deleteUser(username string) error {
	adminUsers := []User{}
	adminQuery := a.db.Find(&User{}).Where(&User{Role: "admin"}).Find(&adminUsers)
	if adminQuery.Error != nil {
		log.Printf("Error retrieving admin user list from database: %s", adminQuery.Error)
		return adminQuery.Error
	}

	for _, user := range adminUsers {
		if user.Username == username {
			if adminQuery.RowsAffected == 1 {
				return errors.New("cannot delete single admin user")
			}
		}
	}

	result := a.db.Model(&User{}).Where(&User{Username: username}).Delete(&User{})
	if result.Error != nil {
		log.Printf("Error deleting user from database: %s", result.Error)
		return result.Error
	}
	return nil
}

func (a *Auth) hasUser(username string) (bool, error) {
	var count int64
	result := a.db.Model(&User{}).Where(&User{Username: username}).Count(&count)
	if result.Error != nil {
		log.Printf("Error checking if user exists in database: %s", result.Error)
		return false, result.Error
	}
	return count == 1, nil
}

func (a *Auth) getUser(username string) (User, error) {
	var user User
	result := a.db.Model(&User{}).Where(&User{Username: username}).Take(&user)
	if result.Error != nil {
		log.Printf("Error reading user from database: %s", result.Error)
		return User{}, result.Error
	}

	return user, nil
}

func (a *Auth) listUsers() ([]User, error) {
	var users []User
	result := a.db.Find(&users)
	if result.Error != nil {
		log.Printf("Error listing all users in database: %s", result.Error)
		return nil, result.Error
	}
	return users, nil
}

func (a *Auth) addUser(user User) error {
	// encrypt password
	pwHash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error generating bcrypt hash from password: %s", err)
		return err
	}

	user.Password = base64.StdEncoding.EncodeToString(pwHash)

	// add user to db
	result := a.db.Create(&user)
	if result.Error != nil {
		log.Printf("Error creating user in database: %s", result.Error)
		return result.Error
	}

	return nil
}

func (a *Auth) addUserWithHash(user User) error {
	// add user to db
	result := a.db.Create(&user)
	if result.Error != nil {
		log.Printf("Error creating user in database: %s", result.Error)
		return result.Error
	}

	return nil
}

func (a *Auth) changePassword(username, password string) error {
	var user User
	result := a.db.Model(&User{}).Where(&User{Username: username}).Take(&user)
	if result.Error != nil {
		log.Printf("Error reading user from database: %s", result.Error)
		return result.Error
	}

	hashPW, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Error generatig bcrypt hash from new password: %s", err)
		return err
	}

	user.Password = base64.StdEncoding.EncodeToString(hashPW)

	result = a.db.Save(&user)
	if result.Error != nil {
		log.Printf("Error resaving user in database: %s", result.Error)
		return result.Error
	}

	return nil
}

// middleware function, that will be called for every request, that has to be authorized
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := sessionStore.Get(r, "authentication")
		if err != nil {
			if session != nil {
				session.Options.MaxAge = -1
				err2 := session.Save(r, w)
				if err2 != nil {
					log.Printf("Error deleting cookie: %s", err2)
				}
			}
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		username, ok := session.Values["username"]
		if !ok {
			http.Error(w, "Could not read username from sessioncookie", http.StatusUnauthorized)
			return
		}

		hasUser, err := auth.hasUser(username.(string))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if hasUser {
			next.ServeHTTP(w, r)
		} else {
			log.Printf("Unauthenticated request %s %s %s", r.Method, r.Host, r.RequestURI)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	})
}
