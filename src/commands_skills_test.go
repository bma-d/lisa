package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdSkillsRouter(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := cmdSkills(nil)
		if code == 0 {
			t.Fatalf("expected skills without subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "usage: lisa skills <subcommand>") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	_, stderr = captureOutput(t, func() {
		code := cmdSkills([]string{"unknown"})
		if code == 0 {
			t.Fatalf("expected unknown skills subcommand to fail")
		}
	})
	if !strings.Contains(stderr, "unknown skills subcommand: unknown") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSkillsSyncFromCodexAndInstallToProject(t *testing.T) {
	origHomeFn := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() { osUserHomeDirFn = origHomeFn })
	t.Cleanup(func() { BuildVersion = origVersion })
	BuildVersion = "dev"

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }

	codexSkillDir := filepath.Join(home, ".codex", "skills", lisaSkillName)
	if err := os.MkdirAll(filepath.Join(codexSkillDir, "examples"), 0o755); err != nil {
		t.Fatalf("mkdir codex skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexSkillDir, "SKILL.md"), []byte("version=1\n"), 0o644); err != nil {
		t.Fatalf("write codex SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexSkillDir, "examples", "README.md"), []byte("example\n"), 0o644); err != nil {
		t.Fatalf("write codex nested file: %v", err)
	}

	repoRoot := t.TempDir()
	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsSync([]string{"--from", "codex", "--repo-root", repoRoot, "--json"})
		if code != 0 {
			t.Fatalf("expected sync success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected sync stderr: %q", stderr)
	}

	var syncPayload skillsCopySummary
	if err := json.Unmarshal([]byte(stdout), &syncPayload); err != nil {
		t.Fatalf("decode sync payload: %v (%q)", err, stdout)
	}
	repoSkillFile := filepath.Join(repoRoot, "skills", lisaSkillName, "SKILL.md")
	raw, err := os.ReadFile(repoSkillFile)
	if err != nil {
		t.Fatalf("read synced repo skill: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "version=1" {
		t.Fatalf("unexpected synced repo skill content: %q", string(raw))
	}
	if syncPayload.Files < 2 {
		t.Fatalf("expected at least 2 files copied, got %+v", syncPayload)
	}

	if err := os.WriteFile(repoSkillFile, []byte("version=2\n"), 0o644); err != nil {
		t.Fatalf("update repo skill file: %v", err)
	}

	projectRoot := t.TempDir()
	stdout, stderr = captureOutput(t, func() {
		code := cmdSkillsInstall([]string{
			"--to", "project",
			"--project-path", projectRoot,
			"--repo-root", repoRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected install success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected install stderr: %q", stderr)
	}

	var installPayload skillsCopySummary
	if err := json.Unmarshal([]byte(stdout), &installPayload); err != nil {
		t.Fatalf("decode install payload: %v (%q)", err, stdout)
	}

	projectSkillFile := filepath.Join(projectRoot, "skills", lisaSkillName, "SKILL.md")
	projectRaw, err := os.ReadFile(projectSkillFile)
	if err != nil {
		t.Fatalf("read installed project skill: %v", err)
	}
	if strings.TrimSpace(string(projectRaw)) != "version=2" {
		t.Fatalf("expected propagated update in project skill, got %q", string(projectRaw))
	}
	if installPayload.Files < 2 {
		t.Fatalf("expected install payload with copied files, got %+v", installPayload)
	}
}

func TestCmdSkillsInstallProjectRequiresProjectPath(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{"--to", "project"})
		if code == 0 {
			t.Fatalf("expected missing --project-path to fail")
		}
	})
	if !strings.Contains(stderr, "--project-path is required when --to project") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSkillsInstallDefaultInstallsAllAvailableTargets(t *testing.T) {
	origHomeFn := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHomeFn
		BuildVersion = origVersion
	})
	BuildVersion = "dev"

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir codex root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir claude root: %v", err)
	}

	repoRoot := t.TempDir()
	repoSkillDir := filepath.Join(repoRoot, "skills", lisaSkillName)
	if err := os.MkdirAll(repoSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir repo skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkillDir, "SKILL.md"), []byte("default-install\n"), 0o644); err != nil {
		t.Fatalf("write repo skill file: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{"--repo-root", repoRoot, "--json"})
		if code != 0 {
			t.Fatalf("expected default install success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload skillsInstallBatchSummary
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, stdout)
	}
	if len(payload.Installs) != 2 {
		t.Fatalf("expected 2 install targets, got %+v", payload)
	}

	codexRaw, err := os.ReadFile(filepath.Join(home, ".codex", "skills", lisaSkillName, "SKILL.md"))
	if err != nil {
		t.Fatalf("read codex installed file: %v", err)
	}
	claudeRaw, err := os.ReadFile(filepath.Join(home, ".claude", "skills", lisaSkillName, "SKILL.md"))
	if err != nil {
		t.Fatalf("read claude installed file: %v", err)
	}
	if strings.TrimSpace(string(codexRaw)) != "default-install" {
		t.Fatalf("unexpected codex content: %q", string(codexRaw))
	}
	if strings.TrimSpace(string(claudeRaw)) != "default-install" {
		t.Fatalf("unexpected claude content: %q", string(claudeRaw))
	}
}

func TestCmdSkillsInstallDefaultRequiresAvailableTargets(t *testing.T) {
	origHomeFn := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHomeFn
		BuildVersion = origVersion
	})
	BuildVersion = "dev"

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }

	repoRoot := t.TempDir()
	repoSkillDir := filepath.Join(repoRoot, "skills", lisaSkillName)
	if err := os.MkdirAll(repoSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir repo skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkillDir, "SKILL.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write repo skill file: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{"--repo-root", repoRoot})
		if code == 0 {
			t.Fatalf("expected default install to fail without ~/.codex or ~/.claude")
		}
	})
	if !strings.Contains(stderr, "no default install targets found") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestResolveSkillsInstallSourceUsesRepoForDevBuild(t *testing.T) {
	origVersion := BuildVersion
	origFetch := fetchReleaseSkillToTempDirFn
	t.Cleanup(func() {
		BuildVersion = origVersion
		fetchReleaseSkillToTempDirFn = origFetch
	})
	BuildVersion = "dev"

	fetchCalled := false
	fetchReleaseSkillToTempDirFn = func(version string) (string, error) {
		fetchCalled = true
		return "", nil
	}

	repoRoot := t.TempDir()
	path, cleanup, err := resolveSkillsInstallSource(repoRoot)
	if err != nil {
		t.Fatalf("resolve source failed: %v", err)
	}
	cleanup()
	if fetchCalled {
		t.Fatalf("release fetch should not be called for dev build")
	}
	want := filepath.Join(repoRoot, "skills", lisaSkillName)
	if path != want {
		t.Fatalf("unexpected source path: got %q want %q", path, want)
	}
}

