package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdSkillsDoctorDeepDetectsContentDrift(t *testing.T) {
	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", "lisa")
	if err := os.MkdirAll(filepath.Join(repoSkill, "data"), 0o755); err != nil {
		t.Fatalf("mkdir repo skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkill, "SKILL.md"), []byte("---\nversion: 1.0.0\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkill, "data", "commands.md"), []byte(strings.Join(requiredSkillCommandNames(), "\n")), 0o644); err != nil {
		t.Fatalf("write commands: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoSkill, "data", "validation.md"), []byte("baseline"), 0o644); err != nil {
		t.Fatalf("write validation: %v", err)
	}

	home := t.TempDir()
	origHome := osUserHomeDirFn
	t.Cleanup(func() { osUserHomeDirFn = origHome })
	osUserHomeDirFn = func() (string, error) { return home, nil }

	codexPath, _ := defaultSkillInstallPath("codex")
	claudePath, _ := defaultSkillInstallPath("claude")
	if _, err := copyDirReplace(repoSkill, codexPath); err != nil {
		t.Fatalf("copy codex: %v", err)
	}
	if _, err := copyDirReplace(repoSkill, claudePath); err != nil {
		t.Fatalf("copy claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexPath, "data", "validation.md"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("mutate codex validation: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--deep", "--json"})
		if code == 0 {
			t.Fatalf("expected deep drift failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		Deep    bool `json:"deep"`
		Targets []struct {
			Target string `json:"target"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse payload: %v (%q)", err, stdout)
	}
	if !payload.Deep {
		t.Fatalf("expected deep=true: %v", payload)
	}
	codexOutdated := false
	for _, target := range payload.Targets {
		if target.Target == "codex" && target.Status == "outdated" && strings.Contains(target.Detail, "content drift") {
			codexOutdated = true
		}
	}
	if !codexOutdated {
		t.Fatalf("expected codex content drift: %v", payload.Targets)
	}
}

func TestCmdSessionTreeWithStateJSONMin(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-tree-state"
	meta := sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo",
		CreatedAt:   "2026-02-22T00:00:00Z",
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "completed", SessionState: "completed"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTree([]string{"--project-root", projectRoot, "--with-state", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"rows"`) || !strings.Contains(stdout, `"sessionState":"completed"`) {
		t.Fatalf("expected with-state rows payload, got %q", stdout)
	}
}

func TestCmdSessionHandoffDeltaFromJSONMin(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-handoff-delta"
	eventsPath := sessionEventsFile(projectRoot, session)
	lines := []string{
		`{"at":"t1","type":"snapshot","session":"lisa-handoff-delta","state":"in_progress","status":"active","reason":"r1","poll":1}`,
		`{"at":"t2","type":"snapshot","session":"lisa-handoff-delta","state":"in_progress","status":"active","reason":"r2","poll":2}`,
		`{"at":"t3","type":"snapshot","session":"lisa-handoff-delta","state":"waiting_input","status":"idle","reason":"r3","poll":3}`,
	}
	if err := os.WriteFile(eventsPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}

	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress", ClassificationReason: "heartbeat_fresh"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{"--session", session, "--project-root", projectRoot, "--delta-from", "1", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"deltaCount":2`) || !strings.Contains(stdout, `"nextDeltaOffset":3`) {
		t.Fatalf("expected delta fields, got %q", stdout)
	}
}

func TestCmdSessionMonitorEmitHandoffStream(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "completed", SessionState: "completed", ClassificationReason: "done_file_exit_zero"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{"--session", "lisa-monitor-handoff", "--stream-json", "--emit-handoff", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"type":"poll"`) || !strings.Contains(stdout, `"type":"handoff"`) {
		t.Fatalf("expected poll+handoff stream events, got %q", stdout)
	}
}

func TestCmdSessionDetectNestedRewrite(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionDetectNested([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "Use lisa inside of lisa inside as well.",
			"--rewrite",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"rewrites"`) || !strings.Contains(stdout, "./lisa") {
		t.Fatalf("expected rewrite suggestions, got %q", stdout)
	}
}

func TestCmdSessionContextPackStrategyDefaults(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-context-strategy"
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress", ClassificationReason: "heartbeat_fresh"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionContextPack([]string{"--for", session, "--project-root", projectRoot, "--strategy", "terse", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"strategy":"terse"`) || !strings.Contains(stdout, `"tokenBudget":400`) {
		t.Fatalf("expected terse defaults, got %q", stdout)
	}
}

func TestCmdSessionRouteEmitRunbook(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{"--goal", "nested", "--emit-runbook", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"runbook"`) || !strings.Contains(stdout, `"spawn"`) {
		t.Fatalf("expected runbook payload, got %q", stdout)
	}
}

func TestCmdSessionCaptureSummaryJSON(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-cap-summary"
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Repeat("long-line ", 200), nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--raw",
			"--summary",
			"--token-budget", "10",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"summary"`) || !strings.Contains(stdout, `"truncated":true`) {
		t.Fatalf("expected bounded summary payload, got %q", stdout)
	}
}

func TestCmdSessionSmokeReportMin(t *testing.T) {
	root := t.TempDir()
	matrixPath := filepath.Join(root, "matrix.txt")
	if err := os.WriteFile(matrixPath, []byte("bypass|No nesting requested here.\n"), 0o644); err != nil {
		t.Fatalf("write matrix: %v", err)
	}

	origRun := runLisaSubcommandFn
	origExec := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExec
	})
	osExecutableFn = func() (string, error) { return "/tmp/fake-lisa", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) >= 2 && args[0] == "session" && args[1] == "spawn" {
			payload := map[string]any{
				"command":         "codex exec 'No nesting requested here.' --full-auto",
				"nestedDetection": nestedCodexDetection{AutoBypass: false, Reason: "no_nested_hint"},
			}
			raw, _ := json.Marshal(payload)
			return string(raw), "", nil
		}
		return "", "", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", root,
			"--levels", "1",
			"--matrix-file", matrixPath,
			"--report-min",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected smoke failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"smoke_prompt_matrix_assertion_failed"`) || strings.Contains(stdout, `"sessions"`) {
		t.Fatalf("expected compact report-min payload, got %q", stdout)
	}
}

func TestCmdSessionListPrunePreviewJSONMin(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-stale-prune"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		ProjectRoot: projectRoot,
		Agent:       "codex",
		Mode:        "interactive",
		StartCmd:    "echo",
		CreatedAt:   "2026-02-22T00:00:00Z",
	}); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	origList := tmuxListSessionsFn
	t.Cleanup(func() { tmuxListSessionsFn = origList })
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) { return []string{}, nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", projectRoot, "--stale", "--prune-preview", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"prunePreview"`) || !strings.Contains(stdout, `"pruneCmd"`) {
		t.Fatalf("expected prune preview payload, got %q", stdout)
	}
}
