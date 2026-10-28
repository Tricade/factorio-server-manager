package factorio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var checkpointMonitorMutex sync.Mutex
var checkpointMonitorCancel context.CancelFunc
var checkpointMonitorPollInterval = 15 * time.Second
var connectedPlayerCountPattern = regexp.MustCompile(`(?im)^[ \t]*online players[ \t]*\([ \t]*([0-9]+)[ \t]*\)[ \t]*:?[ \t]*\r?$`)

func runCleanStopCheckpoint() {
	state, err := GetCheckpointState()
	if err != nil {
		log.Printf("Unable to load clean-stop checkpoint settings: %v", err)
		return
	}
	if !state.Settings.CleanStopEnabled {
		return
	}
	if _, err := CreateCheckpoint(CheckpointTriggerCleanStop); err != nil && !errors.Is(err, ErrCheckpointCooldown) {
		log.Printf("Unable to create clean-stop checkpoint: %v", err)
	}
}

func startCheckpointMonitor(server *Server) {
	checkpointMonitorMutex.Lock()
	if checkpointMonitorCancel != nil {
		checkpointMonitorCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	checkpointMonitorCancel = cancel
	checkpointMonitorMutex.Unlock()
	go checkpointMonitorLoop(ctx, server)
}

func stopCheckpointMonitor() {
	checkpointMonitorMutex.Lock()
	if checkpointMonitorCancel != nil {
		checkpointMonitorCancel()
		checkpointMonitorCancel = nil
	}
	checkpointMonitorMutex.Unlock()
}

func checkpointMonitorLoop(ctx context.Context, server *Server) {
	startedAt := checkpointNow().UTC()
	var previousPlayers *int
	ticker := time.NewTicker(checkpointMonitorPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !server.GetRunning() || server.IsStopping() {
				return
			}
			unlock := LockProfileDataRead()
			state, err := GetCheckpointState()
			if err != nil {
				unlock()
				log.Printf("Unable to load checkpoint schedule: %v", err)
				continue
			}

			now := checkpointNow().UTC()
			lastIntervalBase := startedAt
			if state.LastCheckpointAt != nil && state.LastCheckpointAt.After(lastIntervalBase) {
				lastIntervalBase = *state.LastCheckpointAt
			}
			if state.Settings.IntervalEnabled && now.Sub(lastIntervalBase) >= time.Duration(state.Settings.IntervalMinutes)*time.Minute {
				if _, err := CreateCheckpoint(CheckpointTriggerInterval); err != nil && !errors.Is(err, ErrCheckpointCooldown) {
					log.Printf("Unable to create scheduled checkpoint: %v", err)
				}
			}

			if state.Settings.LastPlayerEnabled {
				players, playerErr := connectedPlayerCount()
				if playerErr != nil {
					log.Printf("Unable to read connected player count for checkpoints: %v", playerErr)
				} else {
					if previousPlayers != nil && *previousPlayers > 0 && players == 0 {
						if _, err := CreateCheckpoint(CheckpointTriggerLastPlayer); err != nil && !errors.Is(err, ErrCheckpointCooldown) {
							log.Printf("Unable to create last-player checkpoint: %v", err)
						}
					}
					previousPlayers = &players
				}
			} else {
				previousPlayers = nil
			}
			unlock()
		}
	}
}

func connectedPlayerCount() (int, error) {
	// Use Factorio's built-in multiplayer command. Lua console commands such as
	// /silent-command permanently disable achievements for the live save.
	response, err := checkpointRunRCON(`/players online count`)
	if err != nil {
		return 0, err
	}
	return parseConnectedPlayerCount(response)
}

func parseConnectedPlayerCount(response string) (int, error) {
	matches := connectedPlayerCountPattern.FindAllStringSubmatch(response, -1)
	if len(matches) != 1 {
		return 0, fmt.Errorf("unexpected player count %q", response)
	}
	players, err := strconv.Atoi(matches[0][1])
	if err != nil || players < 0 {
		return 0, fmt.Errorf("unexpected player count %q", response)
	}
	return players, nil
}
