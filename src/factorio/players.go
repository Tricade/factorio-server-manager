package factorio

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var connectedPlayerLinePattern = regexp.MustCompile(`^[ \t]+(.+?)[ \t]+\([ \t]*online[ \t]*\)[ \t]*$`)
var playerOverviewRunRCON = runCheckpointRCONCommand

type PlayerOverviewPlayer struct {
	MapSnapshotPlayer
	Online bool `json:"online"`
}

// PlayerOverview keeps live and save-derived data explicitly separated. The
// current names come from Factorio's built-in /players command. Playtime comes
// from the latest isolated map snapshot and can therefore be older than the
// live connection state.
type PlayerOverview struct {
	ProfileID          string                 `json:"profile_id"`
	ServerRunning      bool                   `json:"server_running"`
	LiveAvailable      bool                   `json:"live_available"`
	LiveError          string                 `json:"live_error,omitempty"`
	OnlineCount        int                    `json:"online_count"`
	OnlinePlayers      []string               `json:"online_players"`
	StatisticsAsOf     *time.Time             `json:"statistics_as_of,omitempty"`
	StatisticsSaveName string                 `json:"statistics_save_name,omitempty"`
	StatisticsGameTick uint64                 `json:"statistics_game_tick,omitempty"`
	Players            []PlayerOverviewPlayer `json:"players"`
}

func GetPlayerOverview() (PlayerOverview, error) {
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return PlayerOverview{}, err
	}
	server := GetFactorioServer()
	status := server.Snapshot()
	overview := PlayerOverview{
		ProfileID:     profile.ID,
		ServerRunning: status.Running,
		LiveAvailable: false,
		OnlinePlayers: []string{},
		Players:       []PlayerOverviewPlayer{},
	}

	if status.Running {
		if !server.isStartupReady() {
			overview.LiveError = "Factorio RCON is not ready"
		} else {
			response, liveErr := playerOverviewRunRCON("/players online")
			if liveErr != nil {
				overview.LiveError = liveErr.Error()
			} else {
				names, parseErr := parseConnectedPlayerNames(response)
				if parseErr != nil {
					overview.LiveError = "Factorio returned an unrecognized player list"
				} else {
					overview.LiveAvailable = true
					overview.OnlinePlayers = names
					overview.OnlineCount = len(names)
				}
			}
		}
	}

	snapshot, err := loadMapSnapshot(profile.ID)
	if errors.Is(err, os.ErrNotExist) {
		return overview, nil
	}
	if err != nil {
		return PlayerOverview{}, err
	}
	generatedAt := snapshot.GeneratedAt
	overview.StatisticsAsOf = &generatedAt
	overview.StatisticsSaveName = snapshot.SaveName
	overview.StatisticsGameTick = snapshot.GameTick
	online := make(map[string]bool, len(overview.OnlinePlayers))
	for _, name := range overview.OnlinePlayers {
		online[name] = true
	}
	known := make(map[string]bool, len(snapshot.Players))
	for _, player := range snapshot.Players {
		overview.Players = append(overview.Players, PlayerOverviewPlayer{MapSnapshotPlayer: player, Online: online[player.Name]})
		known[player.Name] = true
	}
	for _, name := range overview.OnlinePlayers {
		if !known[name] {
			overview.Players = append(overview.Players, PlayerOverviewPlayer{MapSnapshotPlayer: MapSnapshotPlayer{Name: name}, Online: true})
		}
	}
	return overview, nil
}

func parseConnectedPlayerNames(response string) ([]string, error) {
	lines := strings.Split(strings.ReplaceAll(response, "\r\n", "\n"), "\n")
	headerCount := -1
	names := make([]string, 0)
	seen := make(map[string]bool)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if match := connectedPlayerCountPattern.FindStringSubmatch(line); match != nil {
			if headerCount >= 0 {
				return nil, errors.New("unexpected online player list response")
			}
			count, err := strconv.Atoi(match[1])
			if err != nil || count < 0 {
				return nil, errors.New("unexpected online player list response")
			}
			headerCount = count
			continue
		}
		match := connectedPlayerLinePattern.FindStringSubmatch(line)
		if match == nil {
			return nil, errors.New("unexpected online player list response")
		}
		name := strings.TrimSpace(match[1])
		if name == "" || utf8.RuneCountInString(name) > 200 || strings.IndexFunc(name, unicode.IsControl) >= 0 || seen[name] {
			return nil, errors.New("unexpected online player list response")
		}
		seen[name] = true
		names = append(names, name)
	}
	if headerCount < 0 || headerCount != len(names) {
		return nil, errors.New("unexpected online player list response")
	}
	sort.SliceStable(names, func(left, right int) bool { return strings.ToLower(names[left]) < strings.ToLower(names[right]) })
	return names, nil
}
