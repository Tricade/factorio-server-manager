package factorio

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const logTailLimit = 500

var secretFlagPattern = regexp.MustCompile(`(?i)((?:"?--(?:password|rcon-password|token|server-token|game-password|rcon-pass)"?)\s+)(?:"[^"]*"|'[^']*'|[^\s]+)`)
var secretAssignmentPattern = regexp.MustCompile(`(?i)((?:"?(?:password|token|secret|rcon[_-]?pass(?:word)?)"?)\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
var managerRCONConnectionPattern = regexp.MustCompile(`RemoteCommandProcessor\.cpp:\d+:\s+New RCON connection from IP ADDR:\(\{(?:127\.0\.0\.1|\[?::1\]?):\d+\}\)`)

func TailLog() ([]string, error) {
	return tailAndRedactLog(bootstrap.GetConfig().FactorioLog, logTailLimit)
}

func tailAndRedactLog(path string, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("open Factorio log: %w", err)
	}
	defer file.Close()

	ring := make([]string, limit)
	count := 0
	next := 0
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, readErr := readBoundedLogLine(reader, 1024*1024)
		if line != "" || readErr == nil {
			if !isManagerRCONConnectionLog(line) {
				ring[next] = redactLogLine(line)
				next = (next + 1) % limit
				count++
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read Factorio log: %w", readErr)
		}
	}

	resultCount := count
	if resultCount > limit {
		resultCount = limit
	}
	result := make([]string, 0, resultCount)
	start := 0
	if count >= limit {
		start = next
	}
	for index := 0; index < resultCount; index++ {
		result = append(result, ring[(start+index)%limit])
	}
	return result, nil
}

func isManagerRCONConnectionLog(line string) bool {
	return managerRCONConnectionPattern.MatchString(line)
}

func redactLogLine(line string) string {
	line = secretFlagPattern.ReplaceAllString(line, `${1}"[REDACTED]"`)
	return secretAssignmentPattern.ReplaceAllString(line, `${1}[REDACTED]`)
}
