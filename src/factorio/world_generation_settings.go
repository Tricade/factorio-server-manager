package factorio

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type WorldPreset struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WorldPlanet struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WorldControlDefinition struct {
	Name             string `json:"name"`
	Label            string `json:"label"`
	Planet           string `json:"planet"`
	Category         string `json:"category"`
	SupportsRichness bool   `json:"supports_richness"`
	CanDisable       bool   `json:"can_disable"`
}

type WorldGenerationOptions struct {
	FactorioVersion string                   `json:"factorio_version"`
	GameMode        GameMode                 `json:"game_mode"`
	Planets         []WorldPlanet            `json:"planets"`
	Presets         []WorldPreset            `json:"presets"`
	Controls        []WorldControlDefinition `json:"controls"`
	PreviewSizes    []int                    `json:"preview_sizes"`
}

type WorldControlSettings struct {
	Frequency *float64 `json:"frequency,omitempty"`
	Size      *float64 `json:"size,omitempty"`
	Richness  *float64 `json:"richness,omitempty"`
}

type WorldGenerationRequest struct {
	Name          string                          `json:"name,omitempty"`
	Preset        string                          `json:"preset"`
	Seed          uint64                          `json:"seed"`
	Planet        string                          `json:"planet,omitempty"`
	PreviewSize   int                             `json:"preview_size,omitempty"`
	Width         *uint32                         `json:"width,omitempty"`
	Height        *uint32                         `json:"height,omitempty"`
	StartingArea  *float64                        `json:"starting_area,omitempty"`
	PeacefulMode  *bool                           `json:"peaceful_mode,omitempty"`
	NoEnemiesMode *bool                           `json:"no_enemies_mode,omitempty"`
	Controls      map[string]WorldControlSettings `json:"controls,omitempty"`
}

var worldPresets = []WorldPreset{
	{Name: "default", Label: "Default", Description: "Factorio's balanced default world."},
	{Name: "rich-resources", Label: "Rich resources", Description: "Default layout with much richer deposits."},
	{Name: "marathon", Label: "Marathon", Description: "More expensive technology and a longer progression."},
	{Name: "death-world", Label: "Death world", Description: "Denser enemies and faster evolution."},
	{Name: "death-world-marathon", Label: "Death world marathon", Description: "Death world pressure with marathon costs."},
	{Name: "rail-world", Label: "Rail world", Description: "Large, widely spaced deposits built for trains."},
	{Name: "ribbon-world", Label: "Ribbon world", Description: "A narrow map with resources adjusted for the limited space."},
	{Name: "lakes", Label: "Lakes", Description: "A wetter landscape with larger connected lakes."},
	{Name: "island", Label: "Island", Description: "Start on a bounded island surrounded by water."},
}

var baseWorldPlanets = []WorldPlanet{
	{Name: "nauvis", Label: "Nauvis", Description: "The starting planet and primary factory world."},
}

var spaceAgeWorldPlanets = []WorldPlanet{
	{Name: "vulcanus", Label: "Vulcanus", Description: "Volcanic terrain, lava and heavy industry resources."},
	{Name: "gleba", Label: "Gleba", Description: "Wet organic terrain, agriculture and pentapods."},
	{Name: "fulgora", Label: "Fulgora", Description: "Scrap islands separated by oil oceans."},
	{Name: "aquilo", Label: "Aquilo", Description: "Frozen islands with scarce geothermal resources."},
}

