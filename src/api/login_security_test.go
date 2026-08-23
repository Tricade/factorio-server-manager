package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLoginSecurityTest(t *testing.T) {
	t.Helper()
	originalAuth := auth
	originalStore := sessionStore
	originalCredentialKey := sessionCredentialKey
	auth = newAuthorizationTestAuth(t)
	require.NoError(t, auth.addUser(User{Username: "admin", Password: "correct-password", Role: UserRoleAdmin}))
	sessionStore = sessions.NewCookieStore(bytes.Repeat([]byte{0x33}, 32))
	sessionCredentialKey = bytes.Repeat([]byte{0x34}, 32)
	sessionStore.Options = &sessions.Options{Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: loginSessionMaxAge}
	loginAttemptsMutex.Lock()
	loginAttempts = make(map[string]loginAttemptState)
	loginAttemptsMutex.Unlock()
	t.Cleanup(func() {
		auth = originalAuth
		sessionStore = originalStore
		sessionCredentialKey = originalCredentialKey
		loginAttemptsMutex.Lock()
		loginAttempts = make(map[string]loginAttemptState)
		loginAttemptsMutex.Unlock()
	})
}

func runLoginRequest(body, remoteAddress string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	request.RemoteAddr = remoteAddress
	recorder := httptest.NewRecorder()
	LoginUser(recorder, request)
	return recorder
}

func TestLoginUsesStrictJSONAndGenericCredentialFailure(t *testing.T) {
	setupLoginSecurityTest(t)
	unknownField := runLoginRequest(`{"username":"admin","password":"correct-password","extra":true}`, "192.0.2.1:1000")
	assert.Equal(t, http.StatusBadRequest, unknownField.Code)

	known := runLoginRequest(`{"username":"admin","password":"wrong-password"}`, "192.0.2.2:1000")
	unknown := runLoginRequest(`{"username":"does-not-exist","password":"wrong-password"}`, "192.0.2.3:1000")
	assert.Equal(t, http.StatusUnauthorized, known.Code)
	assert.Equal(t, http.StatusUnauthorized, unknown.Code)
	assert.Equal(t, known.Body.String(), unknown.Body.String())
	assert.NotContains(t, known.Body.String(), "admin")
	assert.NotContains(t, unknown.Body.String(), "does-not-exist")
}

func TestLoginRateLimitAndSessionLifetime(t *testing.T) {
	setupLoginSecurityTest(t)
	for attempt := 0; attempt < loginAttemptLimit; attempt++ {
		recorder := runLoginRequest(`{"username":"admin","password":"wrong-password"}`, "192.0.2.4:1000")
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	}
	limited := runLoginRequest(`{"username":"admin","password":"wrong-password"}`, "192.0.2.4:1000")
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.NotEmpty(t, limited.Header().Get("Retry-After"))

	// A different IP is independently limited and a successful login receives
	// a finite HttpOnly session cookie.
	clearLoginFailures(loginRateLimitKeys(httptest.NewRequest(http.MethodPost, "/api/login", nil), "admin"))
	success := runLoginRequest(`{"username":"admin","password":"correct-password"}`, "192.0.2.5:1000")
	require.Equal(t, http.StatusOK, success.Code)
	cookies := success.Result().Cookies()
	require.NotEmpty(t, cookies)
	assert.Equal(t, loginSessionMaxAge, cookies[0].MaxAge)
	assert.True(t, cookies[0].HttpOnly)
}
