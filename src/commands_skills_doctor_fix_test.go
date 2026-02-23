package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type skillsDoctorFixTestPayload struct {
	Fix     bool `json:"fix"`
	Targets []struct {
		Target   string `json:"target"`
		Status   string `json:"status"`
		Fixed    bool   `json:"fixed"`
		FixError string `json:"fixError"`
	} `json:"targets"`
}

type skillsDoctorErrorPayload struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode"`
	Error     string `json:"error"`
}

type skillsDoctorContractPayload struct {
	OK            bool `json:"ok"`
	ContractCheck bool `json:"contractCheck"`
	Targets       []struct {
		Target       string   `json:"target"`
		Status       string   `json:"status"`
		MissingFlags []string `json:"missingFlags"`
	} `json:"targets"`
}

func TestCmdSkillsDoctorFixSuccess(t *testing.T) {
	origHome := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHome
		BuildVersion = origVersion
	})
	BuildVersion = "dev"

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", lisaSkillName)
	writeSkillFixture(t, repoSkill, "2.0.0")

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexPath, err := defaultSkillInstallPath("codex")
	if err != nil {
		t.Fatalf("codex install path: %v", err)
	}
	claudePath, err := defaultSkillInstallPath("claude")
	if err != nil {
		t.Fatalf("claude install path: %v", err)
	}
	writeSkillFixture(t, codexPath, "1.0.0")
	writeSkillFixture(t, claudePath, "1.0.0")

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--fix", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	payload := decodeSkillsDoctorFixPayload(t, stdout)
	if !payload.Fix {
		t.Fatalf("expected fix=true in payload: %v", payload)
	}
	for _, target := range payload.Targets {
		if target.Status != "up_to_date" {
			t.Fatalf("expected target up_to_date after fix: %+v", target)
		}
		if !target.Fixed {
			t.Fatalf("expected fixed=true for updated target: %+v", target)
		}
		if target.FixError != "" {
			t.Fatalf("expected empty fixError on success: %+v", target)
		}
	}
}

