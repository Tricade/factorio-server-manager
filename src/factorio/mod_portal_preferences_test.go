package factorio

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModPortalPreferencesTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mod-portal-preferences.json")
	original := modPortalPreferencesPath
	modPortalPreferencesPath = func() string { return path }
	t.Cleanup(func() { modPortalPreferencesPath = original })
	return path
}

func TestModPortalPreferencesDefaultsAndPersistence(t *testing.T) {
	path := setupModPortalPreferencesTest(t)
	preferences, err := LoadModPortalPreferences()
	require.NoError(t, err)
	assert.False(t, preferences.PreselectOptional)
	assert.NoFileExists(t, path, "loading an existing installation must not migrate anything")
	for _, selected := range []bool{true, false} {
		_, err = SetModPortalPreferences(ModPortalPreferences{PreselectOptional: selected})
		require.NoError(t, err)
		preferences, err = LoadModPortalPreferences()
		require.NoError(t, err)
		assert.Equal(t, selected, preferences.PreselectOptional)
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestModPortalPreferencesInvalidFileAndFailedWrite(t *testing.T) {
	path := setupModPortalPreferencesTest(t)
	for _, contents := range []string{"", "{broken", strings.Repeat(" ", 4097), `{"preselect_optional":"true"}`} {
		require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
		_, err := LoadModPortalPreferences()
		require.Error(t, err)
	}
	_, err := SetModPortalPreferences(ModPortalPreferences{PreselectOptional: true})
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	original := atomicFileRename
	atomicFileRename = func(string, string) error { return errors.New("disk error") }
	t.Cleanup(func() { atomicFileRename = original })
	_, err = SetModPortalPreferences(ModPortalPreferences{})
	require.Error(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestModPortalOptionalDefaultsOnlyApplyToInitialReview(t *testing.T) {
	setupModPortalPreferencesTest(t)
	responses := map[string]ModPortalStruct{
		"root":            portalFixture("root", "Root", "1.0.0", []string{"required", "? optional", "+ recommended"}),
		"required":        portalFixture("required", "Library", "1.0.0", []string{"? required-addon"}),
		"optional":        portalFixture("optional", "Optional", "1.0.0", []string{"nested-required", "? nested-optional"}),
		"recommended":     portalFixture("recommended", "Recommended", "1.0.0", nil),
		"required-addon":  portalFixture("required-addon", "Required addon", "1.0.0", nil),
		"nested-required": portalFixture("nested-required", "Nested library", "1.0.0", nil),
		"nested-optional": portalFixture("nested-optional", "Another integration", "1.0.0", nil),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/mods/"), "/full")
		if response, ok := responses[name]; ok {
			_ = json.NewEncoder(w).Encode(response)
		} else {
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	originalURL, originalServer := modPortalBaseURL, GetFactorioServer().Snapshot()
	modPortalBaseURL = server.URL
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}, BaseModVersion: "2.1.14"})
	t.Cleanup(func() {
		modPortalBaseURL = originalURL
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})
	factory := func(request ModInstallPlanRequest) (*modDependencyPlanner, error) {
		return dependencyPlannerFixture(request), nil
	}
	request := ModInstallPlanRequest{Name: "root", Version: Version{1, 0, 0, 0}, UseOptionalDefaults: true}
	plan, err := planModInstallation(request, factory)
	require.NoError(t, err)
	for _, item := range plan.Optional {
		assert.False(t, item.Selected)
	}
	_, err = SetModPortalPreferences(ModPortalPreferences{PreselectOptional: true})
	require.NoError(t, err)
	plan, err = planModInstallation(request, factory)
	require.NoError(t, err)
	selected := []string{}
	for _, item := range plan.Optional {
		if item.Selected {
			selected = append(selected, item.Name)
		}
	}
	assert.ElementsMatch(t, []string{"optional", "recommended", "required-addon"}, selected)
	required := []string{}
	for _, item := range plan.Required {
		required = append(required, item.Name)
	}
	assert.Contains(t, required, "nested-required")
	request.UseOptionalDefaults = false
	plan, err = planModInstallation(request, factory)
	require.NoError(t, err)
	for _, item := range plan.Optional {
		assert.False(t, item.Selected, "explicitly unchecking everything must override defaults")
	}
	request.UseOptionalDefaults = true
	_, _, err = InstallModPlan(request)
	require.ErrorContains(t, err, "explicit optional selections")
	responses["optional"] = portalFixture("optional", "Broken extension", "1.0.0", []string{"missing-library"})
	plan, err = planModInstallation(request, factory)
	require.NoError(t, err)
	require.NotEmpty(t, plan.Warnings, "failed defaults must leave the base review available")
	for _, item := range plan.Optional {
		assert.False(t, item.Selected)
	}
	request.UseOptionalDefaults = false
	request.Optional = []string{"optional"}
	_, err = planModInstallation(request, factory)
	require.Error(t, err, "an explicitly selected broken extension must never partially install")
}
