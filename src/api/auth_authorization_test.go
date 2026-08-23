package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAuthorizationTestAuth(t *testing.T) Auth {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.sqlite")), nil)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&User{}))
	sqlDatabase, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDatabase.Close() })
	return Auth{db: database}
}

func TestEnsureAdministrativeUserMigratesLegacyRolesLeastPrivilege(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	require.NoError(t, testAuth.db.Create(&User{Username: "oldest", Password: "hash", Role: "user"}).Error)
	require.NoError(t, testAuth.db.Create(&User{Username: "newer", Password: "hash", Role: UserRoleViewer}).Error)
	require.NoError(t, testAuth.ensureAdministrativeUser())

	oldest, err := testAuth.getUser("oldest")
	require.NoError(t, err)
	newer, err := testAuth.getUser("newer")
	require.NoError(t, err)
	assert.Equal(t, UserRoleAdmin, oldest.Role)
	assert.Equal(t, UserRoleViewer, newer.Role)

	// The migration is idempotent and does not promote every legacy account.
	require.NoError(t, testAuth.ensureAdministrativeUser())
	newer, err = testAuth.getUser("newer")
	require.NoError(t, err)
	assert.Equal(t, UserRoleViewer, newer.Role)
}

func TestEnsureAdministrativeUserNormalizesExistingAdmin(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	require.NoError(t, testAuth.db.Create(&User{Username: "administrator", Password: "hash", Role: " ADMIN "}).Error)
	require.NoError(t, testAuth.db.Create(&User{Username: "oldest-viewer", Password: "hash", Role: UserRoleViewer}).Error)
	require.NoError(t, testAuth.ensureAdministrativeUser())

	administrator, err := testAuth.getUser("administrator")
	require.NoError(t, err)
	viewer, err := testAuth.getUser("oldest-viewer")
	require.NoError(t, err)
	assert.Equal(t, UserRoleAdmin, administrator.Role)
	assert.Equal(t, UserRoleViewer, viewer.Role)
}

func TestAuthRejectsUnknownRolesAndWeakPasswords(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	assert.ErrorIs(t, testAuth.addUser(User{Username: "operator", Password: "long-enough-password", Role: "operator"}), ErrInvalidUser)
	assert.ErrorIs(t, testAuth.addUser(User{Username: "viewer", Password: "short", Role: UserRoleViewer}), ErrInvalidUser)
	assert.ErrorIs(t, testAuth.addUser(User{Username: "contains space", Password: "long-enough-password", Role: UserRoleViewer}), ErrInvalidUser)
	require.NoError(t, testAuth.addUser(User{Username: "read-only", Password: "long-enough-password", Role: UserRoleViewer}))
}

