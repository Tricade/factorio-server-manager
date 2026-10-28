package factorio

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type auditReleaseTransport func(*http.Request) (*http.Response, error)

func (transport auditReleaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestReleaseInstallRejectsStartingRuntimeBeforeNetworkAccess(t *testing.T) {
	configureSettingsFixture(t, `{"name":"isolated"}`)
	SetFactorioServer(Server{starting: true})
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	called := false
	http.DefaultTransport = auditReleaseTransport(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("test must not contact the release service")
	})
	require.ErrorIs(t, installRelease("latest", "latest", nil), ErrServerActive)
	require.False(t, called)
}

func TestReleaseInstallReservationRejectsConcurrentStart(t *testing.T) {
	configureSettingsFixture(t, `{"name":"isolated"}`)
	entered, release := make(chan struct{}), make(chan struct{})
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = auditReleaseTransport(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return nil, errors.New("injected download failure")
	})
	result := make(chan error, 1)
	go func() { result <- installRelease("latest", "latest", nil) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("release install did not reach the isolated transport")
	}
	startResult, err := GetFactorioServer().Start("127.0.0.1", 34197, "missing.zip")
	close(release)
	require.Error(t, <-result)
	if startResult != nil {
		<-startResult // Let a buggy admitted startup release its guards before failing.
	}
	require.ErrorIs(t, err, ErrWorldGenerationBusy)
	require.False(t, GetFactorioServer().IsBusy())
	require.True(t, worldGenerationLock.TryLock(), "failed download leaked the shared engine guard")
	worldGenerationLock.Unlock()
}
