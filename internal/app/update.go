package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

var updateReleasePattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var numericReleaseIdentifier = regexp.MustCompile(`^[0-9]+$`)

type updateRelease struct {
	Tag        string
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease string
	Build      string
}

func parseUpdateRelease(tag string) (updateRelease, error) {
	match := updateReleasePattern.FindStringSubmatch(tag)
	if match == nil {
		return updateRelease{}, fmt.Errorf("target %q must be a v-prefixed Semantic Version release tag", tag)
	}
	values := make([]uint64, 3)
	for index := range values {
		value, err := strconv.ParseUint(match[index+1], 10, 64)
		if err != nil {
			return updateRelease{}, fmt.Errorf("target %q has an invalid numeric version", tag)
		}
		values[index] = value
	}
	for _, identifier := range strings.Split(match[4], ".") {
		if len(identifier) > 1 && identifier[0] == '0' && numericReleaseIdentifier.MatchString(identifier) {
			return updateRelease{}, fmt.Errorf("target %q has a numeric prerelease identifier with a leading zero", tag)
		}
	}
	return updateRelease{Tag: tag, Major: values[0], Minor: values[1], Patch: values[2], Prerelease: match[4], Build: match[5]}, nil
}

func (release updateRelease) Channel() domain.UpdateChannel {
	if release.Prerelease != "" {
		return domain.UpdateChannelPrerelease
	}
	return domain.UpdateChannelStable
}

func compareUpdateReleases(left, right updateRelease) int {
	for _, pair := range [][2]uint64{{left.Major, right.Major}, {left.Minor, right.Minor}, {left.Patch, right.Patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if left.Prerelease == right.Prerelease {
		return 0
	}
	if left.Prerelease == "" {
		return 1
	}
	if right.Prerelease == "" {
		return -1
	}
	leftParts := strings.Split(left.Prerelease, ".")
	rightParts := strings.Split(right.Prerelease, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		if leftParts[index] == rightParts[index] {
			continue
		}
		leftNumeric := numericReleaseIdentifier.MatchString(leftParts[index])
		rightNumeric := numericReleaseIdentifier.MatchString(rightParts[index])
		if leftNumeric && rightNumeric {
			if len(leftParts[index]) < len(rightParts[index]) {
				return -1
			}
			if len(leftParts[index]) > len(rightParts[index]) {
				return 1
			}
			return strings.Compare(leftParts[index], rightParts[index])
		}
		if leftNumeric {
			return -1
		}
		if rightNumeric {
			return 1
		}
		return strings.Compare(leftParts[index], rightParts[index])
	}
	if len(leftParts) < len(rightParts) {
		return -1
	}
	return 1
}
