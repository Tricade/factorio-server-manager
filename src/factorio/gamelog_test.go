package factorio

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRedactLogLine(t *testing.T) {
	line := `Program arguments: "factorio" "--password" "open-sesame" --token portal-token rcon_pass=console-secret settings={"password":"json-secret"}`
	redacted := redactLogLine(line)
	for _, secret := range []string{"open-sesame", "portal-token", "console-secret", "json-secret"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret %q was not redacted: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", redacted)
	}
}

func TestTailAndRedactLogReturnsNewestLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "factorio-current.log")
	contents := strings.Join([]string{"one", "two", "password=private", "four"}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	lines, err := tailAndRedactLog(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"two", "password=[REDACTED]", "four"}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("unexpected tail: %#v", lines)
	}
}

func TestTailAndRedactLogHidesManagerRCONConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "factorio-current.log")
	contents := strings.Join([]string{
		"one",
		"1368.540 Info RemoteCommandProcessor.cpp:236: New RCON connection from IP ADDR:({127.0.0.1:38166})",
		"two",
		"1383.544 Info RemoteCommandProcessor.cpp:236: New RCON connection from IP ADDR:({127.0.0.1:49130})",
		"three",
		"four",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	lines, err := tailAndRedactLog(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"two", "three", "four"}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("unexpected filtered tail: %#v", lines)
	}
}

func TestManagerRCONConnectionFilterKeepsRemoteConnections(t *testing.T) {
	remote := "1368.540 Info RemoteCommandProcessor.cpp:236: New RCON connection from IP ADDR:({192.0.2.10:38166})"
	if isManagerRCONConnectionLog(remote) {
		t.Fatalf("remote RCON connection must stay visible: %s", remote)
	}
}

func TestTailAndRedactLogReturnsEmptyForMissingFile(t *testing.T) {
	lines, err := tailAndRedactLog(filepath.Join(t.TempDir(), "not-created.log"), 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no lines for a log that does not exist yet, got %#v", lines)
	}
}
