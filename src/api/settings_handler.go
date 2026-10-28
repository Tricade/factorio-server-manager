package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

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
	if updatedSettings == nil {
		resp = "Server settings must be a JSON object"
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

	resp = "Settings successfully saved"
}
