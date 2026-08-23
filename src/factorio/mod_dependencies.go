package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type ModDependencyKind string

const (
	ModDependencyRequired      ModDependencyKind = "required"
	ModDependencyOptional      ModDependencyKind = "optional"
	ModDependencyRecommended   ModDependencyKind = "recommended"
	ModDependencyIncompatible  ModDependencyKind = "incompatible"
	maximumModInstallPlanItems                   = 256
)

type ParsedModDependency struct {
	Raw      string
	Name     string
	Kind     ModDependencyKind
	Operator string
	Version  Version
}

type ModInstallPlanRequest struct {
	Name     string   `json:"name"`
	Version  Version  `json:"version"`
	Optional []string `json:"optional"`
}

type ModInstallPlanItem struct {
	Name       string            `json:"name"`
	Title      string            `json:"title"`
	Version    Version           `json:"version"`
	Kind       ModDependencyKind `json:"kind"`
	Constraint string            `json:"constraint,omitempty"`
	RequiredBy []string          `json:"required_by,omitempty"`
	BuiltIn    bool              `json:"built_in"`
	Installed  bool              `json:"installed"`
	Enabled    bool              `json:"enabled"`
	Selected   bool              `json:"selected"`
}

type ModInstallConflict struct {
	Name       string   `json:"name"`
	RequiredBy []string `json:"required_by"`
	Reason     string   `json:"reason"`
}

type ModInstallPlan struct {
	Root      ModInstallPlanItem   `json:"root"`
	Required  []ModInstallPlanItem `json:"required"`
	Optional  []ModInstallPlanItem `json:"optional"`
	Conflicts []ModInstallConflict `json:"conflicts"`
	Warnings  []string             `json:"warnings"`
}

type installedModState struct {
	Version Version
	Enabled bool
}

type builtInModInfo struct {
	Name         string   `json:"name"`
	Title        string   `json:"title"`
	Version      Version  `json:"version"`
	Dependencies []string `json:"dependencies"`
}

type modPlanNode struct {
	item         ModInstallPlanItem
	release      *ModPortalRelease
	dependencies []string
}

type pendingModConflict struct {
	dependency ParsedModDependency
	parent     string
}

type modDependencyPlanner struct {
	request      ModInstallPlanRequest
	selected     map[string]bool
	details      map[string]ModPortalStruct
	installed    map[string]installedModState
	constraints  map[string][]ParsedModDependency
	nodes        map[string]*modPlanNode
	optional     map[string]ModInstallPlanItem
	processed    map[string]bool
	conflicts    map[string]ModInstallConflict
	incompatible []pendingModConflict
	warnings     []string
	modSimple    ModSimpleList
}

var dependencyPattern = regexp.MustCompile(`^\s*(\(\?\)|[!?+~])?\s*([A-Za-z0-9_.-]+)(?:\s*(<=|>=|=|<|>)\s*([0-9]+(?:\.[0-9]+){0,3}))?\s*$`)

func ParseModDependency(raw string) (ParsedModDependency, error) {
	match := dependencyPattern.FindStringSubmatch(raw)
	if match == nil {
		return ParsedModDependency{}, fmt.Errorf("invalid Factorio dependency %q", raw)
	}

	dependency := ParsedModDependency{Raw: strings.TrimSpace(raw), Name: match[2], Kind: ModDependencyRequired}
	switch match[1] {
	case "!":
		dependency.Kind = ModDependencyIncompatible
	case "?", "(?)":
		dependency.Kind = ModDependencyOptional
	case "+":
		dependency.Kind = ModDependencyRecommended
	case "", "~":
		dependency.Kind = ModDependencyRequired
	}
	dependency.Operator = match[3]
	if dependency.Operator == "=" {
		dependency.Operator = "=="
	}
	if match[4] != "" {
		if err := dependency.Version.UnmarshalText([]byte(match[4])); err != nil {
			return ParsedModDependency{}, fmt.Errorf("invalid dependency version in %q: %w", raw, err)
		}
	}
	return dependency, nil
}