func TestCmdSkillsInstallUsesGitHubSourceForTaggedBuild(t *testing.T) {
	origVersion := BuildVersion
	origFetch := fetchReleaseSkillToTempDirFn
	t.Cleanup(func() {
		BuildVersion = origVersion
		fetchReleaseSkillToTempDirFn = origFetch
	})
	BuildVersion = "v2.6.0"

	releaseSkillRoot := t.TempDir()
	releaseSkillDir := filepath.Join(releaseSkillRoot, "skills", lisaSkillName)
	if err := os.MkdirAll(releaseSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir release skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(releaseSkillDir, "SKILL.md"), []byte("from-github\n"), 0o644); err != nil {
		t.Fatalf("write release skill file: %v", err)
	}

	fetchCalled := false
	fetchReleaseSkillToTempDirFn = func(version string) (string, error) {
		fetchCalled = true
		if version != "v2.6.0" {
			t.Fatalf("unexpected version passed to fetcher: %q", version)
		}
		return releaseSkillDir, nil
	}

	projectRoot := t.TempDir()
	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{
			"--to", "project",
			"--project-path", projectRoot,
			"--repo-root", filepath.Join(t.TempDir(), "missing-repo-root"),
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected release install success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !fetchCalled {
		t.Fatalf("expected release fetch path to be used")
	}

	var payload skillsCopySummary
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, stdout)
	}
	installed := filepath.Join(projectRoot, "skills", lisaSkillName, "SKILL.md")
	raw, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "from-github" {
		t.Fatalf("expected installed release content, got %q", string(raw))
	}
	if payload.Source != releaseSkillDir {
		t.Fatalf("unexpected payload source: %q", payload.Source)
	}
}

func TestReleaseRefCandidates(t *testing.T) {
	got := releaseRefCandidates("2.7.1")
	if len(got) != 3 || got[0] != "2.7.1" || got[1] != "v2.7.1" || got[2] != "main" {
		t.Fatalf("unexpected ref candidates: %v", got)
	}

	got = releaseRefCandidates("v2.7.1")
	if len(got) != 3 || got[0] != "v2.7.1" || got[1] != "2.7.1" || got[2] != "main" {
		t.Fatalf("unexpected v-prefixed candidates: %v", got)
	}
}
