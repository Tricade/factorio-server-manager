package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User bootstrap.User

const (
	UserRoleAdmin  = "admin"
	UserRoleViewer = "viewer"
)

var validUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var authMutationMutex sync.Mutex
var ErrInvalidUser = errors.New("invalid user")

const (
	loginAttemptLimit       = 5
	loginAttemptWindow      = time.Minute
	loginSessionMaxAge      = 12 * 60 * 60
	maximumLoginAttemptKeys = 10000
)

type loginAttemptState struct {
	Count       int
	WindowStart time.Time
}

var loginAttempts = make(map[string]loginAttemptState)
var loginAttemptsMutex sync.Mutex
var loginNow = time.Now
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("factorio-server-control-invalid-user"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
}()

type authenticatedUserContextKey struct{}

type Auth struct {
	db *gorm.DB
}

var (
	sessionStore         *sessions.CookieStore
	sessionCredentialKey []byte
	auth                 Auth
)

func SetupAuth() {
	var err error

	config := bootstrap.GetConfig()

	cookieEncryptionKey, err := base64.StdEncoding.DecodeString(config.CookieEncryptionKey)
	if err != nil {
		log.Printf("Error decoding base64 cookie encryption key: %s", err)
		panic(err)
	}
	sessionCredentialKey = append([]byte(nil), cookieEncryptionKey...)
	sessionStore = sessions.NewCookieStore(cookieEncryptionKey)
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		Secure:   config.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   loginSessionMaxAge,
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
		user.Role = UserRoleAdmin

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
	if err := auth.ensureAdministrativeUser(); err != nil {
		log.Printf("Error ensuring an administrative user exists: %s", err)
		panic(err)
	}
}

// sessionCredentialVersion is an opaque, server-keyed marker for the current
// password hash. CookieStore signs but does not encrypt when configured with a
// single key, so storing the BCrypt hash itself would expose it to the client.
func sessionCredentialVersion(passwordHash string) (string, error) {
	if len(sessionCredentialKey) == 0 {
		return "", errors.New("session credential key is not initialized")
	}
	mac := hmac.New(sha256.New, sessionCredentialKey)
	_, _ = mac.Write([]byte("factorio-server-control/session-credential/v1\x00"))
	_, _ = mac.Write([]byte(passwordHash))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// ensureAdministrativeUser preserves upgrade access when a legacy database
// has no meaningful roles. Existing admin spellings are normalized; otherwise
// only the oldest account is promoted, while every other legacy role remains
// non-administrative.
func (a *Auth) ensureAdministrativeUser() error {
	var users []User
	if result := a.db.Order("id asc").Find(&users); result.Error != nil {
		return result.Error
	}
	foundAdmin := false
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Role), UserRoleAdmin) {
			foundAdmin = true
			if user.Role != UserRoleAdmin {
				if result := a.db.Model(&User{}).Where("id = ?", user.ID).Update("role", UserRoleAdmin); result.Error != nil {
					return result.Error
				}
			}
		}
	}
	if foundAdmin || len(users) == 0 {
		return nil
	}
	if result := a.db.Model(&User{}).Where("id = ?", users[0].ID).Update("role", UserRoleAdmin); result.Error != nil {
		return result.Error
	}
	log.Printf("Promoted oldest legacy account %q to admin because the database had no administrator role", users[0].Username)
	return nil
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
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Perform the same expensive password comparison for unknown accounts
			// so login timing does not trivially enumerate usernames.
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
			return bcrypt.ErrMismatchedHashAndPassword
		}
		log.Printf("Error reading user from database during login: %s", result.Error)
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

