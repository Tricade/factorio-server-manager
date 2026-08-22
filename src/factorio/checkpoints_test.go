package factorio

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type checkpointTestEnvironment struct {
	profile *profileTestEnvironment
	now     time.Time
}

func setupCheckpointTest(t *testing.T) *checkpointTestEnvironment {
	t.Helper()
	profileEnvironment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	environment := &checkpointTestEnvironment{
		profile: profileEnvironment,
		now:     time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
	}
	writeCheckpointTestZip(t, filepath.Join(profileEnvironment.active["saves"], "current.zip"), "initial world")

	originalRootPath := checkpointRootPath
	originalSavesPath := checkpointSavesPath
	originalNow := checkpointNow
	originalRunRCON := checkpointRunRCON
	originalCreateLiveSave := checkpointCreateLiveSave
	originalPollInterval := checkpointMonitorPollInterval
	originalCooldown := checkpointAutomatedCooldown
	checkpointRootPath = func() string { return filepath.Join(profileEnvironment.root, "checkpoints") }
	checkpointSavesPath = func() string { return profileEnvironment.active["saves"] }
	checkpointNow = func() time.Time { return environment.now }
	checkpointMonitorPollInterval = 15 * time.Second
	checkpointAutomatedCooldown = 2 * time.Minute

	t.Cleanup(func() {
		stopCheckpointMonitor()
		checkpointRootPath = originalRootPath
		checkpointSavesPath = originalSavesPath
		checkpointNow = originalNow
		checkpointRunRCON = originalRunRCON
		checkpointCreateLiveSave = originalCreateLiveSave
		checkpointMonitorPollInterval = originalPollInterval
		checkpointAutomatedCooldown = originalCooldown
	})
	return environment
}

func TestCheckpointSettingsPersistPerActiveProfile(t *testing.T) {
	environment := setupCheckpointTest(t)
	settings := CheckpointSettings{
		IntervalEnabled:   true,
		IntervalMinutes:   45,
		LastPlayerEnabled: true,
		CleanStopEnabled:  true,
		RetentionCount:    7,
	}
	state, err := UpdateCheckpointSettings(settings)
	require.NoError(t, err)
	assert.Equal(t, settings, state.Settings)
	assert.Empty(t, state.Checkpoints)

	state, err = GetCheckpointState()
	require.NoError(t, err)
	assert.Equal(t, settings, state.Settings)
	assert.FileExists(t, filepath.Join(environment.profile.root, "checkpoints", state.ProfileID, checkpointStateFile))

	_, err = UpdateCheckpointSettings(CheckpointSettings{IntervalMinutes: 1, RetentionCount: 10})
	assert.ErrorIs(t, err, ErrInvalidCheckpointSetting)
}

func TestCheckpointCreationRetentionAndRestoreAreImmutable(t *testing.T) {
	environment := setupCheckpointTest(t)
	state, err := UpdateCheckpointSettings(CheckpointSettings{IntervalMinutes: 30, RetentionCount: 2})
	require.NoError(t, err)
	profileID := state.ProfileID

	state, err = CreateCheckpoint(CheckpointTriggerManual)
	require.NoError(t, err)
	require.Len(t, state.Checkpoints, 1)
	oldest := state.Checkpoints[0]
	oldestPath := filepath.Join(checkpointFilesDirectory(profileID), oldest.FileName)
	assert.Equal(t, "initial world", readCheckpointTestZip(t, oldestPath))

	writeCheckpointTestZip(t, filepath.Join(environment.profile.active["saves"], "current.zip"), "second world")
	environment.now = environment.now.Add(time.Minute)
	_, err = CreateCheckpoint(CheckpointTriggerManual)
	require.NoError(t, err)
	assert.Equal(t, "initial world", readCheckpointTestZip(t, oldestPath), "creating a newer checkpoint must not rewrite the old archive")

	writeCheckpointTestZip(t, filepath.Join(environment.profile.active["saves"], "current.zip"), "third world")
	environment.now = environment.now.Add(time.Minute)
	state, err = CreateCheckpoint(CheckpointTriggerManual)
	require.NoError(t, err)
	require.Len(t, state.Checkpoints, 2)
	assert.NoFileExists(t, oldestPath, "retention removes the oldest archive only after the replacement exists")

	checkpointToRestore := state.Checkpoints[0]
	checkpointPath := filepath.Join(checkpointFilesDirectory(profileID), checkpointToRestore.FileName)
	originalCheckpointContents := readCheckpointTestZip(t, checkpointPath)
	restored, err := RestoreCheckpoint(checkpointToRestore.ID)
	require.NoError(t, err)
	assert.Contains(t, restored.Name, "restored-")
	assert.Equal(t, originalCheckpointContents, readCheckpointTestZip(t, filepath.Join(environment.profile.active["saves"], restored.Name)))
	assert.Equal(t, originalCheckpointContents, readCheckpointTestZip(t, checkpointPath), "restore must leave the fixed checkpoint untouched")

	state, err = DeleteCheckpoint(checkpointToRestore.ID)
	require.NoError(t, err)
	require.Len(t, state.Checkpoints, 1)
	assert.NotNil(t, state.LastCheckpointAt)
	state, err = DeleteCheckpoint(state.Checkpoints[0].ID)
	require.NoError(t, err)
	assert.Empty(t, state.Checkpoints)
	assert.Nil(t, state.LastCheckpointAt, "deleting the final checkpoint must not suppress the next automatic trigger")
}

