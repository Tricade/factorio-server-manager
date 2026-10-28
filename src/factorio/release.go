package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReleaseChannel identifies one of the official rolling Factorio headless releases.
type ReleaseChannel string

const (
	ReleaseChannelStable ReleaseChannel = "stable"
	ReleaseChannelLatest ReleaseChannel = "latest"

	maximumFactorioReleaseArchiveBytes int64 = 2 * 1024 * 1024 * 1024
	officialFactorioDownloadHost             = "www.factorio.com"
	officialFactorioCDNHost                  = "dl.factorio.com"
)

var releaseDownloadURLs = map[ReleaseChannel]string{
	ReleaseChannelStable: "https://www.factorio.com/get-download/stable/headless/linux64",
	ReleaseChannelLatest: "https://www.factorio.com/get-download/latest/headless/linux64",
}

var releaseInstallLock sync.Mutex

var factorioProgramFilesGate sync.RWMutex

var releaseRename = os.Rename

var releaseChannelMetadataURL = "https://factorio.com/api/latest-releases"

var releaseVersionMetadataURL = "https://updater.factorio.com/get-available-versions"

var releaseMetadataHTTPClient = &http.Client{Timeout: 15 * time.Second}

var exactReleasePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:\.0)?$`)

var releaseDataDirectories = map[string]bool{
	"saves":  true,
	"mods":   true,
	"config": true,
}

type ReleaseStatus struct {
	InstalledVersion  string   `json:"installed_version"`
	InstalledChannel  string   `json:"installed_channel"`
	InstalledTarget   string   `json:"installed_target,omitempty"`
	StableVersion     string   `json:"stable_version,omitempty"`
	LatestVersion     string   `json:"latest_version,omitempty"`
	AvailableVersions []string `json:"available_versions"`
	MetadataError     string   `json:"metadata_error,omitempty"`
}

type officialReleaseResponse struct {
	Updates []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"core-linux_headless64"`
}

type officialReleaseChannels struct {
	Experimental json.RawMessage `json:"experimental"`
	Stable       json.RawMessage `json:"stable"`
}

// NormalizeExactReleaseVersion validates and canonicalizes an exact public
// Factorio version. A trailing build component of .0 is accepted because the
// manager exposes Factorio's internal four-component version elsewhere.
func NormalizeExactReleaseVersion(version string) (string, error) {
	version = strings.TrimSpace(version)
	if !exactReleasePattern.MatchString(version) {
		return "", fmt.Errorf("unsupported Factorio release target %q; use stable, latest or an exact version such as 2.1.14", version)
	}
	version = strings.TrimSuffix(version, ".0")
	var parsed Version
	if err := parsed.UnmarshalText([]byte(version)); err != nil {
		return "", fmt.Errorf("invalid Factorio release version %q: %w", version, err)
	}
	return parsed.ReleaseString(), nil
}

// NormalizeReleaseTarget accepts the two official rolling channels and exact
// version identifiers. It never accepts a URL supplied by the browser.
func NormalizeReleaseTarget(target string) (string, error) {
	target = strings.TrimSpace(strings.ToLower(target))
	if _, ok := releaseDownloadURLs[ReleaseChannel(target)]; ok {
		return target, nil
	}
	return NormalizeExactReleaseVersion(target)
}

// ReleaseDownloadURL returns an official Factorio download URL for a supported
// rolling channel or an exact, strictly validated version.
func ReleaseDownloadURL(target string) (string, error) {
	downloadURL, err := officialReleaseDownloadURL(target)
	if err != nil {
		return "", err
	}
	return downloadURL.String(), nil
}

// officialReleaseDownloadURL keeps the request authority independent of the
// selected release. The validated target can affect only one URL path segment.
func officialReleaseDownloadURL(target string) (*url.URL, error) {
	normalized, err := NormalizeReleaseTarget(target)
	if err != nil {
		return nil, err
	}
	downloadURL := &url.URL{
		Scheme: "https",
		Host:   officialFactorioDownloadHost,
		Path:   "/get-download/" + normalized + "/headless/linux64",
	}
	if err := validateOfficialFactorioDownloadURL(downloadURL); err != nil {
		return nil, err
	}
	return downloadURL, nil
}

func validateOfficialFactorioDownloadURL(downloadURL *url.URL) error {
	if downloadURL == nil || !strings.EqualFold(downloadURL.Scheme, "https") || downloadURL.User != nil || downloadURL.Port() != "" {
		return errors.New("the Factorio release download must use HTTPS without credentials or a custom port")
	}
	host := strings.ToLower(downloadURL.Hostname())
	path := downloadURL.EscapedPath()
	switch host {
	case officialFactorioDownloadHost, "factorio.com":
		if !strings.HasPrefix(path, "/get-download/") {
			return errors.New("the Factorio release download URL has an unexpected path")
		}
	case officialFactorioCDNHost:
		if !strings.HasPrefix(path, "/releases/") {
			return errors.New("the Factorio release CDN URL has an unexpected path")
		}
	default:
		return fmt.Errorf("the Factorio release download redirected to untrusted host %q", host)
	}
	return nil
}

func newFactorioReleaseHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many Factorio release download redirects")
			}
			return validateOfficialFactorioDownloadURL(request.URL)
		},
	}
}

func parseOfficialRelease(raw json.RawMessage) (string, error) {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil && direct != "" {
		return NormalizeExactReleaseVersion(direct)
	}
	var branch struct {
		Headless string `json:"headless"`
	}
	if err := json.Unmarshal(raw, &branch); err != nil {
		return "", err
	}
	return NormalizeExactReleaseVersion(branch.Headless)
}

func sortedAvailableVersions(official officialReleaseResponse, stable, latest string) []string {
	unique := map[string]bool{}
	for _, candidate := range []string{stable, latest} {
		if candidate != "" {
			unique[candidate] = true
		}
	}
	for _, update := range official.Updates {
		for _, candidate := range []string{update.From, update.To} {
			if normalized, err := NormalizeExactReleaseVersion(candidate); err == nil {
				unique[normalized] = true
			}
		}
	}
	versions := make([]string, 0, len(unique))
	for candidate := range unique {
		versions = append(versions, candidate)
	}
	sort.Slice(versions, func(i, j int) bool {
		var left, right Version
		_ = left.UnmarshalText([]byte(versions[i]))
		_ = right.UnmarshalText([]byte(versions[j]))
		return left.Greater(right)
	})
	return versions
}

func versionsEqual(left, right string) bool {
	var parsedLeft, parsedRight Version
	if parsedLeft.UnmarshalText([]byte(left)) != nil || parsedRight.UnmarshalText([]byte(right)) != nil {
		return false
	}
	return parsedLeft.Equals(parsedRight)
}

func decodeOfficialMetadata(metadataURL string, target interface{}) error {
	response, err := releaseMetadataHTTPClient.Get(metadataURL)
	if err != nil {
		return fmt.Errorf("load %s: %w", metadataURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("load %s: unexpected status %s", metadataURL, response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", metadataURL, err)
	}
	return nil
}

// SupportedReleaseChannels returns channels that may be installed through the manager.
func SupportedReleaseChannels() []ReleaseChannel {
	return []ReleaseChannel{ReleaseChannelStable, ReleaseChannelLatest}
}

func GetReleaseStatus() (ReleaseStatus, error) {
	snapshot := GetFactorioServer().Snapshot()
	installedVersion := snapshot.BaseModVersion
	if installedVersion == "" {
		installedVersion = snapshot.Version.ReleaseString()
	} else if normalized, err := NormalizeExactReleaseVersion(installedVersion); err == nil {
		installedVersion = normalized
	}
	status := ReleaseStatus{
		InstalledVersion:  installedVersion,
		InstalledChannel:  "custom",
		AvailableVersions: []string{},
	}
	if state, err := LoadRuntimeState(); err == nil {
		status.InstalledTarget = state.ReleaseTarget
		if state.InstalledVersion != "" && versionsEqual(state.InstalledVersion, installedVersion) &&
			(state.ReleaseTarget == string(ReleaseChannelStable) || state.ReleaseTarget == string(ReleaseChannelLatest)) {
			status.InstalledChannel = state.ReleaseTarget
		}
	} else {
		status.MetadataError = err.Error()
	}

	var channels officialReleaseChannels
	if err := decodeOfficialMetadata(releaseChannelMetadataURL, &channels); err != nil {
		status.MetadataError = fmt.Sprintf("load official Factorio release channels: %v", err)
		return status, nil
	}
	var err error
	status.StableVersion, err = parseOfficialRelease(channels.Stable)
	if err != nil {
		status.MetadataError = fmt.Sprintf("decode official stable release: %v", err)
		return status, nil
	}
	status.LatestVersion, err = parseOfficialRelease(channels.Experimental)
	if err != nil {
		status.MetadataError = fmt.Sprintf("decode official experimental release: %v", err)
		return status, nil
	}

	var official officialReleaseResponse
	if err := decodeOfficialMetadata(releaseVersionMetadataURL, &official); err != nil {
		status.MetadataError = fmt.Sprintf("load official Factorio version archive: %v", err)
		status.AvailableVersions = sortedAvailableVersions(official, status.StableVersion, status.LatestVersion)
		return status, nil
	}
	status.AvailableVersions = sortedAvailableVersions(official, status.StableVersion, status.LatestVersion)

	// From this point the official channel versions are authoritative. The
	// persisted rolling target remains available separately, but it must not make
	// an older binary look like the current Stable or Experimental release.
	status.InstalledChannel = "custom"
	var installed, stable, latest Version
	if err := installed.UnmarshalText([]byte(status.InstalledVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse installed Factorio version: %v", err)
		return status, nil
	}
	if err := stable.UnmarshalText([]byte(status.StableVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse stable Factorio version: %v", err)
		return status, nil
	}
	if err := latest.UnmarshalText([]byte(status.LatestVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse latest Factorio version: %v", err)
		return status, nil
	}
	switch {
	case installed.Equals(stable):
		status.InstalledChannel = string(ReleaseChannelStable)
	case installed.Equals(latest):
		status.InstalledChannel = string(ReleaseChannelLatest)
	}
	return status, nil
}
