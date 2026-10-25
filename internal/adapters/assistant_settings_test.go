package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// assistantSettingsBase is a laboratory whose public settings contain values
// the read tool masks: credentialsVersion is withheld, and the sk- inside
// "task-list" and "desk-teacher" looks like a key.
func assistantSettingsBase() domain.LabSettingsFile {
	on, off := true, false
	return domain.LabSettingsFile{SchemaVersion: 1, Lab: domain.LabSettings{
		CredentialsVersion: 3, DeploymentMode: "laboratory", ClassroomView: &on, Assistant: &off,
		MasterDHCPIP: "192.0.2.10", NetworkBase: "10.0.0.0", NetworkPrefix: 24, PCCount: 20, MasterHostNumber: 99,
		InterfaceName: "enp0s3", ControllerInterfaceName: "enp1s0", ClientInterfaceName: "enp2s0", HostInterfaceNames: map[string]string{"pc01": "enp3s0"},
		TeacherUser: "desk-teacher", StudentUser: "student", HomepageURL: "https://school.example/task-list",
		StudentGitName: "Student", StudentGitEmail: "student@school.example", AdminGitName: "Admin", AdminGitEmail: "admin@school.example",
		TimeZone: "Europe/Rome", DefaultLocale: "en_US.UTF-8", ExtraLocale: "it_IT.UTF-8", KeyboardLayout: "it", ConsoleKeyMap: "it2",
	}}
}

func assistantSettingsFixture(t *testing.T) (*AssistantWorkspace, AssistantWorkspaceStore, string, []byte) {
	t.Helper()
	repo := workspaceRepository(t)
	base, err := domain.MarshalPublicLabSettings(assistantSettingsBase())
	if err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repo, "lab-settings.json", string(base))
	writeGitReviewFile(t, repo, "modules/lab-settings.json", string(base))
	workspaceTestGit(t, repo, "add", "-A")
	workspaceTestGit(t, repo, "commit", "-qm", "settings fixture")
	store := AssistantWorkspaceStore{Directory: t.TempDir(), Version: "test"}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, store, repo, base
}

// assistantSettingsModelWrite does what the model does: read the file through
// the tool, edit that copy and write it back whole.
func assistantSettingsModelWrite(t *testing.T, w *AssistantWorkspace, edit func(tree map[string]any)) map[string]any {
	t.Helper()
	read := assistantFileCall(t, w, "read_file", map[string]any{"path": "lab-settings.json"})
	var tree map[string]any
	if err := json.Unmarshal([]byte(read["content"].(string)), &tree); err != nil {
		t.Fatal(err)
	}
	edit(tree)
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return assistantFileCall(t, w, "write_file", map[string]any{"path": "lab-settings.json", "content": string(data)})
}

func assistantSettingsLab(tree map[string]any) map[string]any { return tree["lab"].(map[string]any) }

