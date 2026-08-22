package factorio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type GameMode string

const (
	GameModeFactorio GameMode = "factorio"
	GameModeSpaceAge GameMode = "space-age"
	GameModeCustom   GameMode = "custom"
)

var spaceAgeFeatureMods = []string{"elevated-rails", "quality", "space-age"}

type GameModeFeature struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
}

type GameModeStatus struct {
	Mode              GameMode          `json:"mode"`
	SpaceAgeAvailable bool              `json:"space_age_available"`
	Features          []GameModeFeature `json:"features"`
}

func GetGameModeStatus() (GameModeStatus, error) {
	config := bootstrap.GetConfig()
	modList, err := newModSimpleList(config.FactorioModsDir)
	if err != nil {
		return GameModeStatus{}, err
	}

	status := GameModeStatus{SpaceAgeAvailable: true}
	enabledCount := 0
	for _, name := range spaceAgeFeatureMods {
		_, err := os.Stat(filepath.Join(config.FactorioDir, "data", name, "info.json"))
		available := err == nil
		if err != nil && !os.IsNotExist(err) {
			return GameModeStatus{}, fmt.Errorf("inspect built-in mod %s: %w", name, err)
		}
		enabled := modList.IsEnabled(name)
		status.Features = append(status.Features, GameModeFeature{Name: name, Available: available, Enabled: enabled})
		status.SpaceAgeAvailable = status.SpaceAgeAvailable && available
		if enabled {
			enabledCount++
		}
	}

	switch enabledCount {
	case 0:
		status.Mode = GameModeFactorio
	case len(spaceAgeFeatureMods):
		status.Mode = GameModeSpaceAge
	default:
		status.Mode = GameModeCustom
	}
	return status, nil
}

func SetGameMode(mode GameMode) (GameModeStatus, error) {
	if mode != GameModeFactorio && mode != GameModeSpaceAge {
		return GameModeStatus{}, fmt.Errorf("unsupported game mode %q", mode)
	}
	status, err := GetGameModeStatus()
	if err != nil {
		return GameModeStatus{}, err
	}
	if mode == GameModeSpaceAge && !status.SpaceAgeAvailable {
		return status, errors.New("Space Age data is not available in this Factorio installation")
	}

	enabled := []string{}
	if mode == GameModeSpaceAge {
		enabled = spaceAgeFeatureMods
	}
	if err := setBuiltInFeatureMods(bootstrap.GetConfig().FactorioModsDir, enabled); err != nil {
		return GameModeStatus{}, err
	}
	return GetGameModeStatus()
}

// setBuiltInFeatureMods writes every expansion feature explicitly. Factorio
// treats omitted built-in mods as defaults and may enable them while creating a
// world, so a base-game profile must record false entries instead of relying on
// their absence from mod-list.json.
func setBuiltInFeatureMods(modDirectory string, enabled []string) error {
	enabledSet := make(map[string]bool, len(enabled))
	for _, name := range enabled {
		enabledSet[name] = true
	}
	states := make(map[string]bool, len(spaceAgeFeatureMods))
	for _, name := range spaceAgeFeatureMods {
		states[name] = enabledSet[name]
	}
	modList, err := newModSimpleList(modDirectory)
	if err != nil {
		return err
	}
	return modList.SetModsEnabled(states)
}