func TestCmdSkillsDoctorFixFailurePreservesDestination(t *testing.T) {
	origHome := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHome
		BuildVersion = origVersion
	})
	BuildVersion = "dev"
	if os.Geteuid() == 0 {
		t.Skip("permission-denied fixture is not reliable when running as root")
	}

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", lisaSkillName)
	writeSkillFixture(t, repoSkill, "2.0.0")
	unreadable := filepath.Join(repoSkill, "data", "unreadable.bin")
	if err := os.WriteFile(unreadable, []byte("no-read"), 0o600); err != nil {
		t.Fatalf("write unreadable source fixture: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod unreadable source fixture: %v", err)
	}

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexPath, err := defaultSkillInstallPath("codex")
	if err != nil {
		t.Fatalf("codex install path: %v", err)
	}
	writeSkillFixture(t, codexPath, "1.0.0")

	beforeRaw, err := os.ReadFile(filepath.Join(codexPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("read before content: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--fix", "--json"})
		if code == 0 {
			t.Fatalf("expected failure when staged copy fails")
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	payload := decodeSkillsDoctorFixPayload(t, stdout)
	if !payload.Fix {
		t.Fatalf("expected fix=true in payload: %v", payload)
	}
	codex := findSkillsDoctorTarget(t, payload, "codex")
	if codex.FixError == "" {
		t.Fatalf("expected fixError for failed codex install: %+v", codex)
	}
	if codex.Fixed {
		t.Fatalf("expected fixed=false when fix failed: %+v", codex)
	}

	afterRaw, err := os.ReadFile(filepath.Join(codexPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("read after content: %v", err)
	}
	if string(afterRaw) != string(beforeRaw) {
		t.Fatalf("destination should be preserved on failed fix; before=%q after=%q", string(beforeRaw), string(afterRaw))
	}
}

func TestCmdSkillsDoctorFixNoopSkipsReleaseFetch(t *testing.T) {
	origHome := osUserHomeDirFn
	origVersion := BuildVersion
	origFetch := fetchReleaseSkillToTempDirFn
	t.Cleanup(func() {
		osUserHomeDirFn = origHome
		BuildVersion = origVersion
		fetchReleaseSkillToTempDirFn = origFetch
	})
	BuildVersion = "v9.9.9"

	fetchCalls := 0
	fetchRoot := t.TempDir()
	fetchSkillDir := filepath.Join(fetchRoot, "skills", lisaSkillName)
	if err := os.MkdirAll(fetchSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir fetch fallback skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fetchSkillDir, "SKILL.md"), []byte("version: 0.0.0\n"), 0o644); err != nil {
		t.Fatalf("write fetch fallback skill: %v", err)
	}
	fetchReleaseSkillToTempDirFn = func(version string) (string, error) {
		fetchCalls++
		return fetchSkillDir, nil
	}

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", lisaSkillName)
	writeSkillFixture(t, repoSkill, "7.0.0")

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexPath, err := defaultSkillInstallPath("codex")
	if err != nil {
		t.Fatalf("codex install path: %v", err)
	}
	claudePath, err := defaultSkillInstallPath("claude")
	if err != nil {
		t.Fatalf("claude install path: %v", err)
	}
	if _, err := copyDirReplace(repoSkill, codexPath); err != nil {
		t.Fatalf("copy codex fixture: %v", err)
	}
	if _, err := copyDirReplace(repoSkill, claudePath); err != nil {
		t.Fatalf("copy claude fixture: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--fix", "--json"})
		if code != 0 {
			t.Fatalf("expected no-op fix success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if fetchCalls != 0 {
		t.Fatalf("expected zero fetch calls for no-op --fix, got %d", fetchCalls)
	}

	payload := decodeSkillsDoctorFixPayload(t, stdout)
	if !payload.Fix {
		t.Fatalf("expected fix=true in payload: %v", payload)
	}
	for _, target := range payload.Targets {
		if target.Status != "up_to_date" {
			t.Fatalf("expected up_to_date target in no-op fix path: %+v", target)
		}
		if target.Fixed {
			t.Fatalf("expected fixed=false for no-op fix target: %+v", target)
		}
		if target.FixError != "" {
			t.Fatalf("expected empty fixError in no-op path: %+v", target)
		}
	}
}

func TestCmdSkillsDoctorMissingRepoRootValueJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--json", "--repo-root"})
		if code == 0 {
			t.Fatalf("expected missing --repo-root value to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var payload skillsDoctorErrorPayload
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, stdout)
	}
	if payload.OK {
		t.Fatalf("expected ok=false, got %+v", payload)
	}
	if payload.ErrorCode != "missing_flag_value" {
		t.Fatalf("expected missing_flag_value, got %q", payload.ErrorCode)
	}
	if !strings.Contains(payload.Error, "missing value for --repo-root") {
		t.Fatalf("expected missing value message, got %q", payload.Error)
	}
}

func TestCmdSkillsDoctorContractCheckIncludesMissingFlags(t *testing.T) {
	origHome := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHome
		BuildVersion = origVersion
	})
	BuildVersion = "dev"

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", lisaSkillName)
	writeSkillFixture(t, repoSkill, "2.0.0")

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexPath, err := defaultSkillInstallPath("codex")
	if err != nil {
		t.Fatalf("codex install path: %v", err)
	}
	claudePath, err := defaultSkillInstallPath("claude")
	if err != nil {
		t.Fatalf("claude install path: %v", err)
	}
	writeSkillFixture(t, codexPath, "2.0.0")
	writeSkillFixture(t, claudePath, "2.0.0")

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--contract-check", "--json"})
		if code == 0 {
			t.Fatalf("expected drift result to fail when flags are missing")
		}
	})
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	var payload skillsDoctorContractPayload
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, stdout)
	}
	if payload.OK {
		t.Fatalf("expected ok=false for missing flags, got true")
	}
	if !payload.ContractCheck {
		t.Fatalf("expected contractCheck=true, got false")
	}
	if len(payload.Targets) == 0 {
		t.Fatalf("expected doctor targets in payload")
	}
	for _, target := range payload.Targets {
		if target.Status != "outdated" {
			t.Fatalf("expected target outdated, got %+v", target)
		}
		if len(target.MissingFlags) == 0 {
			t.Fatalf("expected missingFlags populated, got %+v", target)
		}
		expectedMissing := []string{
			"capabilities:--json",
			"session guard:--machine-policy",
			"session route:--profile",
			"session contract-check:--project-root",
		}
		found := map[string]bool{}
		for _, miss := range target.MissingFlags {
			found[miss] = true
		}
		for _, miss := range expectedMissing {
			if !found[miss] {
				t.Fatalf("expected drift entry %q, got %+v", miss, target.MissingFlags)
			}
		}
	}
}

func writeSkillFixture(t *testing.T, skillDir, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(skillDir, "data"), 0o755); err != nil {
		t.Fatalf("mkdir skill fixture: %v", err)
	}
	skillBody := strings.Join([]string{
		"---",
		"version: " + version,
		"---",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatalf("write SKILL.md fixture: %v", err)
	}
	commandsBody := strings.Join(requiredSkillCommandNames(), "\n") + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "data", "commands.md"), []byte(commandsBody), 0o644); err != nil {
		t.Fatalf("write commands fixture: %v", err)
	}
}

func decodeSkillsDoctorFixPayload(t *testing.T, raw string) skillsDoctorFixTestPayload {
	t.Helper()
	var payload skillsDoctorFixTestPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode payload: %v (%q)", err, raw)
	}
	return payload
}

func findSkillsDoctorTarget(t *testing.T, payload skillsDoctorFixTestPayload, target string) struct {
	Target   string `json:"target"`
	Status   string `json:"status"`
	Fixed    bool   `json:"fixed"`
	FixError string `json:"fixError"`
} {
	t.Helper()
	for _, item := range payload.Targets {
		if item.Target == target {
			return item
		}
	}
	t.Fatalf("target not found: %s", target)
	return struct {
		Target   string `json:"target"`
		Status   string `json:"status"`
		Fixed    bool   `json:"fixed"`
		FixError string `json:"fixError"`
	}{}
}