func TestCheckpointsRemainBoundToTheirProfile(t *testing.T) {
	environment := setupCheckpointTest(t)
	firstState, err := CreateCheckpoint(CheckpointTriggerManual)
	require.NoError(t, err)
	require.Len(t, firstState.Checkpoints, 1)

	created, err := CreateProfile("Second setup", "", ProfileSourceClone)
	require.NoError(t, err)
	second := findProfileByName(t, created, "Second setup")
	_, err = ActivateProfile(second.ID)
	require.NoError(t, err)
	secondState, err := GetCheckpointState()
	require.NoError(t, err)
	assert.Equal(t, second.ID, secondState.ProfileID)
	assert.Empty(t, secondState.Checkpoints)

	_, err = ActivateProfile(firstState.ProfileID)
	require.NoError(t, err)
	restoredState, err := GetCheckpointState()
	require.NoError(t, err)
	require.Len(t, restoredState.Checkpoints, 1)
	assert.Equal(t, firstState.Checkpoints[0].ID, restoredState.Checkpoints[0].ID)
	assert.DirExists(t, filepath.Join(environment.profile.root, "checkpoints", firstState.ProfileID))
}

func TestCheckpointMonitorUsesConfirmedPlayerTransition(t *testing.T) {
	environment := setupCheckpointTest(t)
	_, err := UpdateCheckpointSettings(CheckpointSettings{
		IntervalMinutes:   30,
		LastPlayerEnabled: true,
		RetentionCount:    10,
	})
	require.NoError(t, err)
	checkpointMonitorPollInterval = 10 * time.Millisecond
	checkpointAutomatedCooldown = 0
	checkpointNow = time.Now

	var responseMutex sync.Mutex
	responses := []string{"1", "0", "0"}
	checkpointRunRCON = func(string) (string, error) {
		responseMutex.Lock()
		defer responseMutex.Unlock()
		if len(responses) == 0 {
			return "0", nil
		}
		response := responses[0]
		responses = responses[1:]
		return response, nil
	}
	checkpointCreateLiveSave = func(id string) (string, error) {
		path := filepath.Join(environment.profile.active["saves"], fmt.Sprintf("live-%s.zip", id))
		writeCheckpointTestZip(t, path, "live world")
		return path, nil
	}

	server := GetFactorioServer()
	server.SetRunning(true)
	startCheckpointMonitor(server)
	t.Cleanup(func() { server.SetRunning(false) })

	require.Eventually(t, func() bool {
		state, stateErr := GetCheckpointState()
		return stateErr == nil && len(state.Checkpoints) == 1 && state.Checkpoints[0].Trigger == CheckpointTriggerLastPlayer
	}, time.Second, 20*time.Millisecond)
	state, err := GetCheckpointState()
	require.NoError(t, err)
	require.Len(t, state.Checkpoints, 1, "repeated zero-player polls must not create duplicate checkpoints")
}

func TestCleanStopCheckpointHonorsProfileSetting(t *testing.T) {
	environment := setupCheckpointTest(t)
	_, err := UpdateCheckpointSettings(CheckpointSettings{
		IntervalMinutes:  30,
		CleanStopEnabled: true,
		RetentionCount:   10,
	})
	require.NoError(t, err)
	checkpointCreateLiveSave = func(id string) (string, error) {
		path := filepath.Join(environment.profile.active["saves"], fmt.Sprintf("clean-stop-%s.zip", id))
		writeCheckpointTestZip(t, path, "clean stop world")
		return path, nil
	}

	server := GetFactorioServer()
	server.SetRunning(true)
	runCleanStopCheckpoint()
	server.SetRunning(false)

	state, err := GetCheckpointState()
	require.NoError(t, err)
	require.Len(t, state.Checkpoints, 1)
	assert.Equal(t, CheckpointTriggerCleanStop, state.Checkpoints[0].Trigger)
}

func writeCheckpointTestZip(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	file, err := os.Create(path)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create("world/level.dat")
	require.NoError(t, err)
	_, err = entry.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
}

func readCheckpointTestZip(t *testing.T, path string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer reader.Close()
	require.NotEmpty(t, reader.File)
	entry, err := reader.File[0].Open()
	require.NoError(t, err)
	defer entry.Close()
	buffer, err := io.ReadAll(entry)
	require.NoError(t, err)
	return string(buffer)
}
