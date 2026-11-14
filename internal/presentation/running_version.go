package presentation

import (
	"strings"
	"time"
)

// RunningBuild describes a build that follows a moving ref instead of a
// release. Channel is empty for releases; then only the version is shown.
type RunningBuild struct {
	// Channel names the ref the laboratory follows: "master", another branch,
	// "commit" for a pinned commit, or "local" for a local checkout.
	Channel string
	// Revision is the full commit of that build.
	Revision string
	// Date is the commit date as YYYYMMDD.
	Date string
}

// runningVersionLabel names the running Nixorium for people: the version of a
// release, or "master · 06ae395 · 14 Nov" for a build of a moving ref, which
// is not that release even though VERSION still names it. moving reports the
// second case, and a local development build.
func runningVersionLabel(version string, build RunningBuild) (label string, moving bool) {
	channel := strings.TrimSpace(build.Channel)
	if channel == "" {
		switch version = strings.TrimSpace(version); version {
		case "":
			return "unknown", false
		case "development":
			return version, true
		}
		return version, false
	}
	parts := []string{channel}
	if revision := strings.TrimSpace(build.Revision); revision != "" {
		parts = append(parts, revision[:min(7, len(revision))])
	}
	if date, err := time.Parse("20060102", strings.TrimSpace(build.Date)); err == nil {
		parts = append(parts, date.Format("2 Jan 2006"))
	}
	return strings.Join(parts, " · "), true
}

func (model dashboardModel) runningVersionLabel() (string, bool) {
	return runningVersionLabel(model.actions.RunningVersion, model.actions.RunningBuild)
}
