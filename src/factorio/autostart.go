package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const autostartSettingsFileName = "autostart.json"

// AutostartSettings belongs to one manager instance. Multiple manager
// containers therefore retain independent startup behavior as long as they use
// separate persistent /opt/fsm-data mounts.
type AutostartSettings struct {
	Enabled bool `json:"enabled"`
}

var autostartSettingsMutex sync.Mutex
var autostartSettingsPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), autostartSettingsFileName)
}
var autostartSettingsDefault = func() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(bootstrap.GetConfig().Autostart))
	return err == nil && enabled
}

// LoadAutostartSettings returns the persisted UI preference. The legacy
// command-line/environment flag remains a first-start fallback only, allowing
// existing deployments to migrate without overriding later UI choices.
func LoadAutostartSettings() (AutostartSettings, error) {
	autostartSettingsMutex.Lock()
	defer autostartSettingsMutex.Unlock()
	return loadAutostartSettings()
}

func loadAutostartSettings() (AutostartSettings, error) {
	path := autostartSettingsPath()
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AutostartSettings{Enabled: autostartSettingsDefault()}, nil
	}
	if err != nil {
		return AutostartSettings{}, fmt.Errorf("read autostart settings: %w", err)
	}

	var settings AutostartSettings
	if err := json.Unmarshal(contents, &settings); err != nil {
		return AutostartSettings{}, fmt.Errorf("decode autostart settings: %w", err)
	}
	return settings, nil
}

// SetAutostartEnabled persists the behavior for the next manager-container
// start. It deliberately does not start or stop the currently running game.
func SetAutostartEnabled(enabled bool) (AutostartSettings, error) {
	autostartSettingsMutex.Lock()
	defer autostartSettingsMutex.Unlock()

	settings := AutostartSettings{Enabled: enabled}
	path := autostartSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return AutostartSettings{}, fmt.Errorf("create autostart settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".autostart-*")
	if err != nil {
		return AutostartSettings{}, fmt.Errorf("create autostart settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		temporary.Close()
		return AutostartSettings{}, fmt.Errorf("write autostart settings: %w", err)
	}
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return AutostartSettings{}, fmt.Errorf("restrict autostart settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return AutostartSettings{}, fmt.Errorf("close autostart settings: %w", err)
	}
	if err := replaceAutostartSettingsFile(temporaryPath, path); err != nil {
		return AutostartSettings{}, err
	}
	return settings, nil
}

func replaceAutostartSettingsFile(temporaryPath, path string) error {
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	} else if _, statErr := os.Stat(path); statErr != nil {
		return fmt.Errorf("activate autostart settings: %w", err)
	}

	// Windows cannot replace an existing file with os.Rename. The production
	// container uses Linux, but this small rollback path keeps local development
	// and tests functional as well.
	backup, err := os.CreateTemp(filepath.Dir(path), ".autostart-backup-*")
	if err != nil {
		return fmt.Errorf("prepare autostart settings replacement: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return fmt.Errorf("close autostart settings backup: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare autostart settings backup path: %w", err)
	}
	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("back up autostart settings: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if restoreErr := os.Rename(backupPath, path); restoreErr != nil {
			return fmt.Errorf("activate autostart settings: %w (restore failed: %v)", err, restoreErr)
		}
		return fmt.Errorf("activate autostart settings: %w", err)
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old autostart settings: %w", err)
	}
	return nil
}
