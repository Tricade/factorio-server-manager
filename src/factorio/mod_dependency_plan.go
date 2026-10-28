package factorio

import (
	"errors"
	"fmt"
	"sort"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

func PlanModInstallation(request ModInstallPlanRequest) (ModInstallPlan, error) {
	return planModInstallation(request, newModDependencyPlanner)
}

func planModInstallation(request ModInstallPlanRequest, createPlanner func(ModInstallPlanRequest) (*modDependencyPlanner, error)) (ModInstallPlan, error) {
	planner, err := createPlanner(request)
	if err != nil {
		return ModInstallPlan{}, err
	}
	plan, err := planner.build()
	if err != nil || !request.UseOptionalDefaults {
		return plan, err
	}
	preferences, err := LoadModPortalPreferences()
	if err != nil {
		return plan, errors.New("unable to load mod portal preferences")
	}
	if !preferences.PreselectOptional {
		return plan, nil
	}
	// Preselect the optional/recommended entries in the initial review, not
	// every compatibility integration reachable through other optional mods.
	// The next plan resolves their required dependencies as usual.
	for _, item := range plan.Optional {
		request.Optional = appendUniqueString(request.Optional, item.Name)
	}
	planner, err = createPlanner(request)
	if err != nil {
		return plan, err
	}
	selectedPlan, err := planner.build()
	if err != nil {
		// A broken optional extension must not hide the review for the root mod.
		// Keep the explicit initial selection and let the user select individually.
		plan.Warnings = append(plan.Warnings, "Optional defaults could not be resolved together. Review and select optional dependencies individually.")
		return plan, nil
	}
	return selectedPlan, nil
}

func InstallModPlan(request ModInstallPlanRequest) (ModsResultList, ModInstallPlan, error) {
	if request.UseOptionalDefaults {
		return ModsResultList{}, ModInstallPlan{}, errors.New("review dependencies and submit explicit optional selections before installing")
	}
	planner, err := newModDependencyPlanner(request)
	if err != nil {
		return ModsResultList{}, ModInstallPlan{}, err
	}
	plan, err := planner.build()
	if err != nil {
		return ModsResultList{}, plan, err
	}
	if len(plan.Conflicts) > 0 {
		return ModsResultList{}, plan, errors.New("selected mod set conflicts with an enabled mod")
	}

	config := bootstrap.GetConfig()
	mods, err := NewMods(config.FactorioModsDir)
	if err != nil {
		return ModsResultList{}, plan, err
	}

	installNames := make([]string, 0, len(planner.nodes))
	for name := range planner.nodes {
		installNames = append(installNames, name)
	}
	sort.Strings(installNames)
	for _, name := range installNames {
		node := planner.nodes[name]
		item := node.item
		if item.BuiltIn {
			if err := mods.ModSimpleList.SetModEnabled(item.Name, true); err != nil {
				return ModsResultList{}, plan, fmt.Errorf("enable built-in mod %s: %w", item.Name, err)
			}
			continue
		}

		if !item.Installed {
			if node.release == nil {
				return ModsResultList{}, plan, fmt.Errorf("release metadata missing for %s", item.Name)
			}
			if err := mods.DownloadMod(node.release.DownloadURL, node.release.FileName, item.Name); err != nil {
				return ModsResultList{}, plan, fmt.Errorf("download %s %s: %w", item.Name, item.Version.String(), err)
			}
		}
		if err := mods.ModSimpleList.SetModEnabled(item.Name, true); err != nil {
			return ModsResultList{}, plan, fmt.Errorf("enable mod %s: %w", item.Name, err)
		}
	}

	mods, err = NewMods(config.FactorioModsDir)
	if err != nil {
		return ModsResultList{}, plan, err
	}
	return mods.ListInstalledMods(), plan, nil
}

func newModDependencyPlanner(request ModInstallPlanRequest) (*modDependencyPlanner, error) {
	if err := ValidatePathElement(request.Name); err != nil {
		return nil, fmt.Errorf("invalid root mod name: %w", err)
	}
	if request.Version.Equals(NilVersion) {
		return nil, errors.New("root mod version is required")
	}
	if len(request.Optional) > maximumModInstallPlanItems {
		return nil, fmt.Errorf("at most %d optional mods may be selected", maximumModInstallPlanItems)
	}

	config := bootstrap.GetConfig()
	mods, err := NewMods(config.FactorioModsDir)
	if err != nil {
		return nil, err
	}
	installed := make(map[string]installedModState)
	for _, info := range mods.ModInfoList.Mods {
		var version Version
		if err := version.UnmarshalText([]byte(info.Version)); err != nil {
			continue
		}
		installed[info.Name] = installedModState{Version: version, Enabled: mods.ModSimpleList.IsEnabled(info.Name)}
	}

	selected := make(map[string]bool, len(request.Optional))
	for _, name := range request.Optional {
		if err := ValidatePathElement(name); err != nil {
			return nil, fmt.Errorf("invalid optional mod name: %w", err)
		}
		selected[name] = true
	}

	return &modDependencyPlanner{
		request:     request,
		selected:    selected,
		details:     make(map[string]ModPortalStruct),
		installed:   installed,
		constraints: make(map[string][]ParsedModDependency),
		nodes:       make(map[string]*modPlanNode),
		optional:    make(map[string]ModInstallPlanItem),
		processed:   make(map[string]bool),
		conflicts:   make(map[string]ModInstallConflict),
		modSimple:   mods.ModSimpleList,
	}, nil
}