func PlanModInstallation(request ModInstallPlanRequest) (ModInstallPlan, error) {
	planner, err := newModDependencyPlanner(request)
	if err != nil {
		return ModInstallPlan{}, err
	}
	return planner.build()
}

func InstallModPlan(request ModInstallPlanRequest) (ModsResultList, ModInstallPlan, error) {
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

func (planner *modDependencyPlanner) build() (ModInstallPlan, error) {
	rootConstraint := ParsedModDependency{
		Raw:      planner.request.Name + " = " + planner.request.Version.String(),
		Name:     planner.request.Name,
		Kind:     ModDependencyRequired,
		Operator: "==",
		Version:  planner.request.Version,
	}
	if err := planner.resolveRequired(rootConstraint, "Selected mod", true); err != nil {
		return ModInstallPlan{}, err
	}
	for _, conflict := range planner.incompatible {
		planner.addConflictIfActive(conflict.dependency, conflict.parent)
	}

	rootNode := planner.nodes[planner.request.Name]
	if rootNode == nil {
		return ModInstallPlan{}, errors.New("root mod could not be resolved")
	}
	rootNode.item.Kind = ModDependencyRequired
	rootNode.item.Selected = true

	plan := ModInstallPlan{Root: rootNode.item, Warnings: append([]string{}, planner.warnings...)}
	for name, node := range planner.nodes {
		if name == planner.request.Name {
			continue
		}
		if optional, ok := planner.optional[name]; ok && optional.Selected {
			continue
		}
		plan.Required = append(plan.Required, node.item)
	}
	for _, item := range planner.optional {
		plan.Optional = append(plan.Optional, item)
	}
	for _, conflict := range planner.conflicts {
		plan.Conflicts = append(plan.Conflicts, conflict)
	}

	sort.Slice(plan.Required, func(i, j int) bool { return plan.Required[i].Name < plan.Required[j].Name })
	sort.Slice(plan.Optional, func(i, j int) bool { return plan.Optional[i].Name < plan.Optional[j].Name })
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].Name < plan.Conflicts[j].Name })
	return plan, nil
}

func (planner *modDependencyPlanner) resolveRequired(dependency ParsedModDependency, parent string, root bool) error {
	if dependency.Name == "base" || dependency.Name == "core" {
		return nil
	}
	planner.constraints[dependency.Name] = appendUniqueConstraint(planner.constraints[dependency.Name], dependency)

	if planner.nodes[dependency.Name] == nil && len(planner.nodes) >= maximumModInstallPlanItems {
		return fmt.Errorf("mod installation plan exceeds %d items", maximumModInstallPlanItems)
	}
	if info, found, err := planner.readBuiltInMod(dependency.Name); err != nil {
		return err
	} else if found {
		if !matchesAllConstraints(info.Version, planner.constraints[dependency.Name]) {
			return fmt.Errorf("built-in mod %s %s does not satisfy %s", info.Name, info.Version.String(), dependency.Raw)
		}
		node := planner.nodes[dependency.Name]
		if node == nil {
			title := info.Title
			if title == "" {
				title = info.Name
			}
			node = &modPlanNode{
				item: ModInstallPlanItem{
					Name: info.Name, Title: title, Version: info.Version, Kind: ModDependencyRequired,
					BuiltIn: true, Installed: true, Enabled: planner.modSimple.IsEnabled(info.Name), Selected: true,
				},
				dependencies: info.Dependencies,
			}
			planner.nodes[dependency.Name] = node
		}
		node.item.RequiredBy = appendUniqueString(node.item.RequiredBy, parent)
		node.item.Constraint = joinConstraints(planner.constraints[dependency.Name])
		delete(planner.optional, dependency.Name)
		return planner.processNode(node)
	}

	details, err := planner.getDetails(dependency.Name)
	if err != nil {
		return err
	}
	preferred := NilVersion
	if installed, ok := planner.installed[dependency.Name]; ok && !root {
		preferred = installed.Version
	}
	release, err := choosePortalRelease(details, planner.constraints[dependency.Name], preferred)
	if err != nil {
		return err
	}

	node := planner.nodes[dependency.Name]
	if node == nil || !node.item.Version.Equals(release.Version) {
		installed := planner.installed[dependency.Name]
		releaseCopy := release
		node = &modPlanNode{
			item: ModInstallPlanItem{
				Name: dependency.Name, Title: details.Title, Version: release.Version, Kind: ModDependencyRequired,
				Installed: installed.Version.Equals(release.Version), Enabled: installed.Enabled, Selected: true,
			},
			release:      &releaseCopy,
			dependencies: append([]string{}, release.InfoJSON.Dependencies...),
		}
		planner.nodes[dependency.Name] = node
	}
	node.item.RequiredBy = appendUniqueString(node.item.RequiredBy, parent)
	node.item.Constraint = joinConstraints(planner.constraints[dependency.Name])
	delete(planner.optional, dependency.Name)
	return planner.processNode(node)
}

