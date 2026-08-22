package factorio

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModPortalListRequestsInstalledFactorioLine(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "max", r.URL.Query().Get("page_size"))
		assert.Equal(t, "2.1", r.URL.Query().Get("version"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pagination":{},"results":[]}`))
	}))
	defer portal.Close()

	originalBaseURL := modPortalBaseURL
	originalServer := GetFactorioServer().Snapshot()
	modPortalBaseURL = portal.URL
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}, BaseModVersion: "2.1.14"})
	t.Cleanup(func() {
		modPortalBaseURL = originalBaseURL
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})

	response, err, status := ModPortalList()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	payload, ok := response.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2.1", payload["factorio_version"])
}
