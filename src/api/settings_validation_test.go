package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerSettingsNullBodyIsRejectedAsBadRequest(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/server/settings", strings.NewReader("null"))
	UpdateServerSettings(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
