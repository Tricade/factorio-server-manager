package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type ModPortalStruct struct {
	DownloadsCount int                `json:"downloads_count"`
	Name           string             `json:"name"`
	Owner          string             `json:"owner"`
	Releases       []ModPortalRelease `json:"releases"`
	Summary        string             `json:"summary"`
	Title          string             `json:"title"`
}

type ModPortalRelease struct {
	DownloadURL string `json:"download_url"`
	FileName    string `json:"file_name"`
	InfoJSON    struct {
		FactorioVersion Version  `json:"factorio_version"`
		Dependencies    []string `json:"dependencies"`
		QualityRequired bool     `json:"quality_required,omitempty"`
	} `json:"info_json"`
	ReleasedAt    time.Time `json:"released_at"`
	Sha1          string    `json:"sha1"`
	Version       Version   `json:"version"`
	Compatibility bool      `json:"compatibility"`
}

var modPortalHTTPClient = &http.Client{Timeout: 2 * time.Minute}
var modPortalBaseURL = "https://mods.factorio.com"

// get all mods uploaded to the factorio modPortal
func ModPortalList() (interface{}, error, int) {
	snapshot := GetFactorioServer().Snapshot()
	installedVersion := snapshot.BaseModVersion
	if installedVersion == "" {
		installedVersion = snapshot.Version.ReleaseString()
	}
	var parsedVersion Version
	if err := parsedVersion.UnmarshalText([]byte(installedVersion)); err != nil {
		return "error", fmt.Errorf("parse installed Factorio version for mod search: %w", err), http.StatusInternalServerError
	}

	endpoint, err := url.Parse(modPortalBaseURL + "/api/mods")
	if err != nil {
		return "error", err, http.StatusInternalServerError
	}
	query := endpoint.Query()
	query.Set("page_size", "max")
	query.Set("version", parsedVersion.FactorioLine())
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "error", err, http.StatusInternalServerError
	}

	resp, err := modPortalHTTPClient.Do(req)
	if err != nil {
		return "error", err, http.StatusInternalServerError
	}
	defer resp.Body.Close()

	text, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "error", err, http.StatusInternalServerError
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(string(text)), resp.StatusCode
	}

	var jsonVal map[string]interface{}
	err = json.Unmarshal(text, &jsonVal)
	if err != nil {
		return "error", err, http.StatusInternalServerError
	}

	jsonVal["factorio_version"] = parsedVersion.FactorioLine()
	return jsonVal, nil, resp.StatusCode
}

// get the details (mod-info, releases, etc.) from a specific mod from the modPortal
func ModPortalModDetails(modId string) (ModPortalStruct, error, int) {
	var mod ModPortalStruct
	if err := ValidatePathElement(modId); err != nil {
		return mod, err, http.StatusBadRequest
	}

	// The full endpoint is required here: the short response deliberately omits
	// info_json.dependencies, which the install planner needs.
	req, err := http.NewRequest(http.MethodGet, modPortalBaseURL+"/api/mods/"+url.PathEscape(modId)+"/full", nil)
	if err != nil {
		return mod, err, http.StatusInternalServerError
	}

	resp, err := modPortalHTTPClient.Do(req)
	if err != nil {
		return mod, err, http.StatusInternalServerError
	}
	defer resp.Body.Close()

	text, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return mod, err, http.StatusInternalServerError
	}

	err = json.Unmarshal(text, &mod)
	if err != nil {
		return mod, err, http.StatusInternalServerError
	}

	if resp.StatusCode != http.StatusOK {
		return ModPortalStruct{}, errors.New(string(text)), resp.StatusCode
	}

	server := GetFactorioServer()

	installedBaseVersion := Version{}
	_ = installedBaseVersion.UnmarshalText([]byte(server.BaseModVersion))
	requiredVersion := NilVersion

	for key, release := range mod.Releases {
		requiredVersion = release.InfoJSON.FactorioVersion
		release.Compatibility = installedBaseVersion.SameFactorioLine(requiredVersion)
		mod.Releases[key] = release
	}

	return mod, nil, resp.StatusCode
}

// Log the user into factorio, so mods can be downloaded
func FactorioLogin(username string, password string) (error, int) {
	var err error

	resp, err := modPortalHTTPClient.PostForm("https://auth.factorio.com/api-login",
		url.Values{"require_game_ownership": {"true"}, "username": {username}, "password": {password}})

	if err != nil {
		return err, http.StatusInternalServerError
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	bodyString := string(bodyBytes)

	if resp.StatusCode != http.StatusOK {
		return errors.New(bodyString), resp.StatusCode
	}

	var successResponse []string
	err = json.Unmarshal(bodyBytes, &successResponse)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	credentials := Credentials{
		Username: username,
		Userkey:  successResponse[0],
	}

	err = credentials.Save()
	if err != nil {
		return err, http.StatusInternalServerError
	}

	return nil, http.StatusOK
}
