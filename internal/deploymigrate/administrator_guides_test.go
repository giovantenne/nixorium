package deploymigrate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var guidesOnly = registryWith("administrator-guides")

var topicGuides = []string{"CLASSROOM.md", "INSTALLATION.md", "SOFTWARE.md", "MAINTENANCE.md"}

// releasedGuide is the long administrator guide of 3.14.0.
func releasedGuide(t *testing.T) string {
	t.Helper()
	guide, err := releasedTemplateFile(t, "v3.14.0", "README.md")
	if err != nil {
		missingMigrationFixture(t, "no Git history with release tags")
	}
	return guide
}

func TestTheGuidesReplaceTheLongReadmeAndAreCreatedBesideIt(t *testing.T) {
	root := treeWith(t, map[string]string{"README.md": releasedGuide(t), "lab-settings.json": "{}\n"})
	report, err := Run(root, "3.15.0", guidesOnly)
	if err != nil || len(report.Migrations) != 1 || report.Migrations[0].State != Applied {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if got := report.Migrations[0].Files; !slices.Equal(got, append([]string{"README.md"}, topicGuides...)) {
		t.Fatalf("files = %v", got)
	}
	template := filepath.Join("..", "..", "templates", "site")
	for _, name := range append([]string{"README.md"}, topicGuides...) {
		want, err := os.ReadFile(filepath.Join(template, name))
		if err != nil {
			t.Fatal(err)
		}
		if read(t, root, name) != string(want) {
			t.Errorf("%s is not this release's guide", name)
		}
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || info.Mode().Perm() != 0o644 {
			t.Errorf("%s mode = %v, %v", name, info, err)
		}
	}
	again, err := Run(root, "3.15.0", guidesOnly)
	if err != nil || len(again.Migrations) != 0 {
		t.Fatalf("second run = %+v, %v", again, err)
	}
}

func TestEveryCommittedFormOfTheGuideIsRecognised(t *testing.T) {
	guide := releasedGuide(t)
	root := treeWith(t, map[string]string{"README.md": guide})
	if outcome := administratorGuides(Tree{Root: root}); outcome.State != Applied {
		t.Fatalf("3.14.0 guide: %+v", outcome)
	}
	if len(releasedAdministratorGuides) < 90 {
		t.Fatalf("only %d released forms", len(releasedAdministratorGuides))
	}
}

func TestAnEditedGuideOrAnOwnTopicFileTouchesNothing(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"edited readme":    {"README.md": "# Our lab\n"},
		"own classroom.md": {"README.md": releasedGuide(t), "CLASSROOM.md": "our notes\n"},
		"no readme":        {"lab-settings.json": "{}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			root := treeWith(t, files)
			outcome := administratorGuides(Tree{Root: root})
			if outcome.State != Manual || !strings.Contains(outcome.Detail, "changed by hand or missing") {
				t.Fatalf("outcome = %+v", outcome)
			}
			for path, want := range files {
				if read(t, root, path) != want {
					t.Errorf("%s was changed", path)
				}
			}
			for _, guide := range topicGuides {
				if _, found := files[guide]; found {
					continue
				}
				if _, err := os.Stat(filepath.Join(root, guide)); !os.IsNotExist(err) {
					t.Errorf("%s was created: %v", guide, err)
				}
			}
		})
	}
}

func TestAFailedWriteRemovesTheGuidesAlreadyCreated(t *testing.T) {
	guide := releasedGuide(t)
	root := treeWith(t, map[string]string{"README.md": guide})
	failWriteOf(t, "SOFTWARE.md")
	outcome := administratorGuides(Tree{Root: root})
	if outcome.State != Manual || !strings.Contains(outcome.Detail, "Nothing was changed") {
		t.Fatalf("outcome = %+v", outcome)
	}
	if read(t, root, "README.md") != guide {
		t.Fatal("README.md was left changed")
	}
	for _, name := range topicGuides {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Errorf("%s was left behind: %v", name, err)
		}
	}
}
