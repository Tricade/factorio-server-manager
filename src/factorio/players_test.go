package factorio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConnectedPlayerNames(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     []string
	}{
		{name: "none", response: "Online players (0):\n", want: []string{}},
		{name: "one", response: "Online players (1):\r\n  Ada (online)\r\n", want: []string{"Ada"}},
		{name: "sorted and spaced name", response: "\nOnline players (2):\n  Zed (online)\n  Alice Smith (online)\n", want: []string{"Alice Smith", "Zed"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			players, err := parseConnectedPlayerNames(test.response)
			require.NoError(t, err)
			assert.Equal(t, test.want, players)
		})
	}
}

func TestParseConnectedPlayerNamesRejectsAmbiguousOutput(t *testing.T) {
	responses := []string{
		"",
		"Online players (1):\n",
		"Online players (0):\n  Ada (online)\n",
		"Online players (2):\n  Ada (online)\n  Ada (online)\n",
		"Online players (1):\nAda (online)\n",
		"Unknown command: players\n",
		"Online players (1):\n  Ada (offline)\n",
	}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			_, err := parseConnectedPlayerNames(response)
			assert.Error(t, err)
		})
	}
}

func TestParseConnectedPlayerNamesDoesNotEchoRawRCONOutput(t *testing.T) {
	_, err := parseConnectedPlayerNames("secret console payload")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret console payload")
}

func TestStoppedPlayerOverviewDoesNotClaimLiveData(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	originalRoot := mapSnapshotRootPath
	mapSnapshotRootPath = func() string { return environment.root + "/map-snapshots" }
	t.Cleanup(func() { mapSnapshotRootPath = originalRoot })

	overview, err := GetPlayerOverview()
	require.NoError(t, err)
	assert.False(t, overview.ServerRunning)
	assert.False(t, overview.LiveAvailable)
	assert.Empty(t, overview.OnlinePlayers)
}
