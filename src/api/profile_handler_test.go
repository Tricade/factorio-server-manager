package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteProfileStateWithStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	state := factorio.ProfileState{
		SchemaVersion:   1,
		ActiveProfileID: "0123456789abcdef",
		Profiles: []factorio.Profile{{
			ID:     "0123456789abcdef",
			Name:   "Current setup",
			Active: true,
		}},
	}

	writeProfileStateWithStatus(recorder, state, http.StatusCreated)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, "application/json;charset=UTF-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var decoded factorio.ProfileState
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&decoded))
	assert.Equal(t, state.ActiveProfileID, decoded.ActiveProfileID)
	require.Len(t, decoded.Profiles, 1)
	assert.True(t, decoded.Profiles[0].Active)
}
