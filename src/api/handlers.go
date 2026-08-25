package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/sessions"
	"gorm.io/gorm"

	"github.com/gorilla/mux"
)

const readHttpBodyError = "Could not read the Request Body."
const maximumJSONRequestBodyBytes int64 = 1 * 1024 * 1024
const multipartRequestOverheadBytes int64 = 1 * 1024 * 1024

type JSONResponseFileInput struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,string"`
	Error     string      `json:"error"`
	ErrorKeys []int       `json:"errorkeys"`
}

func WriteResponse(w http.ResponseWriter, data interface{}) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error writing response: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func ReadRequestBody(w http.ResponseWriter, r *http.Request) (body []byte, resp interface{}, err error) {
	if r.Body == nil {
		resp = fmt.Sprintf("%s: no request body", readHttpBodyError)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		err = errors.New("no request body")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maximumJSONRequestBodyBytes)
	body, err = io.ReadAll(r.Body)
	if err != nil {
		body = nil
		resp = fmt.Sprintf("%s: %s", readHttpBodyError, err)
		log.Println(resp)
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	return
}

func ReadSessionStore(w http.ResponseWriter, r *http.Request, name string) (session *sessions.Session, resp interface{}, err error) {
	session, err = sessionStore.Get(r, name)
	if err != nil {
		resp = fmt.Sprintf("Error reading session cookie [%s]: %s", name, err)
		log.Println(resp)
		if session != nil {
			session.Options.MaxAge = -1
			err2 := session.Save(r, w)
			if err2 != nil {
				log.Printf("Error deleting session cookie: %s", err2)
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
	return
}

func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) (resp interface{}, err error) {
	err = session.Save(r, w)
	if err != nil {
		resp = fmt.Sprintf("Error saving session cookie: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
	}
	return
}

// Lists all save files in the factorio/saves directory
func ListSaves(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	latestParam := r.URL.Query().Get("latest")

	var withLatest bool

	if latestParam != "" {
		var err error
		withLatest, err = strconv.ParseBool(latestParam)
		if err != nil {
			resp = fmt.Sprintf("Error parsing latestParam: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	savesList, err := factorio.ListSaves()
	if err != nil {
		resp = fmt.Sprintf("Error listing save files: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// get actual latest and add name
	// but only if requested
	if withLatest && len(savesList) != 0 {
		latestSave, err := factorio.GetLatestSave()
		if err != nil {
			resp = fmt.Sprintf("Error getting latest save: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		latestSave.Name = fmt.Sprintf("Load Latest (%s)", latestSave.Name)
		savesList = append(savesList, latestSave)
	}

	resp = savesList
}

func DLSave(w http.ResponseWriter, r *http.Request) {
	config := bootstrap.GetConfig()
	save, err := factorio.FindSave(mux.Vars(r)["save"])
	if err != nil {
		http.Error(w, "save not found", http.StatusNotFound)
		return
	}
	saveName := filepath.Join(config.FactorioSavesDir, save.Name)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": save.Name}))
	log.Printf("%s downloading: %s", r.Host, saveName)

	http.ServeFile(w, r, saveName)
}

func UploadSave(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	log.Println("Uploading save file")

	config := bootstrap.GetConfig()
	if config.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maximumMultipartRequestSize(config.MaxUploadSize))
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			resp = uploadLimitMessage("Save file", config.MaxUploadSize)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			resp = fmt.Sprintf("Unable to parse save upload: %s", err)
			w.WriteHeader(http.StatusBadRequest)
		}
		log.Println(resp)
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["savefile"]
	if len(files) != 1 {
		resp = "Exactly one save file must be provided"
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, saveFile := range files {
		if config.MaxUploadSize > 0 && saveFile.Size > config.MaxUploadSize {
			resp = uploadLimitMessage("Save file", config.MaxUploadSize)
			log.Println(resp)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if err := factorio.ValidatePathElement(saveFile.Filename); err != nil {
			resp = fmt.Sprintf("Invalid save filename: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ext := filepath.Ext(saveFile.Filename)
		if !strings.EqualFold(ext, ".zip") || strings.HasSuffix(strings.ToLower(saveFile.Filename), ".tmp.zip") {
			// Only zip-files allowed
			resp = fmt.Sprintf("Fileformat {%s} is not allowed", ext)
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		file, err := saveFile.Open()
		if err != nil {
			resp = fmt.Sprintf("Error opening uploaded saveFile: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer file.Close()

		temporary, err := os.CreateTemp(config.FactorioSavesDir, ".save-upload-*.zip")
		if err != nil {
			resp = fmt.Sprintf("Error creating temporary save upload: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)

		_, err = io.Copy(temporary, file)
		if err != nil {
			temporary.Close()
			resp = fmt.Sprintf("Error copying uploaded save to disk: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := temporary.Sync(); err != nil {
			temporary.Close()
			resp = fmt.Sprintf("Error syncing uploaded save: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := temporary.Close(); err != nil {
			resp = fmt.Sprintf("Error closing uploaded save: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := os.Chmod(temporaryPath, 0644); err != nil {
			resp = fmt.Sprintf("Error setting save permissions: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := factorio.ActivateSaveUpload(temporaryPath, saveFile.Filename); err != nil {
			resp = fmt.Sprintf("Error activating uploaded save: %s", err)
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
	}

	resp = "Uploading files successful"
}

func maximumMultipartRequestSize(maximumFileBytes int64) int64 {
	const maximumInt64 = int64(1<<63 - 1)
	if maximumFileBytes > maximumInt64-multipartRequestOverheadBytes {
		return maximumInt64
	}
	return maximumFileBytes + multipartRequestOverheadBytes
}

func uploadLimitMessage(subject string, maximumFileBytes int64) string {
	if maximumFileBytes > 0 && maximumFileBytes%(1024*1024) == 0 {
		return fmt.Sprintf("%s exceeds the configured upload limit of %d MiB", subject, maximumFileBytes/(1024*1024))
	}
	return fmt.Sprintf("%s exceeds the configured upload limit of %d bytes", subject, maximumFileBytes)
}

// Deletes provided save
func RemoveSave(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	vars := mux.Vars(r)
	name := vars["save"]

	save, err := factorio.FindSave(name)
	if err != nil {
		resp = fmt.Sprintf("Error finding save {%s}: %s", name, err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = save.Remove()
	if err != nil {
		resp = fmt.Sprintf("Error removing save {%s}: %s", name, err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// save was removed
	resp = fmt.Sprintf("Removed save: %s", save.Name)
}

// Launches Factorio server binary with --create flag to create save
// Url must include save name for creation of savefile
func CreateSaveHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	vars := mux.Vars(r)
	saveName := vars["save"]

	if saveName == "" {
		resp = fmt.Sprintf("Error creating save, no save name provided: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if filepath.Ext(saveName) == "" {
		saveName += ".zip"
	}
	if err := factorio.ValidatePathElement(saveName); err != nil || !strings.EqualFold(filepath.Ext(saveName), ".zip") {
		resp = "Error creating save: invalid save name"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	config := bootstrap.GetConfig()
	saveFile := filepath.Join(config.FactorioSavesDir, saveName)
	cmdOut, err := factorio.CreateSave(saveFile)
	if err != nil {
		resp = fmt.Sprintf("Error creating save {%s}: %s", saveName, err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = fmt.Sprintf("Save %s created successfully. Command output: \n%s", saveName, cmdOut)
}

// LogTail returns last lines of the factorio-current.log file
func LogTail(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	config := bootstrap.GetConfig()
	resp, err = factorio.TailLog()
	if err != nil {
		resp = fmt.Sprintf("Could not tail %s: %s", config.FactorioLog, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// LoadConfig returns JSON response of config.ini file
func LoadConfig(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	config := bootstrap.GetConfig()
	configContents, err := factorio.LoadConfig(config.FactorioConfigFile)
	if err != nil {
		resp = fmt.Sprintf("Could not retrieve config.ini: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = configForRequest(configContents, requestUserIsAdministrator(r))

	log.Printf("Sent config.ini response")
}

func StartServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	var server = factorio.GetFactorioServer()
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	if factorio.WorldGenerationBusy() {
		resp = "A world preview or creation is still running"
		w.WriteHeader(http.StatusConflict)
		return
	}

	if server.IsBusy() {
		resp = "Factorio server is already running or stopping"
		w.WriteHeader(http.StatusConflict)
		return
	}

	log.Printf("Starting Factorio server.")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	var request struct {
		Savefile string `json:"savefile"`
		BindIP   string `json:"bindip"`
		Port     int    `json:"port"`
	}
	err = json.Unmarshal(body, &request)
	if err != nil {
		resp = fmt.Sprintf("Error unmarshalling server settings JSON: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check if savefile was submitted with request to start server.
	if request.Savefile == "" {
		resp = "Error starting Factorio server: No save file provided"
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	parsedIP := net.ParseIP(request.BindIP)
	if parsedIP == nil || parsedIP.To4() == nil {
		resp = "Error starting Factorio server: invalid IPv4 bind address"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if request.Port < 1 || request.Port > 65535 {
		resp = "Error starting Factorio server: port must be between 1 and 65535"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Starting Factorio server on %s:%d with save %q", request.BindIP, request.Port, request.Savefile)
	startResult, startErr := server.Start(request.BindIP, request.Port, request.Savefile)
	if startErr != nil {
		resp = fmt.Sprintf("Error starting Factorio server: %s", startErr)
		if errors.Is(startErr, factorio.ErrServerActive) || errors.Is(startErr, factorio.ErrWorldGenerationBusy) {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	deadline := time.NewTimer(4 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()

	started := false
	for !started {
		select {
		case runErr := <-startResult:
			if runErr != nil {
				resp = fmt.Sprintf("Error starting Factorio server: %s", runErr)
				if errors.Is(runErr, factorio.ErrWorldGenerationBusy) {
					w.WriteHeader(http.StatusConflict)
					return
				}
			} else {
				resp = "Factorio server exited before startup completed"
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		case <-ticker.C:
			started = server.GetRunning()
		case <-deadline.C:
			resp = "Error starting Factorio server: startup timed out"
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
	}

	resp = fmt.Sprintf("Factorio server with save: %s started on port: %d", request.Savefile, request.Port)
	log.Println(resp)
}

func StopServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var server = factorio.GetFactorioServer()
	if server.IsStopping() {
		resp = "Factorio server is already stopping"
		w.WriteHeader(http.StatusConflict)
		return
	}
	if server.GetRunning() {
		err := server.Stop()
		if err != nil {
			resp = fmt.Sprintf("Error stopping factorio server: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp = "Factorio safe shutdown requested"
		log.Println(resp)
	} else {
		resp = "Factorio server is not running"
		w.WriteHeader(http.StatusConflict)
		return
	}
}

func KillServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var server = factorio.GetFactorioServer()
	if server.GetRunning() {
		err := server.Kill()
		if err != nil {
			resp = fmt.Sprintf("Error killing factorio server: %s", err)
			log.Println(resp)
			return
		}

		log.Printf("Killed Factorio server.")
		resp = fmt.Sprintf("Factorio server killed")
	} else {
		resp = "Factorio server is not running"
		w.WriteHeader(http.StatusBadRequest)
	}
}

func CheckServer(w http.ResponseWriter, r *http.Request) {
	defer func() {
		WriteResponse(w, factorio.GetFactorioServer().Snapshot())
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func FactorioVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	snapshot := factorio.GetFactorioServer().Snapshot()
	resp["version"] = snapshot.Version.String()
	resp["base_mod_version"] = snapshot.BaseModVersion
}

func FactorioReleaseStatus(w http.ResponseWriter, r *http.Request) {
	status, err := factorio.GetReleaseStatus()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading official Factorio release status: %s", err), http.StatusBadGateway)
		return
	}
	status = releaseStatusForRequest(status, requestUserIsAdministrator(r))
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

func releaseStatusForRequest(status factorio.ReleaseStatus, administrator bool) factorio.ReleaseStatus {
	if !administrator && status.MetadataError != "" {
		status.MetadataError = "Official release metadata is temporarily unavailable."
	}
	return status
}

func GetGameMode(w http.ResponseWriter, r *http.Request) {
	status, err := factorio.GetGameModeStatus()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading Factorio game mode: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

func UpdateGameMode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode factorio.GameMode `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse the request body: %s", err), http.StatusBadRequest)
		return
	}
	status, err := factorio.SetGameMode(request.Mode)
	if err != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(err, factorio.ErrServerActive) {
			statusCode = http.StatusLocked
		}
		http.Error(w, fmt.Sprintf("Error changing Factorio game mode: %s", err), statusCode)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

// InstallFactorioRelease installs a supported official Factorio headless release.
// The requested rolling channel or exact version is validated before download.
func InstallFactorioRelease(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var request struct {
		Target  string `json:"target"`
		Channel string `json:"channel"` // backwards-compatible with 0.11.0 clients
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		resp = fmt.Sprintf("Unable to parse the request body: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	target := request.Target
	if target == "" {
		target = request.Channel
	}
	normalizedTarget, err := factorio.NormalizeReleaseTarget(target)
	if err != nil {
		resp = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	server := factorio.GetFactorioServer()
	if server.GetRunning() || server.IsStopping() {
		resp = "Stop the Factorio server before installing a release"
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err := factorio.InstallRelease(normalizedTarget); err != nil {
		resp = fmt.Sprintf("Error installing Factorio %s release: %s", normalizedTarget, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp = map[string]string{
		"target":            normalizedTarget,
		"installed_version": factorio.GetFactorioServer().Snapshot().BaseModVersion,
		"message":           "Factorio release installed",
	}
}

// Unmarshall the User object from the given bytearray
// This function has side effects (it will write to resp and to w, in case of an error)
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
		resp = fmt.Sprintf("Password confirmation incorrect")
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
func GetServerSettings(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var server = factorio.GetFactorioServer()
	resp = serverSettingsForRequest(server.SettingsSnapshot(), requestUserIsAdministrator(r))

	log.Printf("Sent server settings response")
}

// serverSettingsForRequest keeps read-only operational settings visible to
// viewers without exposing Factorio account credentials or game passwords.
// SettingsSnapshot already returns a detached map, so removing fields here
// cannot mutate the running server configuration.
func serverSettingsForRequest(settings map[string]interface{}, administrator bool) map[string]interface{} {
	if administrator {
		return settings
	}
	for name := range settings {
		if sensitiveSettingName(name) {
			delete(settings, name)
		}
	}
	return settings
}

func configForRequest(config map[string]map[string]string, administrator bool) map[string]map[string]string {
	if administrator {
		return config
	}
	for section, values := range config {
		if sensitiveConfigLocationName(section) {
			delete(config, section)
			continue
		}
		for name := range values {
			if sensitiveSettingName(name) || sensitiveConfigLocationName(name) {
				delete(values, name)
			}
		}
	}
	return config
}

func sensitiveSettingName(name string) bool {
	compact := strings.NewReplacer("_", "", "-", "", " ", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(name)))
	for _, marker := range []string{"username", "password", "token", "secret", "credential", "userkey", "apikey", "accesskey", "privatekey", "signingkey", "encryptionkey", "proxy", "email"} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	return false
}

func sensitiveConfigLocationName(name string) bool {
	compact := strings.NewReplacer("_", "", "-", "", " ", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(name)))
	return strings.Contains(compact, "path") || strings.Contains(compact, "directory") || strings.HasSuffix(compact, "file") || strings.Contains(compact, "filename")
}

func UpdateServerSettings(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}
	log.Printf("Received server settings update request")
	var server = factorio.GetFactorioServer()

	var updatedSettings map[string]interface{}
	if err = json.Unmarshal(body, &updatedSettings); err != nil {
		resp = fmt.Sprintf("Error unmarhaling server settings JSON: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := server.UpdateServerSettings(updatedSettings); err != nil {
		resp = fmt.Sprintf("Failed to save server settings: %v", err)
		log.Println(resp)
		if errors.Is(err, factorio.ErrServerActive) {
			w.WriteHeader(http.StatusLocked)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	log.Printf("Saved Factorio server settings in server-settings.json")

	resp = fmt.Sprintf("Settings successfully saved")
}
