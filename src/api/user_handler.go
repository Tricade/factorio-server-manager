package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"gorm.io/gorm"
)

func UnmarshallUserJson(body []byte, w http.ResponseWriter) (user User, resp interface{}, err error) {
	err = json.Unmarshal(body, &user)
	if err != nil {
		resp = fmt.Sprintf("Unable to parse the request body: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
	}
	return
}

// Handler for the Login

func LoginUser(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	// add resp to the response
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		resp = "Unable to parse login request"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		resp = "Unable to parse login request"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	credentials.Username = strings.TrimSpace(credentials.Username)
	if credentials.Username == "" || len(credentials.Username) > 256 || credentials.Password == "" || len(credentials.Password) > 1024 {
		resp = "Unable to parse login request"
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rateLimitKeys := loginRateLimitKeys(r, credentials.Username)
	if !loginAttemptAllowed(rateLimitKeys) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginAttemptWindow/time.Second)))
		resp = "Too many login attempts; try again later"
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	err = auth.checkPassword(credentials.Username, credentials.Password)
	if err != nil {
		recordLoginFailure(rateLimitKeys)
		resp = "Invalid username or password"
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	clearLoginFailures(rateLimitKeys)
	authenticatedUser, err := auth.getUser(credentials.Username)
	if err != nil {
		resp = "Unable to load authenticated user"
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	credentialVersion, err := sessionCredentialVersion(authenticatedUser.Password)
	if err != nil {
		resp = "Unable to initialize authenticated session"
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	session, resp, err := ReadSessionStore(w, r, "authentication")
	if err != nil {
		return
	}

	session.Values["username"] = credentials.Username
	session.Values["credential_version"] = credentialVersion
	session.Options.MaxAge = loginSessionMaxAge

	resp, err = SaveSession(w, r, session)
	if err != nil {
		return
	}

	log.Printf("User %q logged in successfully", credentials.Username)

	authenticatedUser.Password = ""
	resp = authenticatedUser
}

func LogoutUser(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	session, resp, err := ReadSessionStore(w, r, "authentication")
	if err != nil {
		return
	}

	delete(session.Values, "username")
	session.Options.MaxAge = -1

	resp, err = SaveSession(w, r, session)
	if err != nil {
		return
	}

	resp = "User logged out successfully."
}

func GetCurrentLogin(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	// add resp to the response
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	user, ok := authenticatedUserFromRequest(r)
	if !ok {
		resp = "Unable to read authenticated user"
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	user.Password = ""

	resp = user
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	users, err := auth.listUsers()
	if err != nil {
		resp = fmt.Sprintf("Error listing users: %s", err)
		log.Println(resp)
		return
	}
	for index := range users {
		users[index].Password = ""
	}

	resp = users
}

func AddUser(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	user, resp, err := UnmarshallUserJson(body, w)
	if err != nil {
		return
	}

	err = auth.addUser(user)
	if err != nil {
		resp = fmt.Sprintf("Error in adding user {%s}: %s", user.Username, err)
		log.Println(resp)
		if errors.Is(err, ErrInvalidUser) {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	resp = fmt.Sprintf("User: %s successfully added.", user.Username)
}

func RemoveUser(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	user, resp, err := UnmarshallUserJson(body, w)
	if err != nil {
		return
	}

	err = auth.deleteUser(user.Username)
	if err != nil {
		resp = fmt.Sprintf("Error in removing user {%s}, error: %s", user.Username, err)
		log.Println(resp)
		if strings.Contains(err.Error(), "single admin") {
			w.WriteHeader(http.StatusConflict)
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	resp = fmt.Sprintf("User: %s successfully removed.", user.Username)
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	var user struct {
		OldPassword        string `json:"old_password"`
		NewPassword        string `json:"new_password"`
		NewPasswordConfirm string `json:"new_password_confirmation"`
	}
	err = json.Unmarshal(body, &user)
	if err != nil {
		resp = fmt.Sprintf("Unable to parse the request body: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Only allow changing the authenticated account's own password.
	authenticatedUser, ok := authenticatedUserFromRequest(r)
	if !ok {
		resp = "Unable to read authenticated user"
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	username := authenticatedUser.Username

	// check if password for user is correct
	err = auth.checkPassword(username, user.OldPassword)
	if err != nil {
		resp = fmt.Sprintf("Password for user %s wrong", username)
		log.Println(resp)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// only run, when confirmation correct
	if user.NewPassword != user.NewPasswordConfirm {
		resp = "Password confirmation incorrect"
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = auth.changePassword(username, user.NewPassword)
	if err != nil {
		resp = fmt.Sprintf("Error changing password: %s", err)
		log.Println(resp)
		if errors.Is(err, ErrInvalidUser) {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	updatedUser, err := auth.getUser(username)
	if err != nil {
		resp = "Password changed, but the refreshed session could not be loaded"
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	credentialVersion, err := sessionCredentialVersion(updatedUser.Password)
	if err != nil {
		resp = "Password changed, but the refreshed session could not be secured"
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	session, sessionResp, err := ReadSessionStore(w, r, "authentication")
	if err != nil {
		resp = sessionResp
		return
	}
	session.Values["credential_version"] = credentialVersion
	session.Options.MaxAge = loginSessionMaxAge
	if sessionResp, err = SaveSession(w, r, session); err != nil {
		resp = sessionResp
		return
	}
	if username == "admin" {
		credentialPath := filepath.Join(filepath.Dir(bootstrap.GetConfig().SQLiteDatabaseFile), "initial-admin-password.txt")
		if removeErr := os.Remove(credentialPath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("Error removing one-time admin credential: %s", removeErr)
		}
	}

	resp = true
}

// GetServerSettings returns JSON response of server-settings.json file
