package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeShareContent struct {
	files []domain.ShareFile
	data  map[int]string
}

func (content *fakeShareContent) Files(transfer string) ([]domain.ShareFile, error) {
	if transfer != "t1" {
		return nil, errors.New("missing")
	}
	return content.files, nil
}

func (content *fakeShareContent) Open(transfer string, index int) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(content.data[index])), nil
}

type shareSession struct {
	fakeLockSession
	sent map[string][]string
}

func (session shareSession) SendFiles(entries []classroomview.FileEntry, content func(int) (io.ReadCloser, error)) ([]string, error) {
	if session.source.silent[session.name] {
		return nil, errors.New("connection lost")
	}
	names := []string{}
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		reader, _ := content(index)
		data, _ := io.ReadAll(reader)
		names = append(names, entry.Path+"="+string(data))
	}
	session.source.mutex.Lock()
	session.sent[session.name] = names
	session.source.mutex.Unlock()
	return []string{"Lesson"}, nil
}

type shareSource struct {
	*fakeLockSource
	sent map[string][]string
}

func (source shareSource) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	session, err := source.fakeLockSource.Connect(ctx, host)
	if err != nil {
		return nil, err
	}
	return shareSession{session.(fakeLockSession), source.sent}, nil
}

func TestSharePlanAndApply(t *testing.T) {
	lab := lockLab()
	lab.away["pc02"] = classroomview.ErrUnreachable
	lab.silent["pc03"] = true
	source := shareSource{lab, map[string][]string{}}
	content := &fakeShareContent{files: []domain.ShareFile{{Path: "Lesson", Dir: true}, {Path: "Lesson/a.txt", Size: 5, SHA256: "abc"}}, data: map[int]string{1: "hello"}}
	manager := NewShareManager(source, content)
	if plan := manager.Plan(context.Background(), "/srv/lab", "pc01", "missing"); !plan.HasErrors() {
		t.Fatalf("a missing transfer was planned: %+v", plan)
	}
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03", "t1")
	if plan.HasErrors() || plan.Bytes != 5 || !plan.Targets[0].Eligible || plan.Targets[1].Eligible || !strings.Contains(plan.Message, "1 item (5 bytes)") {
		t.Fatalf("plan = %+v", plan)
	}
	// The same files prepared again under another name keep the review.
	again := plan
	again.Transfer = "t2"
	if domain.ShareReviewToken(again) != plan.ReviewToken {
		t.Fatal("the review depends on the transfer name")
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || report.Targets[0].State != "delivered" || report.Targets[1].State != "not-sent" || report.Targets[2].State != "unconfirmed" {
		t.Fatalf("report = %+v", report)
	}
	if got := strings.Join(source.sent["pc01"], ","); got != "Lesson/a.txt=hello" {
		t.Fatalf("pc01 received %q", got)
	}
	// Changed content refuses the old review.
	content.files[1].SHA256 = "def"
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "changed") {
		t.Fatalf("changed files = %+v", report)
	}
}