func loginRateLimitKeys(r *http.Request, username string) []string {
	host := strings.TrimSpace(r.RemoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if host == "" {
		host = "unknown"
	}
	return []string{"ip:" + host, "user:" + strings.ToLower(strings.TrimSpace(username))}
}

func loginAttemptAllowed(keys []string) bool {
	now := loginNow()
	loginAttemptsMutex.Lock()
	defer loginAttemptsMutex.Unlock()
	for _, key := range keys {
		state := loginAttempts[key]
		if state.WindowStart.IsZero() || now.Sub(state.WindowStart) >= loginAttemptWindow {
			continue
		}
		if state.Count >= loginAttemptLimit {
			return false
		}
	}
	return true
}

func recordLoginFailure(keys []string) {
	now := loginNow()
	loginAttemptsMutex.Lock()
	defer loginAttemptsMutex.Unlock()
	if len(loginAttempts) >= maximumLoginAttemptKeys {
		for key, state := range loginAttempts {
			if now.Sub(state.WindowStart) >= loginAttemptWindow {
				delete(loginAttempts, key)
			}
		}
	}
	for _, key := range keys {
		state := loginAttempts[key]
		if state.WindowStart.IsZero() && len(loginAttempts) >= maximumLoginAttemptKeys {
			continue
		}
		if state.WindowStart.IsZero() || now.Sub(state.WindowStart) >= loginAttemptWindow {
			state = loginAttemptState{WindowStart: now}
		}
		state.Count++
		loginAttempts[key] = state
	}
}

func clearLoginFailures(keys []string) {
	loginAttemptsMutex.Lock()
	defer loginAttemptsMutex.Unlock()
	for _, key := range keys {
		delete(loginAttempts, key)
	}
}

func (a *Auth) deleteUser(username string) error {
	authMutationMutex.Lock()
	defer authMutationMutex.Unlock()
	return a.db.Transaction(func(tx *gorm.DB) error {
		var user User
		if result := tx.Where(&User{Username: username}).Take(&user); result.Error != nil {
			return result.Error
		}
		if user.Role == UserRoleAdmin {
			var adminCount int64
			if result := tx.Model(&User{}).Where(&User{Role: UserRoleAdmin}).Count(&adminCount); result.Error != nil {
				return result.Error
			}
			if adminCount <= 1 {
				return errors.New("cannot delete single admin user")
			}
		}
		result := tx.Delete(&user)
		if result.Error != nil {
			log.Printf("Error deleting user from database: %s", result.Error)
		}
		return result.Error
	})
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
	user.Username = strings.TrimSpace(user.Username)
	user.Role = strings.ToLower(strings.TrimSpace(user.Role))
	if !validUsernamePattern.MatchString(user.Username) {
		return fmt.Errorf("%w: username must contain 1-64 letters, digits, dots, dashes or underscores", ErrInvalidUser)
	}
	if user.Role != UserRoleAdmin && user.Role != UserRoleViewer {
		return fmt.Errorf("%w: role must be admin or viewer", ErrInvalidUser)
	}
	if err := validateUserPassword(user.Password); err != nil {
		return err
	}
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
	if err := validateUserPassword(password); err != nil {
		return err
	}
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

func validateUserPassword(password string) error {
	if len(password) < 10 || len(password) > 1024 {
		return fmt.Errorf("%w: password must contain between 10 and 1024 bytes", ErrInvalidUser)
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

		rawUsername, ok := session.Values["username"]
		username, validUsername := rawUsername.(string)
		if !ok || !validUsername || username == "" {
			http.Error(w, "Could not read username from sessioncookie", http.StatusUnauthorized)
			return
		}

		user, err := auth.getUser(username)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "authentication is no longer valid", http.StatusUnauthorized)
			} else {
				http.Error(w, "unable to validate authentication", http.StatusInternalServerError)
			}
			return
		}
		rawCredentialVersion, ok := session.Values["credential_version"]
		credentialVersion, validCredentialVersion := rawCredentialVersion.(string)
		expectedCredentialVersion, versionErr := sessionCredentialVersion(user.Password)
		if versionErr != nil {
			http.Error(w, "unable to validate authentication", http.StatusInternalServerError)
			return
		}
		if !ok || !validCredentialVersion || credentialVersion == "" || subtle.ConstantTimeCompare([]byte(credentialVersion), []byte(expectedCredentialVersion)) != 1 {
			http.Error(w, "authentication is no longer valid", http.StatusUnauthorized)
			return
		}
		user.Password = ""
		ctx := context.WithValue(r.Context(), authenticatedUserContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticatedUserFromRequest(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(authenticatedUserContextKey{}).(User)
	return user, ok
}

func requestUserIsAdministrator(r *http.Request) bool {
	user, ok := authenticatedUserFromRequest(r)
	return ok && user.Role == UserRoleAdmin
}

func RequireAdministrator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestUserIsAdministrator(r) {
			http.Error(w, "administrator role required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
