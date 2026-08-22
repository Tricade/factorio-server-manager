package factorio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModDependencyKindsAndVersions(t *testing.T) {
	tests := []struct {
		raw      string
		kind     ModDependencyKind
		name     string
		operator string
		version  Version
	}{
		{"library >= 1.2.3", ModDependencyRequired, "library", ">=", Version{1, 2, 3, 0}},
		{"~ unordered-lib", ModDependencyRequired, "unordered-lib", "", NilVersion},
		{"? optional-mod", ModDependencyOptional, "optional-mod", "", NilVersion},
		{"(?) hidden-optional", ModDependencyOptional, "hidden-optional", "", NilVersion},
		{"+ recommended-mod", ModDependencyRecommended, "recommended-mod", "", NilVersion},
		{"! incompatible-mod", ModDependencyIncompatible, "incompatible-mod", "", NilVersion},
		{"exact = 2.0.4", ModDependencyRequired, "exact", "==", Version{2, 0, 4, 0}},
	}

	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			dependency, err := ParseModDependency(test.raw)
			require.NoError(t, err)
			assert.Equal(t, test.kind, dependency.Kind)
			assert.Equal(t, test.name, dependency.Name)
			assert.Equal(t, test.operator, dependency.Operator)
			assert.Equal(t, test.version, dependency.Version)
		})
	}

	_, err := ParseModDependency("? broken dependency with spaces")
	assert.Error(t, err)
}

func TestDependencyPlanResolvesRequiredAndLeavesOptionalUnchecked(t *testing.T) {
	responses := map[string]ModPortalStruct{
		"root-mod": portalFixture("root-mod", "Root Mod", "1.0.0", []string{
			"base >= 2.1", "required-lib >= 1.0", "? optional-addon", "+ recommended-addon", "! conflicting-mod",
		}),
		"required-lib":      portalFixture("required-lib", "Required Library", "1.2.0", nil),
		"optional-addon":    portalFixture("optional-addon", "Optional Addon", "3.0.0", []string{"nested-lib"}),
		"recommended-addon": portalFixture("recommended-addon", "Recommended Addon", "4.0.0", nil),
		"nested-lib":        portalFixture("nested-lib", "Nested Library", "2.0.0", nil),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/mods/"), "/full")
		fixture, ok := responses[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(fixture))
	}))
	defer server.Close()

	originalBaseURL := modPortalBaseURL
	originalServer := GetFactorioServer().Snapshot()
	modPortalBaseURL = server.URL
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}, BaseModVersion: "2.1.14"})
	t.Cleanup(func() {
		modPortalBaseURL = originalBaseURL
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})

	planner := dependencyPlannerFixture(ModInstallPlanRequest{Name: "root-mod", Version: Version{1, 0, 0, 0}})
	planner.installed["conflicting-mod"] = installedModState{Version: Version{1, 0, 0, 0}, Enabled: true}
	plan, err := planner.build()
	require.NoError(t, err)
	require.Len(t, plan.Required, 1)
	assert.Equal(t, "required-lib", plan.Required[0].Name)
	require.Len(t, plan.Optional, 2)
	assert.False(t, plan.Optional[0].Selected)
	assert.False(t, plan.Optional[1].Selected)
	require.Len(t, plan.Conflicts, 1)
	assert.Equal(t, "conflicting-mod", plan.Conflicts[0].Name)

	planner = dependencyPlannerFixture(ModInstallPlanRequest{
		Name: "root-mod", Version: Version{1, 0, 0, 0}, Optional: []string{"optional-addon"},
	})
	plan, err = planner.build()
	require.NoError(t, err)
	require.Len(t, plan.Optional, 2)
	var selected ModInstallPlanItem
	for _, item := range plan.Optional {
		if item.Name == "optional-addon" {
			selected = item
		}
	}
	assert.True(t, selected.Selected)
	requiredNames := []string{}
	for _, item := range plan.Required {
		requiredNames = append(requiredNames, item.Name)
	}
	assert.ElementsMatch(t, []string{"nested-lib", "required-lib"}, requiredNames)
}

func dependencyPlannerFixture(request ModInstallPlanRequest) *modDependencyPlanner {
	selected := make(map[string]bool)
	for _, name := range request.Optional {
		selected[name] = true
	}
	return &modDependencyPlanner{
		request: request, selected: selected, details: make(map[string]ModPortalStruct),
		installed: make(map[string]installedModState), constraints: make(map[string][]ParsedModDependency),
		nodes: make(map[string]*modPlanNode), optional: make(map[string]ModInstallPlanItem),
		processed: make(map[string]bool), conflicts: make(map[string]ModInstallConflict),
		modSimple: ModSimpleList{},
	}
}

func portalFixture(name, title, version string, dependencies []string) ModPortalStruct {
	var parsedVersion Version
	_ = parsedVersion.UnmarshalText([]byte(version))
	release := ModPortalRelease{Version: parsedVersion, Compatibility: true, FileName: name + "_" + version + ".zip", DownloadURL: "/download/" + name}
	release.InfoJSON.FactorioVersion = Version{2, 1, 0, 0}
	release.InfoJSON.Dependencies = dependencies
	return ModPortalStruct{Name: name, Title: title, Releases: []ModPortalRelease{release}}
}
