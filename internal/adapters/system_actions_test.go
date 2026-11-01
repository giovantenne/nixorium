package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestStartSystemUnitRejectsUnknownUnit(t *testing.T) {
	if err := (Local{}).StartSystemUnit(context.Background(), "ssh.service"); err == nil {
		t.Fatal("unknown privileged unit was accepted")
	}
}

func TestControlSystemUnitRejectsUnknownVerbUnitPairs(t *testing.T) {
	local := Local{}
	for _, test := range []struct {
		verb string
		unit string
	}{
		{verb: "restart", unit: "nixorium-pxe.service"},
		{verb: "start", unit: "nixorium-pxe-network.service"},
		{verb: "stop", unit: "nixorium-pxe-recover.service"},
		{verb: "stop", unit: "nixorium-prepare-pxe.service"},
		{verb: "start", unit: "nixorium-harmonia.service"},
		{verb: "restart", unit: "harmonia.service"},
		{verb: "restart", unit: "nixorium-restart-cache.service"},
	} {
		if err := local.ControlSystemUnit(context.Background(), test.verb, test.unit); err == nil {
			t.Fatalf("accepted %s %s", test.verb, test.unit)
		}
	}
}

func TestServiceRestartAllowlistRejectsDirectCacheStart(t *testing.T) {
	if err := (Local{}).ControlSystemUnit(context.Background(), "start", "nixorium-harmonia.service"); err == nil {
		t.Fatal("cache start was accepted outside the restart workflow")
	}
}

func TestControllerApplyUnitAllowlistAcceptsOnlyFullRevisionInstance(t *testing.T) {
	if !controllerApplyUnitPattern.MatchString("nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service") {
		t.Fatal("valid revision-bound controller unit was rejected")
	}
	for _, unit := range []string{
		"nixorium-apply-controller@short.service",
		"nixorium-apply-controller@0123456789abcdef0123456789abcdef0123456g.service",
		"nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567/evil.service",
	} {
		if controllerApplyUnitPattern.MatchString(unit) {
			t.Fatalf("unsafe unit %q was accepted", unit)
		}
	}
}

func TestCleanupUnitAllowlistAcceptsOnlyReviewedDigest(t *testing.T) {
	if !cleanupUnitPattern.MatchString("nixorium-clean-generations@0123456789abcdef.service") {
		t.Fatal("valid cleanup unit was rejected")
	}
	for _, unit := range []string{
		"nixorium-clean-generations@.service",
		"nixorium-clean-generations@0123456789ABCDEF.service",
		"nixorium-clean-generations@0123456789abcdef0.service",
		"nixorium-clean-generations@0123456789abcde;.service",
	} {
		if cleanupUnitPattern.MatchString(unit) {
			t.Fatalf("unsafe unit %q was accepted", unit)
		}
	}
	if err := (Local{}).CleanControllerGenerations(context.Background(), "../../x"); err == nil {
		t.Fatal("invalid digest reached systemctl")
	}
}

func TestSystemctlArgumentsDisableAskPassword(t *testing.T) {
	got := systemctlArguments("show", "unit")
	if len(got) != 3 || got[2] != "--no-ask-password" || got[0] != "show" {
		t.Fatalf("unexpected arguments: %v", got)
	}
}

// An interrupted client update must never stop a newer release from reaching
// the controller (owner decision 2026-10-15), while every other start stays
// refused by the record.
func TestControlSystemUnitStartsControllerApplyWithPendingDeployment(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, deploymentPendingName), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	noConflict := func(context.Context) error { return nil }
	for _, test := range []struct {
		verb    string
		unit    string
		started bool
	}{
		{verb: "start", unit: "nixorium-apply-controller.service", started: true},
		{verb: "start", unit: "nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service", started: true},
		{verb: "start", unit: "nixorium-pxe-recover.service", started: true},
		{verb: "stop", unit: "nixorium-pxe.service", started: true},
		{verb: "start", unit: "nixorium-prepare-pxe.service"},
		{verb: "start", unit: "nixorium-pxe.service"},
		{verb: "start", unit: "nixorium-remote-install.service"},
		{verb: "start", unit: "nixorium-install-secrets.service"},
		{verb: "start", unit: "nixorium-restart-cache.service"},
		{verb: "start", unit: "nixorium-clean-generations@0123456789abcdef.service"},
	} {
		var calls [][]string
		runner := func(_ context.Context, arguments ...string) error {
			calls = append(calls, arguments)
			return nil
		}
		err := controlSystemUnit(context.Background(), directory, noConflict, runner, test.verb, test.unit)
		if test.started {
			if err != nil || len(calls) != 1 || calls[0][0] != test.verb || calls[0][1] != test.unit {
				t.Fatalf("%s %s with a pending record: err=%v calls=%v", test.verb, test.unit, err, calls)
			}
			continue
		}
		if code, _ := domain.BlockedCodeOf(err); err == nil || len(calls) != 0 || code != "DEPLOY-PENDING" {
			t.Fatalf("%s %s was not refused by the pending record: err=%v calls=%v", test.verb, test.unit, err, calls)
		}
	}
	if _, err := os.Lstat(filepath.Join(directory, deploymentPendingName)); err != nil {
		t.Fatalf("controller apply changed the pending record: %v", err)
	}
}

func TestControlSystemUnitKeepsManagedJobConflictForControllerApply(t *testing.T) {
	busy := errors.New("nixorium-prepare-pxe.service is already running")
	called := false
	err := controlSystemUnit(context.Background(), t.TempDir(), func(context.Context) error { return busy }, func(context.Context, ...string) error {
		called = true
		return nil
	}, "start", "nixorium-apply-controller@0123456789abcdef0123456789abcdef01234567.service")
	if !errors.Is(err, busy) || called {
		t.Fatalf("controller apply ignored a managed job in flight: err=%v called=%t", err, called)
	}
}

func TestUnitFailureCauseNamesWhatTheAdministratorCanFix(t *testing.T) {
	offline := "Building the reviewed controller system\nwarning: unable to download 'https://cache.nixos.org/x.narinfo': Could not resolve hostname (6) Could not resolve host: cache.nixos.org; retrying in 270 ms (attempt 1/5)\nerror: unable to download 'https://cache.nixos.org/x.narinfo': Could not resolve hostname (6) Could not resolve host: cache.nixos.org\nController apply stopped unexpectedly\n"
	if got := unitFailureCause(offline); got != "the controller cannot reach the Internet: it could not find cache.nixos.org. Check its network cable and its DNS server, then try again" {
		t.Fatalf("offline cause = %q", got)
	}
	if got := unitFailureCause("copying\nerror: writing to file: No space left on device\n"); !strings.Contains(got, "disk is full") {
		t.Fatalf("disk cause = %q", got)
	}
	if got := unitFailureCause("building\nerror: builder for '/nix/store/x.drv' failed with exit code 2\nstopped\n"); got != "Nix reported: builder for '/nix/store/x.drv' failed with exit code 2" {
		t.Fatalf("nix cause = %q", got)
	}
	if got := unitFailureCause("all good\nstopped\n"); got != "" {
		t.Fatalf("unknown cause = %q", got)
	}
}