func TestDeleteUserPreservesLastAdministrator(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	require.NoError(t, testAuth.db.Create(&User{Username: "admin-one", Password: "hash", Role: UserRoleAdmin}).Error)
	require.NoError(t, testAuth.db.Create(&User{Username: "viewer", Password: "hash", Role: UserRoleViewer}).Error)
	assert.Error(t, testAuth.deleteUser("admin-one"))

	require.NoError(t, testAuth.db.Create(&User{Username: "admin-two", Password: "hash", Role: UserRoleAdmin}).Error)
	require.NoError(t, testAuth.deleteUser("admin-one"))
	_, err := testAuth.getUser("admin-one")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestWebsocketCommandAuthorizationRevokesExistingPrincipal(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	require.NoError(t, testAuth.db.Create(&User{Username: "socket-admin", Password: "hash-a", Role: UserRoleAdmin}).Error)
	originalAuth := auth
	auth = testAuth
	t.Cleanup(func() { auth = originalAuth })

	authorize := websocketCommandAuthorizer("socket-admin", "hash-a")
	assert.True(t, authorize(), "fresh administrator socket should be authorized")
	require.NoError(t, testAuth.db.Model(&User{}).Where("username = ?", "socket-admin").Update("role", UserRoleViewer).Error)
	assert.False(t, authorize(), "role downgrade must revoke an existing socket")
	require.NoError(t, testAuth.db.Model(&User{}).Where("username = ?", "socket-admin").Updates(map[string]interface{}{"role": UserRoleAdmin, "password": "hash-b"}).Error)
	assert.False(t, authorize(), "password change must revoke an existing socket")
	require.NoError(t, testAuth.db.Where("username = ?", "socket-admin").Delete(&User{}).Error)
	assert.False(t, authorize(), "user deletion must revoke an existing socket")
}

func TestLoginRateLimiterUsesBoundedWindow(t *testing.T) {
	originalNow := loginNow
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	loginNow = func() time.Time { return now }
	loginAttemptsMutex.Lock()
	loginAttempts = make(map[string]loginAttemptState)
	loginAttemptsMutex.Unlock()
	t.Cleanup(func() {
		loginNow = originalNow
		loginAttemptsMutex.Lock()
		loginAttempts = make(map[string]loginAttemptState)
		loginAttemptsMutex.Unlock()
	})

	request := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	request.RemoteAddr = "192.0.2.10:12345"
	keys := loginRateLimitKeys(request, "Viewer")
	for attempt := 0; attempt < loginAttemptLimit; attempt++ {
		assert.True(t, loginAttemptAllowed(keys))
		recordLoginFailure(keys)
	}
	assert.False(t, loginAttemptAllowed(keys))
	now = now.Add(loginAttemptWindow)
	assert.True(t, loginAttemptAllowed(keys))
	recordLoginFailure(keys)
	clearLoginFailures(keys)
	assert.True(t, loginAttemptAllowed(keys))
}

func TestAuthMiddlewareRejectsNonStringSessionUsernameWithoutPanic(t *testing.T) {
	originalStore := sessionStore
	sessionStore = sessions.NewCookieStore(bytes.Repeat([]byte{0x42}, 32))
	sessionStore.Options = &sessions.Options{Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	t.Cleanup(func() { sessionStore = originalStore })

	seedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	seedRecorder := httptest.NewRecorder()
	session, err := sessionStore.Get(seedRequest, "authentication")
	require.NoError(t, err)
	session.Values["username"] = 42
	require.NoError(t, session.Save(seedRequest, seedRecorder))
	cookies := seedRecorder.Result().Cookies()
	require.NotEmpty(t, cookies)

	request := httptest.NewRequest(http.MethodGet, "/api/server/status", nil)
	request.AddCookie(cookies[0])
	recorder := httptest.NewRecorder()
	AuthMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run for malformed session username")
	})).ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestAuthMiddlewareRevokesSessionAfterCredentialChange(t *testing.T) {
	testAuth := newAuthorizationTestAuth(t)
	require.NoError(t, testAuth.db.Create(&User{Username: "admin", Password: "hash-a", Role: UserRoleAdmin}).Error)
	originalAuth := auth
	originalStore := sessionStore
	originalCredentialKey := sessionCredentialKey
	auth = testAuth
	sessionStore = sessions.NewCookieStore(bytes.Repeat([]byte{0x24}, 32))
	sessionCredentialKey = bytes.Repeat([]byte{0x25}, 32)
	sessionStore.Options = &sessions.Options{Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}
	t.Cleanup(func() {
		auth = originalAuth
		sessionStore = originalStore
		sessionCredentialKey = originalCredentialKey
	})

	seedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	seedRecorder := httptest.NewRecorder()
	session, err := sessionStore.Get(seedRequest, "authentication")
	require.NoError(t, err)
	session.Values["username"] = "admin"
	credentialVersion, err := sessionCredentialVersion("hash-a")
	require.NoError(t, err)
	require.NotEqual(t, "hash-a", credentialVersion)
	session.Values["credential_version"] = credentialVersion
	require.NoError(t, session.Save(seedRequest, seedRecorder))
	require.NoError(t, testAuth.db.Model(&User{}).Where("username = ?", "admin").Update("password", "hash-b").Error)

	request := httptest.NewRequest(http.MethodGet, "/api/server/status", nil)
	request.AddCookie(seedRecorder.Result().Cookies()[0])
	recorder := httptest.NewRecorder()
	AuthMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("revoked session reached protected handler")
	})).ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