var baseWorldControls = []WorldControlDefinition{
	{Name: "iron-ore", Label: "Iron ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "copper-ore", Label: "Copper ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "coal", Label: "Coal", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "stone", Label: "Stone", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "uranium-ore", Label: "Uranium ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "crude-oil", Label: "Crude oil", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "water", Label: "Water", Planet: "nauvis", Category: "terrain", CanDisable: true},
	{Name: "trees", Label: "Trees", Planet: "nauvis", Category: "terrain", CanDisable: true},
	{Name: "enemy-base", Label: "Enemy bases", Planet: "nauvis", Category: "enemy", CanDisable: true},
	{Name: "nauvis_cliff", Label: "Cliffs", Planet: "nauvis", Category: "terrain", CanDisable: true},
}

var spaceAgeWorldControls = []WorldControlDefinition{
	{Name: "vulcanus_coal", Label: "Coal", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "tungsten_ore", Label: "Tungsten ore", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "calcite", Label: "Calcite", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "sulfuric_acid_geyser", Label: "Sulfuric acid geysers", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "vulcanus_volcanism", Label: "Volcanism", Planet: "vulcanus", Category: "terrain", CanDisable: false},
	{Name: "gleba_stone", Label: "Stone", Planet: "gleba", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "gleba_plants", Label: "Plants", Planet: "gleba", Category: "terrain", CanDisable: false},
	{Name: "gleba_water", Label: "Water", Planet: "gleba", Category: "terrain", CanDisable: false},
	{Name: "gleba_enemy_base", Label: "Pentapod nests", Planet: "gleba", Category: "enemy", CanDisable: false},
	{Name: "gleba_cliff", Label: "Cliffs", Planet: "gleba", Category: "terrain", CanDisable: true},
	{Name: "scrap", Label: "Scrap", Planet: "fulgora", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "fulgora_islands", Label: "Island size", Planet: "fulgora", Category: "terrain", CanDisable: false},
	{Name: "fulgora_cliff", Label: "Cliffs", Planet: "fulgora", Category: "terrain", CanDisable: true},
	{Name: "aquilo_crude_oil", Label: "Crude oil", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "lithium_brine", Label: "Lithium brine", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "fluorine_vent", Label: "Fluorine vents", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
}

func ValidateWorldGenerationRequest(request WorldGenerationRequest, preview bool) error {
	_, _, err := normalizeWorldGenerationRequest(request, preview)
	return err
}

func normalizeWorldGenerationRequest(request WorldGenerationRequest, preview bool) (WorldGenerationRequest, WorldGenerationOptions, error) {
	options, err := GetWorldGenerationOptions()
	if err != nil {
		return request, options, err
	}
	return normalizeWorldGenerationRequestWithOptions(request, preview, options)
}

func normalizeWorldGenerationRequestWithOptions(request WorldGenerationRequest, preview bool, options WorldGenerationOptions) (WorldGenerationRequest, WorldGenerationOptions, error) {
	request.Preset = strings.TrimSpace(strings.ToLower(request.Preset))
	if request.Preset == "" {
		request.Preset = "default"
	}
	validPreset := false
	for _, preset := range options.Presets {
		if preset.Name == request.Preset {
			validPreset = true
			break
		}
	}
	if !validPreset {
		return request, options, fmt.Errorf("unsupported map preset %q", request.Preset)
	}
	if request.Seed > uint64(^uint32(0)) {
		return request, options, errors.New("map seed must be between 0 and 4294967295")
	}
	if (request.Width != nil && *request.Width > maxWorldDimension) || (request.Height != nil && *request.Height > maxWorldDimension) {
		return request, options, fmt.Errorf("map dimensions must not exceed %d tiles", maxWorldDimension)
	}
	if request.StartingArea != nil && (*request.StartingArea < 0.1 || *request.StartingArea > 10) {
		return request, options, errors.New("starting area must be between 0.1 and 10")
	}

	request.Planet = strings.TrimSpace(strings.ToLower(request.Planet))
	if request.Planet == "" {
		request.Planet = "nauvis"
	}
	validPlanet := false
	for _, planet := range options.Planets {
		if planet.Name == request.Planet {
			validPlanet = true
			break
		}
	}
	if !validPlanet {
		return request, options, fmt.Errorf("planet %q is not available in the active game mode", request.Planet)
	}
	if preview {
		if request.PreviewSize == 0 {
			request.PreviewSize = defaultMapPreviewSize
		}
		if request.PreviewSize < 256 || request.PreviewSize > maxMapPreviewSize {
			return request, options, fmt.Errorf("preview size must be between 256 and %d pixels", maxMapPreviewSize)
		}
	}

	if !preview {
		request.Name = strings.TrimSpace(request.Name)
		if filepath.Ext(request.Name) == "" {
			request.Name += ".zip"
		}
		if err := ValidatePathElement(request.Name); err != nil || !strings.EqualFold(filepath.Ext(request.Name), ".zip") {
			return request, options, errors.New("save name must be a valid .zip filename")
		}
		request.Name = filepath.Base(request.Name)
	}

	definitions := make(map[string]WorldControlDefinition, len(options.Controls))
	for _, definition := range options.Controls {
		definitions[definition.Name] = definition
	}
	for name, settings := range request.Controls {
		definition, ok := definitions[name]
		if !ok || !worldIdentifier.MatchString(name) {
			return request, options, fmt.Errorf("map control %q is not available in the active game mode", name)
		}
		for field, value := range map[string]*float64{
			"frequency": settings.Frequency,
			"size":      settings.Size,
			"richness":  settings.Richness,
		} {
			if value == nil {
				continue
			}
			if *value < 0 || *value > 6 {
				return request, options, fmt.Errorf("%s for %s must be between 0 and 6", field, name)
			}
			if *value == 0 && !definition.CanDisable {
				return request, options, fmt.Errorf("%s cannot be disabled", definition.Label)
			}
			if field == "richness" && !definition.SupportsRichness {
				return request, options, fmt.Errorf("%s does not support richness", definition.Label)
			}
		}
	}
	return request, options, nil
}

func buildMapGenSettings(request WorldGenerationRequest, options WorldGenerationOptions) map[string]interface{} {
	settings := map[string]interface{}{
		"seed": request.Seed,
	}
	if request.Width != nil {
		settings["width"] = *request.Width
	}
	if request.Height != nil {
		settings["height"] = *request.Height
	}
	if request.StartingArea != nil {
		settings["starting_area"] = *request.StartingArea
	}
	if request.PeacefulMode != nil {
		settings["peaceful_mode"] = *request.PeacefulMode
	}
	if request.NoEnemiesMode != nil {
		settings["no_enemies_mode"] = *request.NoEnemiesMode
	}

	definitions := make(map[string]WorldControlDefinition, len(options.Controls))
	for _, definition := range options.Controls {
		definitions[definition.Name] = definition
	}
	autoplace := make(map[string]map[string]float64, len(request.Controls))
	for name, control := range request.Controls {
		definition := definitions[name]
		values := make(map[string]float64, 3)
		if control.Frequency != nil {
			values["frequency"] = *control.Frequency
		}
		if control.Size != nil {
			values["size"] = *control.Size
		}
		if definition.SupportsRichness && control.Richness != nil {
			values["richness"] = *control.Richness
		}
		if len(values) > 0 {
			autoplace[name] = values
		}
	}
	if len(autoplace) > 0 {
		settings["autoplace_controls"] = autoplace
	}
	return settings
}
