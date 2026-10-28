package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

// SettingsSnapshot returns a detached JSON-compatible settings map so API
// encoders never observe a concurrent replacement of the live map.
func (server *Server) SettingsSnapshot() map[string]interface{} {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	data, err := json.Marshal(server.Settings)
	if err != nil {
		log.Printf("Error copying Factorio server settings: %v", err)
		return map[string]interface{}{}
	}
	settings := make(map[string]interface{})
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("Error decoding copied Factorio server settings: %v", err)
		return map[string]interface{}{}
	}
	return settings
}

// UpdateServerSettings commits the on-disk settings (and, for modern
// Factorio, the separate admin list) before publishing the new in-memory map.
// The lifecycle mutex closes the middleware check/use gap with server start.
func (server *Server) UpdateServerSettings(updated map[string]interface{}) error {
	if updated == nil {
		return errors.New("server settings must be a JSON object")
	}
	settingsData, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal server settings: %w", err)
	}

	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.RLock()
	busy := server.Running || server.Stopping || server.starting
	version := server.Version
	stateMutex.RUnlock()
	if busy {
		return ErrServerActive
	}

	config := bootstrap.GetConfig()
	previousSettings, settingsExisted, err := readOptionalFile(config.SettingsFile)
	if err != nil {
		return fmt.Errorf("read previous server settings: %w", err)
	}
	if err := writeFileAtomically(config.SettingsFile, settingsData, 0644); err != nil {
		return fmt.Errorf("save server settings: %w", err)
	}

	if version.Greater(Version{0, 17, 0}) {
		adminData, marshalErr := json.MarshalIndent(updated["admins"], "", "  ")
		if marshalErr != nil {
			rollbackErr := restoreOptionalFile(config.SettingsFile, previousSettings, settingsExisted, 0644)
			return errors.Join(fmt.Errorf("marshal admin list: %w", marshalErr), rollbackErr)
		}
		if writeErr := writeFileAtomically(config.FactorioAdminFile, adminData, 0644); writeErr != nil {
			rollbackErr := restoreOptionalFile(config.SettingsFile, previousSettings, settingsExisted, 0644)
			return errors.Join(fmt.Errorf("save admin list: %w", writeErr), rollbackErr)
		}
	}

	stateMutex.Lock()
	server.Settings = updated
	stateMutex.Unlock()
	return nil
}

func readOptionalFile(path string) ([]byte, bool, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return contents, err == nil, err
}

func restoreOptionalFile(path string, contents []byte, existed bool, mode os.FileMode) error {
	if existed {
		if err := writeFileAtomically(path, contents, mode); err != nil {
			return fmt.Errorf("restore %s: %w", filepath.Base(path), err)
		}
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove newly-created %s: %w", filepath.Base(path), err)
	}
	return nil
}
