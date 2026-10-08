package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func templateResetFixture() TemplateResetPlan {
	files := map[string]TemplateFile{
		"nixorium-recovery.age": {Mode: "100644", Data: []byte("encrypted-recovery")},
		"lab-settings.json":     {Mode: "100644", Data: []byte("private-settings")},
		"flake.lock":            {Mode: "100644", Data: []byte("locked-inputs")},
		"keys/admin-ssh.pub":    {Mode: "100644", Data: []byte("public-key")},
		".gitignore":            {Mode: "100644", Data: []byte("secret-key\n")},
	}
	candidate := map[string]TemplateFile{}
	for name, file := range files {
		candidate[name] = file
	}
	return TemplateResetPlan{State: "ready", Repository: "/deployment", Revision: strings.Repeat("a", 40), UpstreamRevision: strings.Repeat("b", 40), Preset: SoftwarePreset{ID: "essential"}, Proposal: TemplateResetProposal{
		Revision: strings.Repeat("a", 40), UpstreamRevision: strings.Repeat("b", 40), Branch: "refs/heads/master", SourceIdentity: "identity", Original: files, Candidate: candidate,
	}}
}

func TestTemplateResetTokenBindsPrivateProposalAndReview(t *testing.T) {
	p := templateResetFixture()
	before, err := TemplateResetToken(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Proposal.Candidate["module.nix"] = TemplateFile{Mode: "100644", Data: []byte("new module")}
	after, err := TemplateResetToken(p)
	if err != nil || after == before {
		t.Fatal("candidate bytes are not bound")
	}
	p.Changes = []TemplateResetChange{{Path: "module.nix", Action: "remove"}}
	changed, err := TemplateResetToken(p)
	if err != nil || changed == after {
		t.Fatal("review summary is not bound")
	}
	data, err := json.Marshal(p)
	if err != nil || strings.Contains(string(data), "private-settings") || strings.Contains(string(data), "new module") {
		t.Fatal("public report exposed proposal bytes")
	}
}

func TestTemplateResetRejectsPreservationLossAndPrivateCollisions(t *testing.T) {
	for _, name := range []string{"nixorium-recovery.age", "lab-settings.json", "flake.lock", "keys/admin-ssh.pub", ".gitignore"} {
		t.Run(name, func(t *testing.T) {
			p := templateResetFixture()
			delete(p.Proposal.Candidate, name)
			if _, err := TemplateResetToken(p); err == nil {
				t.Fatal("allowed preserved file deletion")
			}
		})
	}
	for _, path := range []string{"new", "new/file", "new/file/private"} {
		p := templateResetFixture()
		p.Proposal.Candidate["new/file"] = TemplateFile{Mode: "100644"}
		p.Proposal.UntrackedPaths = []string{path}
		if _, err := TemplateResetToken(p); err == nil {
			t.Fatalf("allowed collision: %s", path)
		}
	}
}

func TestTemplateResetRejectsUnsafeTrees(t *testing.T) {
	for _, path := range []string{"../outside", "/absolute", ".git/config", "safe/../unsafe", "escape\x1b", "control\x07", "bad\\path"} {
		if ValidTemplatePath(path) {
			t.Fatalf("accepted %q", path)
		}
	}
	for _, target := range []string{"../../outside", "/outside", "../.git/config", "escape\x07"} {
		if err := ValidateTemplateFiles(map[string]TemplateFile{"sub/link": {Mode: "120000", Data: []byte(target)}}); err == nil {
			t.Fatalf("accepted target %q", target)
		}
	}
	if err := ValidateTemplateFiles(map[string]TemplateFile{"sub": {Mode: "120000", Data: []byte("real")}, "sub/child": {Mode: "100644"}}); err == nil {
		t.Fatal("accepted symlink ancestor")
	}
}
