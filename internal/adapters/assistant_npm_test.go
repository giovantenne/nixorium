package adapters

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/npmartifact"
	"golang.org/x/sys/unix"
)

func TestAssistantNpmFilesPreflightResumeAndCarryover(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	files := map[string]string{"npm/pi/manifest.json": `{"schemaVersion":1,"artifactID":"synthetic"}`, "npm/pi/default.nix": "{ pkgs }: pkgs.hello\n"}
	if err := assistantWriteState(w.root, assistantEvaluationReceiptName, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := assistantNpmFiles(w, files); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantReadStateLimit(w.root, assistantEvaluationReceiptName, assistantEvaluationReceiptLimit); !errors.Is(err, unix.ENOENT) {
		t.Fatal("npm publication kept the evaluation receipt", err)
	}
	fresh, err := store.Open(w.Conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	for path, want := range files {
		got, err := fresh.raw(path)
		if err != nil || string(got) != want {
			t.Fatal(path, string(got), err)
		}
	}
	if err := assistantNpmFiles(fresh, map[string]string{"npm/pi/default.nix": "changed", "npm/pi/new.nix": "new"}); err == nil {
		t.Fatal("must refuse existing artifact atomically")
	}
	if _, err := fresh.raw("npm/pi/new.nix"); err == nil {
		t.Fatal("partial publication")
	}
	if err := assistantNpmFiles(fresh, map[string]string{"npm/new/manifest.json": strings.Repeat("x", 512*1024+1), "npm/new/default.nix": "new"}); err == nil {
		t.Fatal("must reject oversized batch")
	}
	if _, err := fresh.raw("npm/new/default.nix"); err == nil {
		t.Fatal("oversized batch partially published")
	}
	next, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := store.CarryOverProposal(w.Conversation.ID, next.ID); err != nil || count != 2 {
		t.Fatal(count, err)
	}
}

func TestAssistantNpmArtifactCleanupKeepsSavedAndUnsaved(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	roots, err := assistantNpmRootDirectory(store)
	if err != nil {
		t.Fatal(err)
	}
	roots.Close()
	unavailable := t.TempDir()
	for _, id := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		path := filepath.Join(store.Directory, "npm-artifacts", id)
		if err := os.Symlink("/nix/store/synthetic-output", path); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(assistantNpmArtifactRecord{repo, "npm/" + id[:1] + "/manifest.json", id, 100})
		if err := os.WriteFile(path+".json", data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	id := strings.Repeat("d", 64)
	rootPath := filepath.Join(store.Directory, "npm-artifacts", id)
	if err := os.Symlink("/nix/store/synthetic-output", rootPath); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(assistantNpmArtifactRecord{unavailable, "npm/d/manifest.json", id, 100})
	if err := os.WriteFile(rootPath+".json", metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.Write("npm/a/manifest.json", `{"artifactID":"`+strings.Repeat("a", 64)+`"}`, false); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "npm/b"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "npm/b/manifest.json"), []byte(`{"artifactID":"`+strings.Repeat("b", 64)+`"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := resetGit(context.Background(), repo, nil, 4096, "reset", "--hard", w.Conversation.Base); err != nil {
			t.Error(err)
		}
	})
	workspaceTestGit(t, repo, "add", "npm/b/manifest.json")
	workspaceTestGit(t, repo, "commit", "-m", "saved artifact")
	if err := store.cleanupNpmArtifacts(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		id   string
		keep bool
	}{{"a", true}, {"b", true}, {"c", false}, {"d", true}} {
		_, err := os.Lstat(filepath.Join(store.Directory, "npm-artifacts", strings.Repeat(v.id, 64)))
		if (err == nil) != v.keep {
			t.Fatal(v, err)
		}
	}
}

// Opt-in public npm qualification. Never executed in normal checks, and never
// calls a model or activates a lab. CLI code runs only in the generated sandbox.
func TestAssistantNpmPublicPreparation(t *testing.T) {
	name := os.Getenv("NIXORIUM_TEST_NPM_PACKAGE")
	if name == "" {
		t.Skip("set public package and clean deployment fixture explicitly")
	}
	store := AssistantWorkspaceStore{Directory: t.TempDir()}
	if err := os.Chmod(store.Directory, 0700); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateConversation(os.Getenv("NIXORIUM_TEST_NPM_REPOSITORY"))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	data, _ := json.Marshal(map[string]string{"package": name, "version": "latest", "id": "public-fixture"})
	result := prepareAssistantNpmWithRunner(t.Context(), store, workspace, domain.AssistantToolCall{Name: "prepare_npm", Arguments: data}, func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
		stdout, stderr, err := runBoundedCommand(ctx, limit, name, args...)
		if err != nil {
			t.Log(assistantEvaluationError(stderr))
		}
		return stdout, stderr, err
	})
	t.Log(result)
	var value struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal([]byte(result), &value) != nil || !value.OK {
		t.Fatal("public npm preparation failed")
	}
	if output := os.Getenv("NIXORIUM_TEST_NPM_OUTPUT"); output != "" {
		if !strings.HasPrefix(output, "/tmp/") {
			t.Fatal("qualification output must stay under /tmp")
		}
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"manifest.json", "package.json", "package-lock.json", "default.nix", "module.nix"} {
			data, err := workspace.raw("npm/public-fixture/" + file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, file), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAssistantNpmGeneratedFilesNormalSave(t *testing.T) {
	w, store, repo := assistantEvaluationFixture(t, false)
	m := npmartifact.Manifest{SchemaVersion: 1, Package: "fixture", Version: "1.0.0", NodeRuntime: "/nix/store/fixture-node", ArtifactID: strings.Repeat("d", 64), Bins: map[string]string{"fixture": "bin.js"}, Dependencies: []npmartifact.Dependency{}, OmittedOptional: []string{}}
	nix, module := npmartifact.Render(m)
	manifest, _ := json.Marshal(m)
	files := map[string]string{"npm/fixture/package.json": "{\"dependencies\":{\"fixture\":\"1.0.0\"}}", "npm/fixture/package-lock.json": "{\"lockfileVersion\":3}", "npm/fixture/manifest.json": string(manifest), "npm/fixture/default.nix": nix, "npm/fixture/module.nix": module}
	if err := assistantNpmFiles(w, files); err != nil {
		t.Fatal(err)
	}
	plan := PlanAssistantSave(t.Context(), store, w.Conversation.ID)
	if plan.State != "ready" || !plan.Evaluation.OK || len(plan.Added) != 5 {
		t.Fatal(plan)
	}
	root, err := openWorkspaceRoot(repo, unix.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	result := commitConfigurationTree(t.Context(), root, plan.Proposal, plan.ReviewToken, assistantCommit, nil)
	root.Close()
	if result.State != "saved" || result.RecoveryRequired {
		t.Fatal(result)
	}
	for p, want := range files {
		data, err := os.ReadFile(filepath.Join(repo, p))
		if err != nil || string(data) != want {
			t.Fatal(p, err)
		}
	}
}

func TestAssistantNpmOfflineLifecycleSandbox(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_NPM_SANDBOX") != "1" {
		t.Skip("explicit disposable Nix sandbox qualification")
	}
	stage := t.TempDir()
	secret := filepath.Join(stage, "host-secret")
	if err := os.WriteFile(secret, []byte("synthetic-host-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	quoted, _ := json.Marshal(secret)
	hook := `const fs=require('node:fs'),net=require('node:net');if(fs.existsSync(` + string(quoted) + `)||process.env.NIXORIUM_NPM_SENTINEL)throw Error('host exposed');try{fs.writeFileSync('/outside','bad');throw Error('write escaped')}catch(e){if(e.message==='write escaped')throw e}const s=net.connect({host:'127.0.0.1',port:` + strconv.Itoa(port) + `});s.on('connect',()=>process.exit(1));s.on('error',()=>{fs.writeFileSync('installed-marker','isolated')});s.setTimeout(500,()=>process.exit(1));`
	pkg, _ := json.Marshal(map[string]any{"name": "synthetic-cli", "version": "1.0.0", "bin": map[string]string{"fixture": "cli.js"}, "scripts": map[string]string{"postinstall": "node hook.js"}})
	cli := `#!/usr/bin/env node
const fs=require('node:fs');if(fs.readFileSync(__dirname+'/installed-marker','utf8')!=='isolated')throw Error('hook skipped');console.log('1.0.0');
`
	var data bytes.Buffer
	zip := gzip.NewWriter(&data)
	writer := tar.NewWriter(zip)
	for name, content := range map[string]string{"package.json": string(pkg), "cli.js": cli, "hook.js": hook} {
		if err := writer.WriteHeader(&tar.Header{Name: "package/" + name, Mode: 0755, Typeflag: tar.TypeReg, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		writer.Write([]byte(content))
	}
	writer.Close()
	zip.Close()
	hash := sha512.Sum512(data.Bytes())
	dep := npmartifact.Dependency{Path: "node_modules/synthetic-cli", Version: "1.0.0", URL: "https://registry.npmjs.org/synthetic-cli/-/synthetic-cli-1.0.0.tgz", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(hash[:])}
	if _, err := (&npmartifact.ArchiveBudget{}).Inspect(data.Bytes(), dep, true); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(stage, "fixture.tgz")
	os.WriteFile(file, data.Bytes(), 0600)
	if _, _, err := runBoundedCommand(t.Context(), 8192, "nix", "store", "add-file", "--hash-algo", "sha512", "--name", npmartifact.TarballName(dep), file); err != nil {
		t.Fatal(err)
	}
	repository := os.Getenv("NIXORIUM_TEST_NPM_REPOSITORY")
	ref, err := rawDeploymentFlakeReference(repository)
	if err != nil {
		t.Fatal(err)
	}
	expression := "let f = builtins.getFlake " + assistantNixQuote(ref) + "; in f.inputs.nixpkgs.legacyPackages.x86_64-linux.nodejs"
	result, _, err := runBoundedCommand(t.Context(), 8192, "nix", "build", "--offline", "--no-link", "--json", "--impure", "--expr", expression)
	if err != nil {
		t.Fatal(err)
	}
	var outputs []struct {
		Outputs map[string]string `json:"outputs"`
	}
	json.Unmarshal([]byte(result), &outputs)
	m := npmartifact.Manifest{Package: "synthetic-cli", Version: "1.0.0", NodeRuntime: outputs[0].Outputs["out"], Bins: map[string]string{"fixture": "cli.js"}, Dependencies: []npmartifact.Dependency{dep}}
	nix, _ := npmartifact.Render(m)
	rootPackage := map[string]any{"name": "nixorium-npm-baseline", "version": "1.0.0", "private": true, "dependencies": map[string]string{"synthetic-cli": "1.0.0"}}
	root, _ := json.Marshal(rootPackage)
	lock, _ := json.Marshal(map[string]any{"name": "nixorium-npm-baseline", "version": "1.0.0", "lockfileVersion": 3, "packages": map[string]any{"": rootPackage, "node_modules/synthetic-cli": map[string]any{"version": "1.0.0", "resolved": dep.URL, "integrity": dep.Integrity, "hasInstallScript": true, "bin": map[string]string{"fixture": "cli.js"}}}})
	for name, data := range map[string][]byte{"package.json": root, "package-lock.json": lock, "default.nix": []byte(nix)} {
		if err := os.WriteFile(filepath.Join(stage, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NIXORIUM_NPM_SENTINEL", "host-secret")
	expression = "let f = builtins.getFlake " + assistantNixQuote(ref) + "; pkgs = f.inputs.nixpkgs.legacyPackages.x86_64-linux; in import " + assistantNixQuote(stage+"/default.nix") + " { inherit pkgs; }"
	if _, _, err := runBoundedCommand(t.Context(), 32768, "nix", "build", "--offline", "--no-link", "--json", "--impure", "--option", "sandbox", "true", "--expr", expression); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(secret)
	if string(got) != "synthetic-host-secret" {
		t.Fatal("host file changed")
	}
}

func TestAssistantNpmSavedPiOfflineAndResetFixture(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	fixture := os.Getenv("NIXORIUM_TEST_NPM_OUTPUT")
	if fixture == "" {
		t.Skip("explicit prepared public Pi fixture required")
	}
	data, err := os.ReadFile(filepath.Join(fixture, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest npmartifact.Manifest
	if json.Unmarshal(data, &manifest) != nil || manifest.Package != "@mariozechner/pi-coding-agent" {
		t.Fatal("unexpected fixture")
	}
	nix, module := npmartifact.Render(manifest)
	for p, text := range map[string]string{"default.nix": nix, "module.nix": module} {
		if err := os.WriteFile(filepath.Join(fixture, p), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ref, err := rawDeploymentFlakeReference(os.Getenv("NIXORIUM_TEST_NPM_REPOSITORY"))
	if err != nil {
		t.Fatal(err)
	}
	preamble := "let f = builtins.getFlake " + assistantNixQuote(ref) + "; pkgs = f.inputs.nixpkgs.legacyPackages.x86_64-linux; "
	expression := preamble + "in [ (import " + assistantNixQuote(fixture+"/default.nix") + " { inherit pkgs; }) pkgs.fd pkgs.ripgrep ]"
	output, _, err := runBoundedCommand(ctx, 65536, "nix", "build", "--offline", "--impure", "--no-link", "--json", "--expr", expression)
	if err != nil {
		t.Fatal(err)
	}
	var outputs []struct {
		Outputs map[string]string `json:"outputs"`
	}
	if json.Unmarshal([]byte(output), &outputs) != nil || len(outputs) != 3 {
		t.Fatal(output)
	}
	artifact, fd, rg := "", "", ""
	for _, item := range outputs {
		p := item.Outputs["out"]
		switch {
		case strings.Contains(p, "-nixorium-npm-baseline-"):
			artifact = p
		case strings.Contains(p, "-fd-"):
			fd = p
		case strings.Contains(p, "-ripgrep-"):
			rg = p
		}
	}
	if artifact == "" || fd == "" || rg == "" {
		t.Fatal(output)
	}
	moduleExpression := preamble + "s = import (pkgs.path + \"/nixos/lib/eval-config.nix\") { system = \"x86_64-linux\"; modules = [ " + assistantNixQuote(fixture+"/module.nix") + " { _module.args.labSettings.studentUser = \"student\"; system.stateVersion = \"26.05\"; users.users.student = { isNormalUser = true; group = \"users\"; }; } ]; }; c = s.config.systemd.services.nixorium-student-npm-baseline; "
	output, _, err = runBoundedCommand(ctx, 65536, "nix", "eval", "--impure", "--json", "--expr", moduleExpression+"in { inherit (c) after before requires restartIfChanged; inherit (c.serviceConfig) User ProtectHome BindPaths; inherit (c.unitConfig) RefuseManualStart; script = c.serviceConfig.ExecStart; }")
	if err != nil {
		t.Fatal(err)
	}
	var unit struct {
		After             []string
		Before            []string
		Requires          []string
		RestartIfChanged  bool
		User              string
		ProtectHome       string
		BindPaths         []string
		RefuseManualStart bool
		Script            string
	}
	if json.Unmarshal([]byte(output), &unit) != nil || unit.User != "student" || unit.RestartIfChanged || !unit.RefuseManualStart || !slices.Contains(unit.After, "home-reset.service") || !slices.Contains(unit.Before, "display-manager.service") || !slices.Contains(unit.Requires, "home-reset.service") {
		t.Fatal(output)
	}
	if _, _, err := runBoundedCommand(ctx, 65536, "nix", "build", "--offline", "--impure", "--no-link", "--expr", moduleExpression+"in pkgs.runCommand \"npm-baseline-module-check\" { nativeBuildInputs = [ pkgs.shellcheck ]; } '' shellcheck ${c.serviceConfig.ExecStart}; cp ${c.serviceConfig.ExecStart} $out ''"); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "student")
	paths := manifest.NodeRuntime + "/bin:" + fd + "/bin:" + rg + "/bin"
	run := func(program string, args ...string) string {
		flags := []string{"--unshare-all", "--die-with-parent", "--new-session", "--ro-bind", "/nix/store", "/nix/store", "--bind", home, "/home/student", "--tmpfs", "/tmp", "--proc", "/proc", "--dev", "/dev", "--clearenv", "--setenv", "HOME", "/home/student", "--setenv", "PATH", paths, "--setenv", "PI_OFFLINE", "1", "--chdir", "/home/student", "--remount-ro", "/", program}
		flags = append(flags, args...)
		output, stderr, err := runBoundedCommand(ctx, 32768, os.Getenv("NIXORIUM_NPM_BWRAP"), flags...)
		if err != nil {
			t.Fatal(err)
		}
		return output + stderr
	}
	for iteration := 0; iteration < 2; iteration++ {
		// This simulates the neutral home produced by reset; it does not claim a
		// VM reboot or physical GNOME login qualification.
		if err := os.RemoveAll(home); err != nil {
			t.Fatal(err)
		}
		prefix := filepath.Join(home, ".local/npm")
		if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		unrelated := filepath.Join(prefix, "bin/unrelated-seed")
		os.WriteFile(unrelated, []byte("keep"), 0644)
		run(unit.Script)
		got, _ := os.ReadFile(unrelated)
		if string(got) != "keep" {
			t.Fatal("unrelated seed replaced")
		}
		if got := strings.TrimSpace(run("/home/student/.local/npm/bin/pi", "--version")); got != manifest.Version {
			t.Fatal(got)
		}
		run("/home/student/.local/npm/bin/pi", "--offline", "--mode", "rpc", "--no-session")
		js := `const p=require('/home/student/.local/npm/lib/node_modules/@silvia-odwyer/photon-node');const image=new p.PhotonImage(new Uint8Array([255,0,0,255]),1,1);if(image.get_width()!==1)throw Error('WASM failed');image.free();const tools=await import('/home/student/.local/npm/lib/node_modules/@mariozechner/pi-coding-agent/dist/utils/tools-manager.js');globalThis.fetch=()=>{throw Error('unexpected download')};if(await tools.ensureTool('fd',true)!=='fd'||await tools.ensureTool('rg',true)!=='rg')throw Error('runtime programs missing');`
		run(manifest.NodeRuntime+"/bin/node", "--input-type=module", "-e", `import {createRequire} from 'node:module';const require=createRequire(import.meta.url);`+js)
		if err := os.WriteFile(filepath.Join(prefix, "lib/node_modules/@mariozechner/pi-coding-agent/package.json"), []byte(`{"version":"student-modified"}`), 0600); err != nil {
			t.Fatal("baseline not student-writable", err)
		}
	}
	t.Logf("Offline rebuilt %s; module ordering/ShellCheck, two fresh-home restores, Pi RPC startup, photon WASM and pinned fd/rg passed", artifact)
}
