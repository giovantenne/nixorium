package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// deskSession is a student's session with files on the desktop.
type deskSession struct {
	screenSession
	entries []classroomview.FileEntry
	content map[int]string
}

func (session deskSession) ListDesktop() ([]classroomview.FileEntry, int, error) {
	return session.entries, 0, nil
}

func (session deskSession) ReadDesktopFile(index int, size int64, piece func([]byte) error) error {
	return piece([]byte(session.content[index]))
}

func (session deskSession) EndCollect() error { return nil }

// deskLab has pc01 with a desktop, pc02 switched off and pc03 empty.
type deskLab struct{}

func (deskLab) LabMeta(context.Context, string) (domain.LabMeta, error) {
	meta := domain.LabMeta{}
	meta.Controller.Name = "pc99"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}}
	return meta, nil
}

func (deskLab) Connect(_ context.Context, host domain.HostMeta) (classroomview.Session, error) {
	switch host.Name {
	case "pc01":
		return deskSession{entries: []classroomview.FileEntry{{Path: "Homework", Dir: true}, {Path: "Homework/essay.odt", Size: 5}, {Path: "notes.txt", Size: 3}}, content: map[int]string{1: "essay", 2: "abc"}}, nil
	case "pc03":
		return deskSession{entries: []classroomview.FileEntry{}}, nil
	}
	return nil, classroomview.ErrUnreachable
}

func collectWorker(t *testing.T) classroomWorker {
	t.Helper()
	store := testCollectStore(t)
	worker := classroomWorker{
		repository: "/srv/lab",
		view:       newViewServer(fakeViewSource{}),
		viewOn:     func() bool { return true },
		collect:    app.NewCollectManager(deskLab{}, store),
		plans:      newPlanStore(),
		collected:  store,
	}
	worker.view.actions = worker.handle
	worker.view.desktops = newDesktopBroker()
	return worker
}

// helperSaves answers the save jobs of uid like the desktop helper: it
// copies the files into home and reports where.
func helperSaves(t *testing.T, broker *desktopBroker, uid int, home string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for ctx.Err() == nil {
			job := broker.Wait(ctx, uid)
			if job.id == "" {
				continue
			}
			names, message := []string{}, ""
			placed, err := helperCopy(broker, uid, job.id, filepath.Join(home, "Nixorium", "Collected"))
			if err != nil {
				message = err.Error()
			}
			for _, name := range placed {
				names = append(names, "Nixorium/Collected/"+name)
			}
			broker.SaveDone(uid, job.id, names, message)
		}
	}()
	time.Sleep(20 * time.Millisecond)
}

func TestPageCollectsDesktopsIntoTheTeachersHome(t *testing.T) {
	worker := collectWorker(t)
	server := worker.view
	address, err := server.Open(1000)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	cookie := viewRequest(server, "/open"+strings.SplitN(address, "/open", 2)[1], host, "").Result().Cookies()[0].Value
	origin := "http://" + host
	// Without the helper the page cannot save, so it does not collect.
	plan := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"collect","computers":["pc01","pc02","pc03"]}`)
	var review actionReview
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || review.Ready || !strings.Contains(review.Message, "open it again from Nixorium") {
		t.Fatalf("no helper = %s", plan.Body.String())
	}
	home := t.TempDir()
	helperSaves(t, server.desktops, 1000, home)
	plan = actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"collect","computers":["pc01","pc02","pc03"]}`)
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || !review.Ready || review.Confirm != "Collect from 2 computers" || review.Title != "Collect" {
		t.Fatalf("review = %s", plan.Body.String())
	}
	if strings.Contains(plan.Body.String(), "sha256:") {
		t.Fatal("the review token reached the page")
	}
	if review.Rows[0].Note != "2 files (8 bytes)." || !review.Rows[1].Skip || review.Rows[2].Note != "Nothing on the desktop." {
		t.Fatalf("rows = %+v", review.Rows)
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	var result actionResult
	if err := json.Unmarshal(apply.Body.Bytes(), &result); err != nil || len(result.Rows) != 3 {
		t.Fatalf("result = %s", apply.Body.String())
	}
	outcomes := []string{}
	for _, row := range result.Rows {
		outcomes = append(outcomes, row.Name+":"+row.Outcome)
	}
	if strings.Join(outcomes, ",") != "pc01:done,pc02:off,pc03:empty" || !strings.Contains(result.Rows[0].Note, "In Nixorium/Collected/") {
		t.Fatalf("outcomes = %v %+v", outcomes, result.Rows)
	}
	folders, _ := filepath.Glob(filepath.Join(home, "Nixorium", "Collected", "*", "pc01", "Homework", "essay.odt"))
	if len(folders) != 1 {
		t.Fatalf("collected files = %v", folders)
	}
	if content, _ := os.ReadFile(folders[0]); string(content) != "essay" {
		t.Fatalf("essay = %q", content)
	}
	// The worker's copy is gone once saved.
	if len(worker.collected.collections) != 0 || len(server.desktops.saves) != 0 {
		t.Fatal("the collected files were kept after saving")
	}
	// The review applies once.
	again := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	if !strings.Contains(again.Body.String(), `"expired"`) {
		t.Fatalf("second apply = %s", again.Body.String())
	}
}

