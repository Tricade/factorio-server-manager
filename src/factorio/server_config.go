package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

var factorioVersionPattern = regexp.MustCompile(`Version.*?((\d+\.)?(\d+\.)?(\*|\d+)+)`)

func readFactorioVersionMetadata() (Version, string, error) {
	config := bootstrap.GetConfig()
	var output []byte
	var err error
	if config.GlibcCustom == "true" {
		output, err = exec.Command(config.GlibcLocation, "--library-path", config.GlibcLibLoc, config.FactorioBinary, "--version").Output()
	} else {
		output, err = exec.Command(config.FactorioBinary, "--version").Output()
	}
	if err != nil {
		return Version{}, "", fmt.Errorf("run Factorio version command: %w", err)
	}

	found := factorioVersionPattern.FindStringSubmatch(string(output))
	if len(found) < 2 {
		return Version{}, "", fmt.Errorf("parse Factorio version output: version marker missing")
	}
	var version Version
	if err := version.UnmarshalText([]byte(found[1])); err != nil {
		return Version{}, "", fmt.Errorf("parse Factorio version: %w", err)
	}

	baseModInfoFile := filepath.Join(config.FactorioBaseModDir, "info.json")
	baseModInfo, err := os.ReadFile(baseModInfoFile)
	if err != nil {
		return Version{}, "", fmt.Errorf("read base mod info: %w", err)
	}
	var modInfo ModInfo
	if err := json.Unmarshal(baseModInfo, &modInfo); err != nil {
		return Version{}, "", fmt.Errorf("decode base mod info: %w", err)
	}
	return version, modInfo.Version, nil
}

// RefreshFactorioVersion updates only executable metadata. Unlike rebuilding the
// Server singleton it preserves the stopped/running state, selected save and
// settings, so replacing a Factorio release cannot implicitly autostart or reset
// the current server state.
func RefreshFactorioVersion() error {
	version, baseModVersion, err := readFactorioVersionMetadata()
	if err != nil {
		return err
	}
	stateMutex.Lock()
	defer stateMutex.Unlock()
	instantiated.Version = version
	instantiated.BaseModVersion = baseModVersion
	return nil
}

func NewFactorioServer() error {
	config := bootstrap.GetConfig()
	if err := os.MkdirAll(config.FactorioConfigDir, 0755); err != nil {
		return fmt.Errorf("create Factorio config directory: %w", err)
	}
	settings, err := readServerSettings(config.SettingsFile)
	if errors.Is(err, os.ErrNotExist) {
		settings, err = readServerSettings(filepath.Join(config.FactorioDir, "data", "server-settings.example.json"))
		if err != nil {
			return fmt.Errorf("read example server settings: %w", err)
		}
		contents, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("encode example server settings: %w", err)
		}
		if err := writeFileAtomically(config.SettingsFile, contents, 0644); err != nil {
			return fmt.Errorf("initialize server settings: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read server settings: %w", err)
	}
	version, baseVersion, err := readFactorioVersionMetadata()
	if err != nil {
		return err
	}
	if version.Greater(Version{0, 17, 0}) {
		admins, err := readServerAdmins(config.FactorioAdminFile)
		if errors.Is(err, os.ErrNotExist) {
			admins = []string{}
			if err := writeFileAtomically(config.FactorioAdminFile, []byte("[]"), 0664); err != nil {
				return fmt.Errorf("initialize server admin list: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("read server admin list: %w", err)
		}
		settings["admins"] = admins
	}
	SetFactorioServer(Server{Settings: settings, Version: version, BaseModVersion: baseVersion})
	return nil
}

// ReloadFactorioServerProfile refreshes the stopped server singleton after a
// profile has replaced the active config directory. It deliberately does not
// start Factorio; selecting a profile and starting its save remain separate
// user actions.
func ReloadFactorioServerProfile(bindIP string, port int, savefile string) error {
	server := GetFactorioServer()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.RLock()
	busy := server.Running || server.Stopping || server.starting
	stateMutex.RUnlock()
	if busy {
		return errors.New("stop Factorio before reloading a profile")
	}

	config := bootstrap.GetConfig()
	settingsFile, err := os.Open(config.SettingsFile)
	if err != nil {
		return fmt.Errorf("open profile server settings: %w", err)
	}
	defer settingsFile.Close()

	settings, err := decodeServerSettings(settingsFile)
	if err != nil {
		return fmt.Errorf("decode profile server settings: %w", err)
	}

	snapshot := server.Snapshot()
	if snapshot.Version.Greater(Version{0, 17, 0}) {
		admins, err := readServerAdmins(config.FactorioAdminFile)
		if errors.Is(err, os.ErrNotExist) {
			admins = []string{}
		} else if err != nil {
			return fmt.Errorf("read profile admin list: %w", err)
		}
		settings["admins"] = admins
	}

	if bindIP == "" {
		bindIP = config.FactorioIP
	}
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	if port == 0 {
		port = 34197
	}

	stateMutex.Lock()
	server.Settings = settings
	server.BindIP = bindIP
	server.Port = port
	server.Savefile = savefile
	server.Stopping = false
	server.startupReady = false
	server.stopSignaled = false
	stateMutex.Unlock()
	server.broadcastStatus()
	return nil
}

// A native configuration or profile archive is not necessarily written through
// the HTTP API. Validate the JSON root before assigning the modern admin list.
const maximumServerSettingsBytes = 16 << 20

func decodeServerSettings(reader io.Reader) (map[string]interface{}, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, maximumServerSettingsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read settings JSON: %w", err)
	}
	if len(contents) > maximumServerSettingsBytes {
		return nil, errors.New("server settings exceed the size limit")
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(contents, &settings); err != nil {
		return nil, fmt.Errorf("decode server settings: %w", err)
	}
	if settings == nil {
		return nil, errors.New("server settings must be a JSON object")
	}
	return settings, nil
}

func readServerSettings(path string) (map[string]interface{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return decodeServerSettings(file)
}

func readServerAdmins(path string) ([]string, error) {
	contents, err := readBoundedRegularFile(path, maximumServerSettingsBytes, false)
	if err != nil {
		return nil, err
	}
	var admins []string
	if err := json.Unmarshal(contents, &admins); err != nil {
		return nil, fmt.Errorf("decode server admin list: %w", err)
	}
	if admins == nil {
		return nil, errors.New("server admin list must be an array of names")
	}
	return admins, nil
}