func assistantCandidateSettings(t *testing.T, w *AssistantWorkspace) domain.LabSettingsFile {
	t.Helper()
	data, err := w.raw("lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	settings, issues := domain.DecodeLabSettings(data)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return settings
}

func assistantSettingsUntouched(t *testing.T, w *AssistantWorkspace, store AssistantWorkspaceStore, base []byte) {
	t.Helper()
	if count, err := store.ChangedFiles(w.Conversation.ID); err != nil || count != 0 {
		t.Fatalf("changed files %d: %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(store.Directory, "conversations", w.Conversation.ID, "overlay", "lab-settings.json")); !os.IsNotExist(err) {
		t.Fatalf("a refused write left a candidate file: %v", err)
	}
	if data, err := w.raw("lab-settings.json"); err != nil || string(data) != string(base) {
		t.Fatalf("candidate changed: %v", err)
	}
}

func TestAssistantSettingsAcceptsTheWritableFields(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	result := assistantSettingsModelWrite(t, w, func(tree map[string]any) {
		lab := assistantSettingsLab(tree)
		lab["homepageUrl"] = "https://intranet.school.example/start"
		lab["timeZone"] = "Europe/London"
		lab["defaultLocale"] = "en_GB.UTF-8"
		lab["extraLocale"] = "fr_FR.UTF-8"
		lab["keyboardLayout"] = "gb"
		lab["studentGitName"] = "Pupil"
		lab["studentGitEmail"] = "pupil@school.example"
		lab["adminGitName"] = "Technician"
		lab["adminGitEmail"] = "technician@school.example"
	})
	if result["ok"] != true {
		t.Fatal(result)
	}
	got := assistantCandidateSettings(t, w)
	want := assistantSettingsBase()
	want.Lab.HomepageURL, want.Lab.TimeZone, want.Lab.DefaultLocale, want.Lab.ExtraLocale, want.Lab.KeyboardLayout = "https://intranet.school.example/start", "Europe/London", "en_GB.UTF-8", "fr_FR.UTF-8", "gb"
	want.Lab.StudentGitName, want.Lab.StudentGitEmail, want.Lab.AdminGitName, want.Lab.AdminGitEmail = "Pupil", "pupil@school.example", "Technician", "technician@school.example"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate %+v", got.Lab)
	}
	data, _ := w.raw("lab-settings.json")
	if canonical, _ := domain.MarshalPublicLabSettings(want); string(data) != string(canonical) {
		t.Fatal("the candidate is not the file Nixorium writes")
	}
	fields := []string{}
	for _, change := range domain.DiffLabSettings(assistantSettingsBase(), got) {
		fields = append(fields, change.Field)
	}
	if !reflect.DeepEqual(fields, []string{"lab.homepageUrl", "lab.studentGitName", "lab.studentGitEmail", "lab.adminGitName", "lab.adminGitEmail", "lab.timeZone", "lab.defaultLocale", "lab.extraLocale", "lab.keyboardLayout"}) {
		t.Fatalf("fields %v", fields)
	}
	// The value the read tool masks and the model never saw stay as they were.
	if got.Lab.CredentialsVersion != 3 || got.Lab.TeacherUser != "desk-teacher" || strings.Contains(string(data), "WITHHELD") || strings.Contains(string(data), "REDACTED") {
		t.Fatal("masked values were written")
	}
	reopened, err := store.Open(w.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if executable, written := reopened.modes["lab-settings.json"]; !written || executable {
		t.Fatalf("settings mode %v %v", executable, written)
	}
	if count, err := store.ChangedFiles(w.Conversation.ID); err != nil || count != 1 {
		t.Fatalf("changed files %d: %v", count, err)
	}
	info, err := os.Stat(filepath.Join(store.Directory, "conversations", w.Conversation.ID, "overlay", "lab-settings.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe overlay mode", err)
	}
	// Writing the original values back restores the saved file exactly.
	if result := assistantSettingsModelWrite(t, w, func(tree map[string]any) {
		lab := assistantSettingsLab(tree)
		lab["homepageUrl"], lab["timeZone"], lab["defaultLocale"], lab["extraLocale"], lab["keyboardLayout"] = "https://school.example/ta[REDACTED]", "Europe/Rome", "en_US.UTF-8", "it_IT.UTF-8", "it"
		lab["studentGitName"], lab["studentGitEmail"], lab["adminGitName"], lab["adminGitEmail"] = "Student", "student@school.example", "Admin", "admin@school.example"
	}); result["ok"] != true {
		t.Fatal(result)
	}
	if count, err := store.ChangedFiles(w.Conversation.ID); err != nil || count != 0 {
		t.Fatalf("reverted settings still count: %d %v", count, err)
	}
	if data, _ := w.raw("lab-settings.json"); string(data) != string(base) {
		t.Fatal("revert did not restore the saved file")
	}
}

func TestAssistantSettingsRefusesEachReadOnlyFieldByName(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	fields := []string{}
	labType := reflect.TypeFor[domain.LabSettings]()
	for index := 0; index < labType.NumField(); index++ {
		name, _, _ := strings.Cut(labType.Field(index).Tag.Get("json"), ",")
		if !domain.AssistantSettingWritable("lab." + name) {
			fields = append(fields, "lab."+name)
		}
	}
	if len(fields) < 14 {
		t.Fatalf("only %d read-only fields found", len(fields))
	}
	for _, field := range fields {
		result := assistantSettingsModelWrite(t, w, func(tree map[string]any) {
			assistantSettingsLab(tree)[strings.TrimPrefix(field, "lab.")] = "changed by the model"
		})
		message, _ := result["message"].(string)
		if result["ok"] == true || result["error"] != "settings_field_read_only" || !strings.HasPrefix(message, "settings_field_read_only: "+field+". Only these settings can be changed: lab.homepageUrl, lab.timeZone") {
			t.Fatalf("%s: %v", field, result)
		}
		assistantSettingsUntouched(t, w, store, base)
	}
	result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { tree["schemaVersion"] = 2 })
	if result["error"] != "settings_field_read_only" || !strings.HasPrefix(result["message"].(string), "settings_field_read_only: schemaVersion.") {
		t.Fatal(result)
	}
	assistantSettingsUntouched(t, w, store, base)
}

func TestAssistantSettingsRefusesReadOnlyChangeAlongsideWritableChange(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	result := assistantSettingsModelWrite(t, w, func(tree map[string]any) {
		lab := assistantSettingsLab(tree)
		lab["timeZone"] = "Europe/London"
		lab["ifaceName"] = "eth0"
	})
	if result["error"] != "settings_field_read_only" || !strings.HasPrefix(result["message"].(string), "settings_field_read_only: lab.ifaceName.") {
		t.Fatal(result)
	}
	assistantSettingsUntouched(t, w, store, base)
	// Nor can a changed account name or a changed password count slip through
	// as a copy of the masked value: only the exact view is accepted.
	for _, key := range []string{"credentialsVersion", "masterDhcpIp"} {
		result = assistantSettingsModelWrite(t, w, func(tree map[string]any) {
			lab := assistantSettingsLab(tree)
			lab["timeZone"] = "Europe/London"
			lab[key] = "changed by the model"
		})
		if result["error"] != "settings_field_read_only" || !strings.HasPrefix(result["message"].(string), "settings_field_read_only: lab."+key+".") {
			t.Fatal(key, result)
		}
		assistantSettingsUntouched(t, w, store, base)
	}
}

func TestAssistantSettingsReadBackIsRedacted(t *testing.T) {
	w, _, _, _ := assistantSettingsFixture(t)
	if result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { assistantSettingsLab(tree)["timeZone"] = "Europe/London" }); result["ok"] != true {
		t.Fatal(result)
	}
	read := assistantFileCall(t, w, "read_file", map[string]any{"path": "lab-settings.json"})
	content := read["content"].(string)
	for _, want := range []string{`"credentialsVersion": "[WITHHELD]"`, `"timeZone": "Europe/London"`, `"teacherUser": "de[REDACTED]"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("read-back lacks %s:\n%s", want, content)
		}
	}
	if strings.Contains(content, `"credentialsVersion": 3`) || strings.Contains(content, "desk-teacher") {
		t.Fatal("the read-back disclosed a masked value")
	}
	if matches := assistantFileCall(t, w, "search", map[string]any{"pattern": "WITHHELD", "path": "lab-settings.json"}); len(matches["matches"].([]any)) != 1 {
		t.Fatal(matches)
	}
}

func TestAssistantSettingsRefusesInvalidWrites(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	for name, test := range map[string]struct {
		edit func(map[string]any)
		want string
	}{
		"empty":    {func(tree map[string]any) { assistantSettingsLab(tree)["timeZone"] = "" }, "settings_invalid: lab.timeZone: must not be empty"},
		"homepage": {func(tree map[string]any) { assistantSettingsLab(tree)["homepageUrl"] = "file:///etc/passwd" }, "settings_invalid: lab.homepageUrl: must be an absolute http:// or https:// URL"},
		"marker": {func(tree map[string]any) {
			assistantSettingsLab(tree)["homepageUrl"] = "https://other.example/[REDACTED]"
		}, "settings_invalid: lab.homepageUrl contains a marker that replaces a value you could not read; do not copy it"},
		"unknown":    {func(tree map[string]any) { assistantSettingsLab(tree)["timezone"] = "UTC" }, "settings_invalid: unknown field lab.timezone"},
		"two lines":  {func(tree map[string]any) { assistantSettingsLab(tree)["adminGitName"] = "Admin\nName" }, "settings_invalid: lab.adminGitName must be one line of text without control characters"},
		"not a text": {func(tree map[string]any) { assistantSettingsLab(tree)["keyboardLayout"] = []any{"it"} }, "settings_invalid: lab.keyboardLayout must be text"},
	} {
		result := assistantSettingsModelWrite(t, w, test.edit)
		if result["ok"] == true || result["error"] != "settings_invalid" || !strings.HasPrefix(result["message"].(string), test.want) {
			t.Fatalf("%s: %v", name, result)
		}
		assistantSettingsUntouched(t, w, store, base)
	}
	for name, content := range map[string]string{"empty": "", "text": "timeZone = UTC", "array": "[]", "object": "{}"} {
		result := assistantFileCall(t, w, "write_file", map[string]any{"path": "lab-settings.json", "content": content})
		if result["ok"] == true || (result["error"] != "settings_invalid" && result["error"] != "settings_field_read_only") {
			t.Fatalf("%s: %v", name, result)
		}
		assistantSettingsUntouched(t, w, store, base)
	}
}

func TestAssistantSettingsRouteIsOnlyTheRootSettingsFile(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"delete_file", map[string]any{"path": "lab-settings.json"}},
		{"delete_file", map[string]any{"path": "modules/lab-settings.json"}},
		{"write_file", map[string]any{"path": "modules/lab-settings.json", "content": string(base)}},
	} {
		if result := assistantFileCall(t, w, call.name, call.args); result["error"] != "protected_path" {
			t.Fatalf("%s %v: %v", call.name, call.args["path"], result)
		}
	}
	assistantSettingsUntouched(t, w, store, base)
	// The mode comes from the base commit, whatever the model asks for.
	result := assistantFileCall(t, w, "write_file", map[string]any{"path": "lab-settings.json", "executable": true, "content": strings.Replace(assistantSettingsReadText(t, w), `"Europe/Rome"`, `"Europe/Paris"`, 1)})
	if result["ok"] != true {
		t.Fatal(result)
	}
	if w.modes["lab-settings.json"] {
		t.Fatal("the model made the settings file executable")
	}
}

func assistantSettingsReadText(t *testing.T, w *AssistantWorkspace) string {
	t.Helper()
	return assistantFileCall(t, w, "read_file", map[string]any{"path": "lab-settings.json"})["content"].(string)
}

// assistantSettingsReadyRun lets a plan reuse a synthetic evaluation, so no
// Nix is needed to review the settings.
func assistantSettingsPlan(t *testing.T, w *AssistantWorkspace, store AssistantWorkspaceStore) domain.AssistantSavePlan {
	t.Helper()
	key, err := assistantEvaluationInputs(t.Context(), w)
	if err != nil {
		t.Fatal(err)
	}
	assistantWriteReceipt(t, w, assistantEvaluationReceipt{SchemaVersion: 1, Key: key, VerifiedAt: time.Now().UTC(), Evaluation: domain.AssistantEvaluation{OK: true, ChangedHosts: []string{"pc01", "pc02", "pc99"}}})
	return planAssistantSave(t.Context(), store, w.Conversation.ID, assistantNoNix(t))
}

func TestAssistantSavePlanShowsSettingsImpactsAndSaves(t *testing.T) {
	w, store, repo, _ := assistantSettingsFixture(t)
	if result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { assistantSettingsLab(tree)["timeZone"] = "Europe/London" }); result["ok"] != true {
		t.Fatal(result)
	}
	plan := assistantSettingsPlan(t, w, store)
	if plan.State != "ready" || strings.Join(plan.Changed, ",") != "lab-settings.json" || len(plan.Added)+len(plan.Removed) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
	kinds := []string{}
	for _, impact := range plan.SettingsImpacts {
		kinds = append(kinds, impact.Kind)
	}
	if strings.Join(kinds, ",") != "controller-apply,client-update,pxe-prepare" {
		t.Fatalf("impacts %v", kinds)
	}
	lines := plan.SettingsReviewLines()
	if len(lines) != 3 || lines[0] != "Checked by Nixorium: Apply to controller so this controller uses the saved settings." || !strings.Contains(lines[1], "offline computers keep the old ones until updated") {
		t.Fatalf("review lines %q", lines)
	}
	for _, want := range []string{`-    "timeZone": "Europe/Rome"`, `+    "timeZone": "Europe/London"`} {
		if !strings.Contains(plan.Diff, want) {
			t.Fatalf("diff lacks %q:\n%s", want, plan.Diff)
		}
	}
	if strings.Contains(plan.Diff, "WITHHELD") || strings.Contains(plan.Diff, "REDACTED") {
		t.Fatal("the diff holds a stand-in")
	}
	again := assistantSettingsPlan(t, w, store)
	if again.ReviewToken != plan.ReviewToken {
		t.Fatal("review token is not stable")
	}
	root, err := openWorkspaceRoot(repo, unix.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	result := commitConfigurationTree(t.Context(), root, plan.Proposal, plan.ReviewToken, assistantCommit, nil)
	root.Close()
	if result.State != "saved" || result.RecoveryRequired {
		t.Fatalf("save: %+v", result)
	}
	saved, err := os.ReadFile(filepath.Join(repo, "lab-settings.json"))
	if err != nil || string(saved) != string(plan.Proposal.Candidate["lab-settings.json"].Data) || !strings.Contains(string(saved), `"timeZone": "Europe/London"`) || !strings.Contains(string(saved), `"credentialsVersion": 3`) {
		t.Fatalf("saved file: %v\n%s", err, saved)
	}
	if status := workspaceTestGit(t, repo, "status", "--porcelain"); status != "" {
		t.Fatal(status)
	}
	// A controller-only identity reaches no client.
	other, otherStore, _, _ := assistantSettingsFixture(t)
	if result := assistantSettingsModelWrite(t, other, func(tree map[string]any) { assistantSettingsLab(tree)["adminGitEmail"] = "technician@school.example" }); result["ok"] != true {
		t.Fatal(result)
	}
	controllerOnly := assistantSettingsPlan(t, other, otherStore)
	if lines := controllerOnly.SettingsReviewLines(); controllerOnly.State != "ready" || len(lines) != 1 || !strings.Contains(lines[0], "Apply to controller") {
		t.Fatalf("controller-only plan: %+v", controllerOnly)
	}
}

func TestAssistantSavePlanWithoutSettingsChangeHasNoImpacts(t *testing.T) {
	w, store, _, _ := assistantSettingsFixture(t)
	if result := assistantFileCall(t, w, "write_file", map[string]any{"path": "course.nix", "content": "{ }\n"}); result["ok"] != true {
		t.Fatal(result)
	}
	plan := assistantSettingsPlan(t, w, store)
	if plan.State != "ready" || plan.SettingsImpacts == nil || len(plan.SettingsImpacts) != 0 || len(plan.SettingsReviewLines()) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
}

// assistantSettingsTamper puts a file in the overlay the way no tool can, to
// show that the review does not rely on the tool having checked it.
func assistantSettingsTamper(t *testing.T, w *AssistantWorkspace, data []byte, executable bool) {
	t.Helper()
	overlay, err := w.overlay()
	if err != nil {
		t.Fatal(err)
	}
	defer overlay.Close()
	if err := assistantWriteState(overlay, "lab-settings.json", data); err != nil {
		t.Fatal(err)
	}
	w.modes["lab-settings.json"] = executable
	encoded, _ := json.Marshal(w.modes)
	if err := assistantWriteState(w.root, "modes.json", encoded); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantSavePlanRefusesWhatANativeSettingsSaveRefuses(t *testing.T) {
	canonical := func(edit func(*domain.LabSettings)) []byte {
		settings := assistantSettingsBase()
		edit(&settings.Lab)
		data, err := domain.MarshalPublicLabSettings(settings)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for name, test := range map[string]struct {
		data       []byte
		executable bool
		want       string
	}{
		"network":    {canonical(func(lab *domain.LabSettings) { lab.NetworkBase = "10.1.0.0" }), false, domain.NetworkLockedMessage(20)},
		"controller": {canonical(func(lab *domain.LabSettings) { lab.MasterHostNumber = 100 }), false, domain.ControllerNumberLockedMessage(20)},
		"interface":  {canonical(func(lab *domain.LabSettings) { lab.InterfaceName = "eth0" }), false, "The proposal changes a setting the assistant may not change (lab.ifaceName)."},
		"dhcp":       {canonical(func(lab *domain.LabSettings) { lab.MasterDHCPIP = "192.0.2.11" }), false, "(lab.masterDhcpIp)"},
		"invalid":    {[]byte("{}"), false, "The proposed settings are not valid:"},
		"form":       {[]byte(strings.ReplaceAll(string(canonical(func(lab *domain.LabSettings) { lab.TimeZone = "UTC" })), "  ", "\t")), false, "not in the form Nixorium writes"},
		"mode":       {canonical(func(lab *domain.LabSettings) { lab.TimeZone = "UTC" }), true, "The proposal changes a protected path."},
	} {
		t.Run(name, func(t *testing.T) {
			w, store, repo, _ := assistantSettingsFixture(t)
			assistantSettingsTamper(t, w, test.data, test.executable)
			before := assistantGitSnapshot(t, repo)
			plan := planAssistantSave(t.Context(), store, w.Conversation.ID, assistantNoNix(t))
			if plan.State != "blocked" || !strings.Contains(plan.Message, test.want) || plan.ReviewToken != "" {
				t.Fatalf("plan: %+v", plan)
			}
			if saved := SaveAssistant(t.Context(), store, plan); saved.State != "blocked" {
				t.Fatalf("save: %+v", saved)
			}
			if resetFileDigest(before) != resetFileDigest(assistantGitSnapshot(t, repo)) {
				t.Fatal("a refused plan changed the repository")
			}
		})
	}
}

func TestAssistantClientAdvertisesSettingsCapability(t *testing.T) {
	// The header allowlist is exact: settings_v1 and settings_v2 pass, near misses do not.
	seen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Nixorium-Capabilities")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: final\ndata: {\"summary\":\"Ready\"}\n\n")
	}))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL + "/v1", Version: "3.8.0"}
	payload := domain.AssistantStepRequest{Context: &domain.AssistantContext{Nixorium: "3.8.0", Files: []string{"flake.nix"}, Capabilities: []string{"settings_v1", "SETTINGS_V1", "settings", "settings_v3", "settings_v1", "settings_v2", "settings_v1,prefetch_v1"}}, Messages: []domain.AssistantMessage{{Role: "user", Content: "Synthetic"}}}
	if err := client.Step(t.Context(), strings.Repeat("t", 43), assistantTestTurnID, payload, func(domain.AssistantEvent) {}); err != nil {
		t.Fatal(err)
	}
	if seen != "settings_v1,settings_v2" {
		t.Fatalf("header %q", seen)
	}
}

func TestAssistantSettingsAcceptsTheMenuSettings(t *testing.T) {
	for name, test := range map[string]struct {
		edit   func(lab map[string]any)
		want   func(lab *domain.LabSettings)
		field  string
		impact string
	}{
		"classroom view off":  {func(lab map[string]any) { lab["classroomView"] = false }, func(lab *domain.LabSettings) { off := false; lab.ClassroomView = &off }, "lab.classroomView", "classroom-view"},
		"computer count up":   {func(lab map[string]any) { lab["pcCount"] = 24 }, func(lab *domain.LabSettings) { lab.PCCount = 24 }, "lab.pcCount", "computers-added"},
		"computer count down": {func(lab map[string]any) { lab["pcCount"] = 12 }, func(lab *domain.LabSettings) { lab.PCCount = 12 }, "lab.pcCount", "computers-removed"},
		"teacher user":        {func(lab map[string]any) { lab["teacherUser"] = "docente" }, func(lab *domain.LabSettings) { lab.TeacherUser = "docente" }, "lab.teacherUser", "accounts"},
		"student user":        {func(lab map[string]any) { lab["studentUser"] = "alunno" }, func(lab *domain.LabSettings) { lab.StudentUser = "alunno" }, "lab.studentUser", "accounts"},
	} {
		t.Run(name, func(t *testing.T) {
			w, store, _, _ := assistantSettingsFixture(t)
			if result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { test.edit(assistantSettingsLab(tree)) }); result["ok"] != true {
				t.Fatal(result)
			}
			want := assistantSettingsBase()
			test.want(&want.Lab)
			if got := assistantCandidateSettings(t, w); !reflect.DeepEqual(got, want) {
				t.Fatalf("candidate %+v", got.Lab)
			}
			plan := assistantSettingsPlan(t, w, store)
			if plan.State != "ready" {
				t.Fatalf("plan: %+v", plan)
			}
			found := false
			for _, impact := range plan.SettingsImpacts {
				found = found || impact.Kind == test.impact
			}
			if !found {
				t.Fatalf("no %s impact: %+v", test.impact, plan.SettingsImpacts)
			}
		})
	}
}

func TestAssistantSettingsClassroomViewImpactIsTheMenuText(t *testing.T) {
	w, store, _, _ := assistantSettingsFixture(t)
	if result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { assistantSettingsLab(tree)["classroomView"] = false }); result["ok"] != true {
		t.Fatal(result)
	}
	plan := assistantSettingsPlan(t, w, store)
	if !slices.Contains(plan.SettingsReviewLines(), "Checked by Nixorium: "+domain.ClassroomViewImpact(false).Detail) {
		t.Fatalf("lines %q", plan.SettingsReviewLines())
	}
}

func TestAssistantSettingsRefusesInvalidMenuSettings(t *testing.T) {
	w, store, _, base := assistantSettingsFixture(t)
	for name, test := range map[string]struct {
		edit func(lab map[string]any)
		want string
	}{
		"count zero":       {func(lab map[string]any) { lab["pcCount"] = 0 }, "settings_invalid: lab.pcCount: must be between 1 and 253 in laboratory mode"},
		"count too many":   {func(lab map[string]any) { lab["pcCount"] = 254 }, "settings_invalid: lab.pcCount: must be between 1 and 253 in laboratory mode"},
		"count negative":   {func(lab map[string]any) { lab["pcCount"] = -3 }, "settings_invalid: lab.pcCount: must be between 1 and 253 in laboratory mode"},
		"count above host": {func(lab map[string]any) { lab["pcCount"] = 120 }, "settings_invalid: lab.masterHostNumber: must be greater than pcCount"},
		"count fraction":   {func(lab map[string]any) { lab["pcCount"] = 2.5 }, "settings_invalid: lab.pcCount must be an integer"},
		"count text":       {func(lab map[string]any) { lab["pcCount"] = "20" }, "settings_invalid: lab.pcCount must be an integer"},
		"view text":        {func(lab map[string]any) { lab["classroomView"] = "off" }, "settings_invalid: lab.classroomView must be true or false"},
		"teacher root":     {func(lab map[string]any) { lab["teacherUser"] = "root" }, "settings_invalid: lab.teacherUser: must not be root or admin"},
		"student bad":      {func(lab map[string]any) { lab["studentUser"] = "Bad Name" }, "settings_invalid: lab.studentUser: must be a valid Unix user name"},
		"same account":     {func(lab map[string]any) { lab["studentUser"] = "desk-teacher" }, "settings_invalid: lab.studentUser: must be different from teacherUser"},
	} {
		result := assistantSettingsModelWrite(t, w, func(tree map[string]any) { test.edit(assistantSettingsLab(tree)) })
		if result["ok"] == true || result["error"] != "settings_invalid" || !strings.HasPrefix(result["message"].(string), test.want) {
			t.Fatalf("%s: %v", name, result)
		}
		assistantSettingsUntouched(t, w, store, base)
	}
}
