package factorio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessLogReaderSkipsOversizedLineAndContinues(t *testing.T) {
	config := configureSettingsFixture(t, `{"name":"isolated"}`)
	originalPublisher := publishProcessLog
	var published []string
	publishProcessLog = func(line string) { published = append(published, line) }
	t.Cleanup(func() { publishProcessLog = originalPublisher })
	input := strings.Repeat("x", 2<<20) + "\n0.2 Info continuation\n"
	require.NoError(t, GetFactorioServer().parseRunningCommand(io.NopCloser(strings.NewReader(input))))
	contents, err := os.ReadFile(config.ConsoleLogFile)
	require.NoError(t, err)
	require.Contains(t, string(contents), "continuation")
	require.Less(t, len(contents), 1024, "an oversized record must not enter manager logs")
	require.Equal(t, []string{omittedLogLine, "0.2 Info continuation"}, published)
}

func TestTailLogSkipsOversizedLineWithoutLosingLaterRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.log")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", 2<<20)+"\ncontinuation"), 0600))
	lines, err := tailAndRedactLog(path, 2)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	require.Equal(t, "continuation", lines[1])
}
