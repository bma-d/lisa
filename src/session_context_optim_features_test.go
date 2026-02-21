package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectNestedCodexBypassQuoteGuard(t *testing.T) {
	detection := detectNestedCodexBypass("codex", "exec", "The string './lisa' appears in docs only.", "", "auto")
	if detection.AutoBypass {
		t.Fatalf("expected quote/doc mention to avoid bypass, got %+v", detection)
	}
}

func TestApplyNestedPolicyNestingIntentNested(t *testing.T) {
	detection, args, err := applyNestedPolicyToAgentArgs("codex", "exec", "No nesting requested here.", "", "auto", "nested")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !detection.AutoBypass || detection.Reason != "nesting_intent_nested" {
		t.Fatalf("unexpected detection: %+v", detection)
	}
	if !strings.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected bypass arg injected, got %q", args)
	}
}

func TestCmdSessionDetectNestedJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionDetectNested([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--nesting-intent", "nested",
			"--prompt", "No nesting requested here.",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected detect-nested success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload struct {
		NestedDetection nestedCodexDetection `json:"nestedDetection"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if !payload.NestedDetection.AutoBypass {
		t.Fatalf("expected autoBypass=true, got %+v", payload.NestedDetection)
	}
}

func TestCmdSessionSendJSONMin(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendText := tmuxSendTextFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendTextFn = origSendText
	})
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxSendTextFn = func(session, text string, enter bool) error { return nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{
			"--session", "lisa-send-json-min",
			"--text", "hello",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if payload["session"] != "lisa-send-json-min" || payload["ok"] != true {
		t.Fatalf("unexpected payload: %v", payload)
	}
	if _, exists := payload["enter"]; exists {
		t.Fatalf("json-min should omit enter field: %v", payload)
	}
}

func TestCmdSessionCaptureMarkersJSONMin(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "alpha\nDONE\nomega", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-cap-markers",
			"--project-root", t.TempDir(),
			"--raw",
			"--markers", "DONE,NOPE",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if _, ok := payload["capture"]; ok {
		t.Fatalf("marker mode should omit capture in json output: %v", payload)
	}
	found, _ := payload["foundMarkers"].([]any)
	missing, _ := payload["missingMarkers"].([]any)
	if len(found) != 1 || len(missing) != 1 {
		t.Fatalf("unexpected marker summary: %v", payload)
	}
}

func TestCmdSessionSnapshotJSONMin(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "active", SessionState: "in_progress"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "SNAP_MARK\nline2", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSnapshot([]string{
			"--session", "lisa-snapshot",
			"--project-root", t.TempDir(),
			"--delta-from", "0",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if payload["sessionState"] != "in_progress" || payload["nextOffset"].(float64) <= 0 {
		t.Fatalf("unexpected snapshot payload: %v", payload)
	}
}

func TestCmdSessionListStaleJSON(t *testing.T) {
	projectRoot := t.TempDir()
	active := "lisa-active"
	stale := "lisa-stale"
	if err := saveSessionMeta(projectRoot, active, sessionMeta{Session: active, ProjectRoot: projectRoot, Agent: "codex", Mode: "interactive", StartCmd: "echo", CreatedAt: "2026-02-21T00:00:00Z"}); err != nil {
		t.Fatalf("save active meta: %v", err)
	}
	if err := saveSessionMeta(projectRoot, stale, sessionMeta{Session: stale, ProjectRoot: projectRoot, Agent: "codex", Mode: "interactive", StartCmd: "echo", CreatedAt: "2026-02-21T00:00:01Z"}); err != nil {
		t.Fatalf("save stale meta: %v", err)
	}

	origList := tmuxListSessionsFn
	t.Cleanup(func() { tmuxListSessionsFn = origList })
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{active}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", projectRoot, "--stale", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if payload["staleCount"].(float64) != 1 {
		t.Fatalf("expected staleCount=1, got %v", payload)
	}
}

func TestCmdSessionExplainJSONMin(t *testing.T) {
	origCompute := computeSessionStatusFn
	origReadTail := readSessionEventTailFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		readSessionEventTailFn = origReadTail
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "idle", SessionState: "waiting_input", ClassificationReason: "interactive_idle_cpu"}, nil
	}
	readSessionEventTailFn = func(projectRoot, session string, max int) (sessionEventTail, error) {
		return sessionEventTail{Events: []sessionEvent{{At: "2026-02-21T00:00:00Z", Type: "snapshot", State: "waiting_input", Status: "idle", Reason: "interactive_idle_cpu"}}}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionExplain([]string{"--session", "lisa-explain", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v (%q)", err, stdout)
	}
	if payload["sessionState"] != "waiting_input" {
		t.Fatalf("unexpected payload: %v", payload)
	}
	if _, ok := payload["recent"].([]any); !ok {
		t.Fatalf("expected recent events in json-min payload: %v", payload)
	}
}

func TestCmdSessionMonitorIncludesNextOffset(t *testing.T) {
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: session, Status: "completed", SessionState: "completed"}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "ABC", nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{"--session", "lisa-monitor-offset", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"nextOffset":3`) {
		t.Fatalf("expected nextOffset in monitor payload, got %q", stdout)
	}
}

func TestCmdSkillsInstallProjectSamePathNoop(t *testing.T) {
	repoRoot := t.TempDir()
	skillDir := filepath.Join(repoRoot, "skills", "lisa")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{"--to", "project", "--project-path", repoRoot, "--repo-root", repoRoot, "--json"})
		if code != 0 {
			t.Fatalf("expected noop success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"noop":true`) {
		t.Fatalf("expected noop summary, got %q", stdout)
	}
}

func TestCmdSessionPreflightModelCheckHook(t *testing.T) {
	origProbe := sessionPreflightModelCheckFn
	t.Cleanup(func() { sessionPreflightModelCheckFn = origProbe })
	sessionPreflightModelCheckFn = func(agent, model string) sessionPreflightModelCheck {
		return sessionPreflightModelCheck{Agent: agent, Model: model, OK: false, Detail: "unsupported", ErrorCode: "preflight_model_not_supported"}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPreflight([]string{"--agent", "codex", "--model", "GPT-5.3-Codex-Spark", "--json"})
		if code == 0 {
			t.Fatalf("expected preflight failure due model probe")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"modelCheck"`) {
		t.Fatalf("expected modelCheck payload, got %q", stdout)
	}
}

func TestRunSmokePromptMatrixProbe(t *testing.T) {
	matrixPath := filepath.Join(t.TempDir(), "matrix.txt")
	content := strings.Join([]string{
		"bypass|Use ./lisa for child orchestration.",
		"full-auto|No nesting requested here.",
	}, "\n") + "\n"
	if err := os.WriteFile(matrixPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write matrix: %v", err)
	}

	origRun := runLisaSubcommandFn
	t.Cleanup(func() { runLisaSubcommandFn = origRun })
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		prompt := ""
		for i := 0; i < len(args); i++ {
			if args[i] == "--prompt" && i+1 < len(args) {
				prompt = args[i+1]
				break
			}
		}
		command := "codex exec 'No nesting requested here.' --full-auto"
		detection := nestedCodexDetection{AutoBypass: false, Reason: "no_nested_hint"}
		if strings.Contains(prompt, "./lisa") {
			command = "codex exec 'Use ./lisa for child orchestration.' --dangerously-bypass-approvals-and-sandbox"
			detection = nestedCodexDetection{AutoBypass: true, Reason: "prompt_contains_dot_slash_lisa", MatchedHint: "./lisa"}
		}
		payload := map[string]any{"command": command, "nestedDetection": detection}
		raw, _ := json.Marshal(payload)
		return string(raw), "", nil
	}

	probes, err := runSmokePromptMatrixProbe("/tmp/fake-lisa", t.TempDir(), matrixPath)
	if err != nil {
		t.Fatalf("expected matrix probe success, got %v", err)
	}
	if len(probes) != 2 {
		t.Fatalf("expected 2 probes, got %d", len(probes))
	}
	for i, probe := range probes {
		if !probe.Pass {
			t.Fatalf("probe %d should pass: %+v", i, probe)
		}
	}
}

func TestParseSmokePromptMatrixFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-matrix.txt")
	if err := os.WriteFile(path, []byte("invalid-line\n"), 0o644); err != nil {
		t.Fatalf("write matrix: %v", err)
	}
	_, err := parseSmokePromptMatrixFile(path)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "expected mode|prompt") {
		t.Fatalf("unexpected parse error: %v", err)
	}
}

func TestParseCaptureMarkersFlagRejectsEmptyMarker(t *testing.T) {
	_, err := parseCaptureMarkersFlag("A,,B")
	if err == nil {
		t.Fatalf("expected marker parse error")
	}
}

func TestBuildCaptureMarkerSummaryCounts(t *testing.T) {
	summary := buildCaptureMarkerSummary("A B A", []string{"A", "C"})
	if summary.Counts["A"] != 2 || summary.Matches["C"] {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestParseNestingIntent(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "auto", want: "auto", ok: true},
		{raw: "nested", want: "nested", ok: true},
		{raw: "neutral", want: "neutral", ok: true},
		{raw: "bad", ok: false},
	} {
		got, err := parseNestingIntent(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("parseNestingIntent(%q) unexpected err: %v", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("parseNestingIntent(%q) expected error", tc.raw)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("parseNestingIntent(%q)=%q want=%q", tc.raw, got, tc.want)
		}
	}
}

func TestCmdSessionListStaleJSONMinIncludesCounts(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-stale-min"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{Session: session, ProjectRoot: projectRoot, Agent: "codex", Mode: "interactive", StartCmd: "echo", CreatedAt: "2026-02-21T00:00:00Z"}); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	origList := tmuxListSessionsFn
	t.Cleanup(func() { tmuxListSessionsFn = origList })
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) { return []string{}, nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", projectRoot, "--stale", "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"staleCount":1`) || !strings.Contains(stdout, `"historicalCount":1`) {
		t.Fatalf("expected stale counts in json-min payload, got %q", stdout)
	}
}

func TestCmdSessionSmokeMatrixFileFailure(t *testing.T) {
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
		return "", "", fmt.Errorf("unexpected call: %v", args)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", root,
			"--levels", "1",
			"--matrix-file", matrixPath,
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected matrix assertion failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"smoke_prompt_matrix_assertion_failed"`) {
		t.Fatalf("expected matrix failure payload, got %q", stdout)
	}
}
