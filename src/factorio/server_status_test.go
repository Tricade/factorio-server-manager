package factorio

import (
	"errors"
	"testing"
	"time"

	"github.com/OpenFactorioServerManager/rcon"
)

func TestServerSnapshotAndStartConfiguration(t *testing.T) {
	server := &Server{
		Running:        false,
		Stopping:       true,
		Version:        Version{2, 1, 14},
		BaseModVersion: "2.1.14",
		startupReady:   true,
		stopSignaled:   true,
	}
	server.ConfigureStart("0.0.0.0", 34197, "factory.zip")

	snapshot := server.Snapshot()
	if snapshot.BindIP != "0.0.0.0" || snapshot.Port != 34197 || snapshot.Savefile != "factory.zip" {
		t.Fatalf("unexpected start configuration: %#v", snapshot)
	}
	if snapshot.Stopping {
		t.Fatal("new start configuration must clear stale stopping state")
	}
	if server.startupReady || server.stopSignaled {
		t.Fatal("new start configuration must clear stale startup and shutdown state")
	}
	if !snapshot.Version.Equals(Version{2, 1, 14}) || snapshot.BaseModVersion != "2.1.14" {
		t.Fatalf("version information missing from snapshot: %#v", snapshot)
	}
}

func TestShutdownWaitsForStartupReadiness(t *testing.T) {
	server := &Server{Running: true, Stopping: true}

	if server.claimStopSignalIfReady() {
		t.Fatal("shutdown signal must not be claimed before Factorio is ready")
	}
	if !server.markStartupReady() {
		t.Fatal("the readiness transition must claim a pending shutdown signal")
	}
	if server.claimStopSignalIfReady() || server.markStartupReady() {
		t.Fatal("a shutdown signal must only be claimed once")
	}
}

func TestShutdownClaimsSignalWhenAlreadyReady(t *testing.T) {
	server := &Server{Running: true}

	if server.markStartupReady() {
		t.Fatal("readiness without a pending shutdown must not claim a signal")
	}
	server.Stopping = true
	if !server.claimStopSignalIfReady() {
		t.Fatal("shutdown must claim a signal once Factorio is ready")
	}
}

func TestReadinessRecordedBeforeRunningStateIsPublished(t *testing.T) {
	server := &Server{}

	if server.markStartupReady() {
		t.Fatal("readiness alone must not claim a shutdown signal")
	}
	server.Running = true
	server.Stopping = true
	if !server.claimStopSignalIfReady() {
		t.Fatal("early readiness marker must be available to a later stop request")
	}
}

func TestPendingStartupStopClaimsOnlyAfterRCONIsPublished(t *testing.T) {
	server := &Server{Running: true, Stopping: true}
	console := &rcon.RemoteConsole{}
	pendingStop, installed := server.installStartupRCON(console)
	if !installed || !pendingStop {
		t.Fatalf("pending stop was not claimed after RCON installation: installed=%v pending=%v", installed, pendingStop)
	}
	if server.getRcon() != console {
		t.Fatal("startup stop became ready before its RCON connection was visible")
	}
	if server.claimStopSignalIfReady() {
		t.Fatal("pending startup stop was claimed more than once")
	}
	stateMutex.Lock()
	server.Rcon = nil
	stateMutex.Unlock()
}

func TestParallelStartKeepsWinningRequestSnapshotAndRejectsSecondClaim(t *testing.T) {
	server := &Server{Settings: map[string]interface{}{"name": "atomic-start"}, Version: Version{2, 1, 14}}
	claimed := make(chan serverStartSnapshot, 1)
	release := make(chan struct{})
	originalHook := serverBeforeProcessStart
	serverBeforeProcessStart = func(snapshot serverStartSnapshot) {
		claimed <- snapshot
		<-release
	}
	t.Cleanup(func() { serverBeforeProcessStart = originalHook })

	firstResult, err := server.Start("127.0.0.1", 34197, "first.zip")
	if err != nil {
		t.Fatalf("first start claim failed: %v", err)
	}

	var firstSnapshot serverStartSnapshot
	select {
	case firstSnapshot = <-claimed:
	case <-time.After(2 * time.Second):
		t.Fatal("first start did not reach the startup barrier")
	}
	if firstSnapshot.BindIP != "127.0.0.1" || firstSnapshot.Port != 34197 || firstSnapshot.Savefile != "first.zip" {
		t.Fatalf("first request was not captured intact: %#v", firstSnapshot)
	}

	secondResult, secondErr := server.Start("0.0.0.0", 34200, "second.zip")
	if !errors.Is(secondErr, ErrServerActive) {
		t.Fatalf("second start must be rejected while the first is claimed, got result=%v error=%v", secondResult, secondErr)
	}
	if firstSnapshot.BindIP != "127.0.0.1" || firstSnapshot.Port != 34197 || firstSnapshot.Savefile != "first.zip" {
		t.Fatalf("rejected request changed the winning snapshot: %#v", firstSnapshot)
	}

	close(release)
	select {
	case <-firstResult:
	case <-time.After(2 * time.Second):
		t.Fatal("first start did not release its reservation")
	}
}