func (planner *modDependencyPlanner) processNode(node *modPlanNode) error {
	key := node.item.Name + "@" + node.item.Version.String()
	if planner.processed[key] {
		return nil
	}
	planner.processed[key] = true

	for _, raw := range node.dependencies {
		dependency, err := ParseModDependency(raw)
		if err != nil {
			planner.warnings = append(planner.warnings, err.Error())
			continue
		}
		switch dependency.Kind {
		case ModDependencyIncompatible:
			planner.incompatible = append(planner.incompatible, pendingModConflict{dependency: dependency, parent: node.item.Name})
		case ModDependencyOptional, ModDependencyRecommended:
			if err := planner.resolveOptional(dependency, node.item.Name); err != nil {
				planner.warnings = append(planner.warnings, err.Error())
			}
		default:
			if err := planner.resolveRequired(dependency, node.item.Name, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (planner *modDependencyPlanner) resolveOptional(dependency ParsedModDependency, parent string) error {
	if dependency.Name == "base" || dependency.Name == "core" {
		return nil
	}
	if existing := planner.nodes[dependency.Name]; existing != nil {
		existing.item.RequiredBy = appendUniqueString(existing.item.RequiredBy, parent)
		return nil
	}

	if planner.selected[dependency.Name] {
		item, err := planner.describeOptional(dependency, parent)
		if err != nil {
			return err
		}
		item.Selected = true
		dependency.Kind = ModDependencyRequired
		if err := planner.resolveRequired(dependency, parent, false); err != nil {
			return err
		}
		planner.optional[dependency.Name] = item
		return nil
	}

	item, err := planner.describeOptional(dependency, parent)
	if err != nil {
		return err
	}
	if existing, ok := planner.optional[dependency.Name]; ok {
		existing.RequiredBy = appendUniqueString(existing.RequiredBy, parent)
		planner.optional[dependency.Name] = existing
	} else {
		planner.optional[dependency.Name] = item
	}
	return nil
}

func (planner *modDependencyPlanner) describeOptional(dependency ParsedModDependency, parent string) (ModInstallPlanItem, error) {
	if info, found, err := planner.readBuiltInMod(dependency.Name); err != nil {
		return ModInstallPlanItem{}, err
	} else if found {
		if !matchesAllConstraints(info.Version, []ParsedModDependency{dependency}) {
			return ModInstallPlanItem{}, fmt.Errorf("optional built-in mod %s does not satisfy %s", info.Name, dependency.Raw)
		}
		title := info.Title
		if title == "" {
			title = info.Name
		}
		return ModInstallPlanItem{
			Name: info.Name, Title: title, Version: info.Version, Kind: dependency.Kind,
			Constraint: dependency.Raw, RequiredBy: []string{parent}, BuiltIn: true,
			Installed: true, Enabled: planner.modSimple.IsEnabled(info.Name), Selected: false,
		}, nil
	}

	details, err := planner.getDetails(dependency.Name)
	if err != nil {
		return ModInstallPlanItem{}, err
	}
	preferred := NilVersion
	if installed, ok := planner.installed[dependency.Name]; ok {
		preferred = installed.Version
	}
	release, err := choosePortalRelease(details, []ParsedModDependency{dependency}, preferred)
	if err != nil {
		return ModInstallPlanItem{}, err
	}
	installed := planner.installed[dependency.Name]
	return ModInstallPlanItem{
		Name: dependency.Name, Title: details.Title, Version: release.Version, Kind: dependency.Kind,
		Constraint: dependency.Raw, RequiredBy: []string{parent}, Installed: installed.Version.Equals(release.Version),
		Enabled: installed.Enabled, Selected: false,
	}, nil
}

func (planner *modDependencyPlanner) addConflictIfActive(dependency ParsedModDependency, parent string) {
	active := false
	if installed, ok := planner.installed[dependency.Name]; ok && installed.Enabled {
		active = true
	}
	if node := planner.nodes[dependency.Name]; node != nil && (node.item.Selected || node.item.Enabled) {
		active = true
	}
	if info, found, _ := planner.readBuiltInMod(dependency.Name); found && planner.modSimple.IsEnabled(info.Name) {
		active = true
	}
	if !active {
		return
	}
	conflict := planner.conflicts[dependency.Name]
	conflict.Name = dependency.Name
	conflict.Reason = dependency.Raw
	conflict.RequiredBy = appendUniqueString(conflict.RequiredBy, parent)
	planner.conflicts[dependency.Name] = conflict
}

func (planner *modDependencyPlanner) getDetails(name string) (ModPortalStruct, error) {
	if details, ok := planner.details[name]; ok {
		return details, nil
	}
	details, err, status := ModPortalModDetails(name)
	if err != nil {
		return ModPortalStruct{}, fmt.Errorf("load dependency %s from mod portal: %w", name, err)
	}
	if status < 200 || status >= 300 {
		return ModPortalStruct{}, fmt.Errorf("load dependency %s from mod portal: HTTP %d", name, status)
	}
	planner.details[name] = details
	return details, nil
}

func (planner *modDependencyPlanner) readBuiltInMod(name string) (builtInModInfo, bool, error) {
	path := filepath.Join(bootstrap.GetConfig().FactorioDir, "data", name, "info.json")
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return builtInModInfo{}, false, nil
	}
	if err != nil {
		return builtInModInfo{}, false, fmt.Errorf("read built-in mod %s: %w", name, err)
	}
	var info builtInModInfo
	if err := json.Unmarshal(contents, &info); err != nil {
		return builtInModInfo{}, false, fmt.Errorf("decode built-in mod %s: %w", name, err)
	}
	if info.Name == "" {
		info.Name = name
	}
	return info, true, nil
}

func choosePortalRelease(details ModPortalStruct, constraints []ParsedModDependency, preferred Version) (ModPortalRelease, error) {
	if !preferred.Equals(NilVersion) {
		for _, release := range details.Releases {
			if release.Version.Equals(preferred) && release.Compatibility && matchesAllConstraints(release.Version, constraints) {
				return release, nil
			}
		}
	}

	var selected *ModPortalRelease
	for _, release := range details.Releases {
		if !release.Compatibility || !matchesAllConstraints(release.Version, constraints) {
			continue
		}
		if selected == nil || release.Version.Greater(selected.Version) {
			releaseCopy := release
			selected = &releaseCopy
		}
	}
	if selected == nil {
		return ModPortalRelease{}, fmt.Errorf("no compatible release of %s satisfies %s", details.Name, joinConstraints(constraints))
	}
	return *selected, nil
}

func matchesAllConstraints(version Version, constraints []ParsedModDependency) bool {
	for _, dependency := range constraints {
		if dependency.Operator == "" || dependency.Version.Equals(NilVersion) {
			continue
		}
		if !version.Compatible(dependency.Version, dependency.Operator) {
			return false
		}
	}
	return true
}

func appendUniqueConstraint(constraints []ParsedModDependency, candidate ParsedModDependency) []ParsedModDependency {
	for _, existing := range constraints {
		if existing.Raw == candidate.Raw {
			return constraints
		}
	}
	return append(constraints, candidate)
}

func appendUniqueString(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func joinConstraints(constraints []ParsedModDependency) string {
	values := make([]string, 0, len(constraints))
	for _, dependency := range constraints {
		values = append(values, dependency.Raw)
	}
	return strings.Join(values, ", ")
}