func TestCollectOperationsStayWithinTheClassroomBoundary(t *testing.T) {
	worker := collectWorker(t)
	teacher := adapters.WithClassroomPeerUID(context.Background(), 1000)
	request := func(ctx context.Context, operation domain.ClassroomOperation, configure func(*domain.ClassroomRequest)) domain.ClassroomResponse {
		message, err := domain.NewClassroomRequest(operation)
		if err != nil {
			t.Fatal(err)
		}
		if configure != nil {
			configure(&message)
		}
		return worker.handle(ctx, message)
	}
	// Only a caller the socket identified may collect.
	if response := request(context.Background(), domain.ClassroomCollectPlanOperation, nil); response.State != "failed" {
		t.Fatalf("anonymous plan = %+v", response)
	}
	worker.viewOn = func() bool { return false }
	if response := request(teacher, domain.ClassroomCollectPlanOperation, nil); response.State != "failed" || !strings.Contains(response.Message, "CLASSROOM-VIEW-OFF") {
		t.Fatalf("view off = %+v", response)
	}
	worker.viewOn = func() bool { return true }
	response := request(teacher, domain.ClassroomCollectPlanOperation, func(message *domain.ClassroomRequest) {
		message.Requested, message.CollectFolder = "pc01,pc03", "/home/teacher/Homework"
	})
	if response.CollectPlan == nil || response.CollectPlan.HasErrors() || response.CollectPlan.Repository != "/srv/lab" {
		t.Fatalf("plan = %+v", response)
	}
	plan := *response.CollectPlan
	// The same target validation as every classroom action.
	for _, requested := range []string{"pc99", "pc42", "10.0.0.1", "@lab;rm"} {
		refused := request(teacher, domain.ClassroomCollectPlanOperation, func(message *domain.ClassroomRequest) {
			message.Requested, message.CollectFolder = requested, "/tmp"
		})
		if refused.CollectPlan == nil || !refused.CollectPlan.HasErrors() {
			t.Fatalf("%s = %+v", requested, refused)
		}
	}
	foreign := plan
	foreign.Repository = "/home/teacher/fake"
	if response := request(teacher, domain.ClassroomCollectApplyOperation, func(message *domain.ClassroomRequest) { message.CollectPlan = &foreign }); response.State != "failed" || response.CollectReport != nil {
		t.Fatalf("foreign repository = %+v", response)
	}
	response = request(teacher, domain.ClassroomCollectApplyOperation, func(message *domain.ClassroomRequest) { message.CollectPlan = &plan })
	report := response.CollectReport
	if report == nil || report.Save == "" || report.Folder == "" {
		t.Fatalf("apply = %+v", response)
	}
	// The collected files are the teacher's to save: another user of the
	// classroom socket cannot list, read or end them.
	administrator := adapters.WithClassroomPeerUID(context.Background(), 1001)
	for _, operation := range []domain.ClassroomOperation{domain.ClassroomSaveItemsOperation, domain.ClassroomSaveReadOperation, domain.ClassroomSaveDoneOperation} {
		if response := request(administrator, operation, func(message *domain.ClassroomRequest) { message.DesktopJob = report.Save }); response.State != "failed" {
			t.Fatalf("%s by another user = %+v", operation, response)
		}
	}
	items := request(teacher, domain.ClassroomSaveItemsOperation, func(message *domain.ClassroomRequest) { message.DesktopJob = report.Save })
	if items.State != "completed" || items.DesktopKind != domain.DesktopSaveCollected || items.SaveTotal != 5 || items.SaveItems[2].Path != report.Folder+"/pc01/Homework" {
		t.Fatalf("items = %+v", items)
	}
	piece := request(teacher, domain.ClassroomSaveReadOperation, func(message *domain.ClassroomRequest) {
		message.DesktopJob, message.ShareIndex, message.ShareOffset = report.Save, 3, 1
	})
	if string(piece.SaveData) != "ssay" {
		t.Fatalf("piece = %+v", piece)
	}
	if response := request(teacher, domain.ClassroomSaveDoneOperation, func(message *domain.ClassroomRequest) {
		message.DesktopJob, message.SaveNames = report.Save, []string{report.Folder}
	}); response.State != "completed" || len(worker.collected.collections) != 0 {
		t.Fatalf("done = %+v", response)
	}
	// The request types reach the worker through the strict decoder.
	for _, operation := range []domain.ClassroomOperation{domain.ClassroomCollectPlanOperation, domain.ClassroomCollectApplyOperation, domain.ClassroomSaveItemsOperation, domain.ClassroomSaveReadOperation, domain.ClassroomSaveDoneOperation} {
		if !operation.Valid() {
			t.Fatalf("%s is not a valid operation", operation)
		}
	}
}
