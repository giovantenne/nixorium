package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type restoredLabFake struct {
	moved, recorded bool
	err             error
}

func (f *restoredLabFake) AdoptRestoredLab(repository, restored, adminKey string, now time.Time) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.moved = true
	return repository + "-before-restore-x", nil
}

func (f *restoredLabFake) RecordRestoredBackup(string, string, domain.BackupReport) {
	f.recorded = true
}

func TestUseRestoredLab(t *testing.T) {
	restored := domain.BackupReport{State: "completed", Path: "/home/admin/nixorium-restored", Revision: "abc"}
	ready := func(context.Context, string) domain.KeyReconcileReport {
		return domain.KeyReconcileReport{State: "ready"}
	}
	installed := func(context.Context) domain.ActionReport { return domain.ActionReport{State: "completed"} }

	fake := &restoredLabFake{}
	report := RestoredLab{Source: fake, VerifyKeys: func(context.Context, string) domain.KeyReconcileReport {
		return domain.KeyReconcileReport{State: "action-required"}
	}, InstallSecrets: installed}.Use(t.Context(), "/home/admin/nixorium-deployment", "", "", restored)
	if !report.HasErrors() || fake.moved {
		t.Fatalf("mismatched keys were put in use: %+v", report)
	}

	fake = &restoredLabFake{err: errors.New("this controller already uses its own lab keys")}
	if report := (RestoredLab{Source: fake, VerifyKeys: ready, InstallSecrets: installed}).Use(t.Context(), "/home/admin/nixorium-deployment", "", "", restored); !report.HasErrors() || fake.recorded {
		t.Fatalf("refused move reported success: %+v", report)
	}

	fake = &restoredLabFake{}
	report = RestoredLab{Source: fake, VerifyKeys: ready, InstallSecrets: func(context.Context) domain.ActionReport {
		return domain.ActionReport{State: "failed", Message: "unit failed"}
	}}.Use(t.Context(), "/home/admin/nixorium-deployment", "", "/media/usb/b.age", restored)
	if report.State != "partial" || !fake.moved || report.Previous == "" {
		t.Fatalf("failed key installation: %+v", report)
	}

	fake = &restoredLabFake{}
	report = RestoredLab{Source: fake, VerifyKeys: ready, InstallSecrets: installed}.Use(t.Context(), "/home/admin/nixorium-deployment", "", "/media/usb/b.age", restored)
	if report.State != "completed" || !fake.recorded || report.Previous != "/home/admin/nixorium-deployment-before-restore-x" {
		t.Fatalf("use restored lab: %+v", report)
	}
	if blocked := (RestoredLab{Source: &restoredLabFake{}, VerifyKeys: ready, InstallSecrets: installed}).Use(t.Context(), "/r", "", "", domain.BackupReport{State: "blocked"}); !blocked.HasErrors() {
		t.Fatal("failed restore was put in use")
	}
}
