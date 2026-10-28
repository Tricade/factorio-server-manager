package factorio

import (
	"fmt"
	"strings"
)

func choosePortalRelease(details ModPortalStruct, constraints []ParsedModDependency, preferred Version) (ModPortalRelease, error) {
	if !preferred.Equals(NilVersion) {
		for _, release := range details.Releases {
			if release.Version.Equals(preferred) && release.Compatibility && matchesAllConstraints(release.Version, constraints) {
				return release, nil
			}
		}
	}

	var selected *ModPortalRelease
	for _, release := range details.Releases {
		if !release.Compatibility || !matchesAllConstraints(release.Version, constraints) {
			continue
		}
		if selected == nil || release.Version.Greater(selected.Version) {
			releaseCopy := release
			selected = &releaseCopy
		}
	}
	if selected == nil {
		return ModPortalRelease{}, fmt.Errorf("no compatible release of %s satisfies %s", details.Name, joinConstraints(constraints))
	}
	return *selected, nil
}

func matchesAllConstraints(version Version, constraints []ParsedModDependency) bool {
	for _, dependency := range constraints {
		if dependency.Operator == "" || dependency.Version.Equals(NilVersion) {
			continue
		}
		if !version.Compatible(dependency.Version, dependency.Operator) {
			return false
		}
	}
	return true
}

func appendUniqueConstraint(constraints []ParsedModDependency, candidate ParsedModDependency) []ParsedModDependency {
	for _, existing := range constraints {
		if existing.Raw == candidate.Raw {
			return constraints
		}
	}
	return append(constraints, candidate)
}

func appendUniqueString(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func joinConstraints(constraints []ParsedModDependency) string {
	values := make([]string, 0, len(constraints))
	for _, dependency := range constraints {
		values = append(values, dependency.Raw)
	}
	return strings.Join(values, ", ")
}
