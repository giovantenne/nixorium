package app

import (
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestParseUpdateReleaseClassifiesAndComparesTargets(t *testing.T) {
	stable, err := parseUpdateRelease("v2.1.0")
	if err != nil || stable.Channel() != domain.UpdateChannelStable {
		t.Fatalf("stable = %+v, error = %v", stable, err)
	}
	prerelease, err := parseUpdateRelease("v2.1.0-beta.3")
	if err != nil || prerelease.Channel() != domain.UpdateChannelPrerelease || compareUpdateReleases(prerelease, stable) >= 0 {
		t.Fatalf("prerelease = %+v, error = %v", prerelease, err)
	}
	older, _ := parseUpdateRelease("v1.9.9")
	if compareUpdateReleases(older, prerelease) >= 0 {
		t.Fatal("older release did not sort below prerelease")
	}
	for _, value := range []string{"2.1.0", "v2.1", "master", "v02.1.0", "v2.1.0-"} {
		if _, err := parseUpdateRelease(value); err == nil {
			t.Fatalf("invalid release accepted: %q", value)
		}
	}
	beta2, _ := parseUpdateRelease("v2.1.0-beta.2+build.7")
	beta10, _ := parseUpdateRelease("v2.1.0-beta.10")
	if compareUpdateReleases(beta2, beta10) >= 0 {
		t.Fatal("numeric prerelease identifiers were not compared numerically")
	}
	if _, err := parseUpdateRelease("v2.1.0-beta.01"); err == nil {
		t.Fatal("leading-zero numeric prerelease identifier was accepted")
	}
}
