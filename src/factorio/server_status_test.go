package factorio

import "testing"

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
