package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCmdSessionListJSONMinOutput(t *testing.T) {
	origList := tmuxListSessionsFn
	t.Cleanup(func() { tmuxListSessionsFn = origList })
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", t.TempDir(), "--json-min"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse output: %v (%q)", err, stdout)
	}
	if payload["count"] != float64(2) {
		t.Fatalf("unexpected count: %v", payload["count"])
	}
	if _, ok := payload["projectRoot"]; ok {
		t.Fatalf("json-min should omit projectRoot: %v", payload)
	}
}

func TestCmdSessionStatusJSONMinOutput(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "active",
			SessionState: "in_progress",
			TodosDone:    1,
			TodosTotal:   3,
			WaitEstimate: 42,
			Agent:        "codex",
			Mode:         "interactive",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{
			"--session", "lisa-status-min",
			"--project-root", t.TempDir(),
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
		t.Fatalf("failed to parse output: %v (%q)", err, stdout)
	}
	if payload["session"] != "lisa-status-min" || payload["sessionState"] != "in_progress" {
		t.Fatalf("unexpected payload: %v", payload)
	}
	if _, ok := payload["agent"]; ok {
		t.Fatalf("json-min should omit agent: %v", payload)
	}
}

func TestCmdSessionTreeJSONMinOutput(t *testing.T) {
	projectRoot := t.TempDir()
	parent := "lisa-tree-min-parent"
	child := "lisa-tree-min-child"

	if err := saveSessionMeta(projectRoot, parent, sessionMeta{
		Session:     parent,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo parent",
		CreatedAt:   "2026-02-21T00:00:00Z",
	}); err != nil {
		t.Fatalf("save parent meta failed: %v", err)
	}
	if err := saveSessionMeta(projectRoot, child, sessionMeta{
		Session:       child,
		ParentSession: parent,
		Agent:         "codex",
		Mode:          "interactive",
		ProjectRoot:   projectRoot,
		StartCmd:      "echo child",
		CreatedAt:     "2026-02-21T00:00:01Z",
	}); err != nil {
		t.Fatalf("save child meta failed: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTree([]string{
			"--project-root", projectRoot,
			"--flat",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		NodeCount int `json:"nodeCount"`
		Rows      []struct {
			Session       string `json:"session"`
			ParentSession string `json:"parentSession"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse output: %v (%q)", err, stdout)
	}
	if payload.NodeCount != 2 || len(payload.Rows) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestCmdSessionCaptureDeltaFrom(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "one\ntwo\nthree", nil
	}

	projectRoot := t.TempDir()

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-delta",
			"--project-root", projectRoot,
			"--raw",
			"--delta-from", "4",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"capture":"two\nthree"`) {
		t.Fatalf("expected offset delta capture, got %q", stdout)
	}
	if !strings.Contains(stdout, `"nextOffset":13`) {
		t.Fatalf("expected nextOffset in payload, got %q", stdout)
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-delta",
			"--project-root", projectRoot,
			"--raw",
			"--delta-from", "@9999999999",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"capture":""`) {
		t.Fatalf("expected empty timestamp delta capture, got %q", stdout)
	}
}

func TestCmdSessionCaptureDeltaRequiresRaw(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-delta",
			"--delta-from", "10",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"delta_requires_raw_capture"`) {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

func TestCmdSessionMonitorTimeoutFinalStatusDeterministic(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "waiting_input",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-timeout",
			"--max-polls", "1",
			"--poll-interval", "1",
			"--stop-on-waiting", "false",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected timeout exit 2, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalStatus":"timeout"`) {
		t.Fatalf("expected deterministic timeout finalStatus, got %q", stdout)
	}
}

func TestCmdSessionSpawnNestedPolicyModes(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", t.TempDir(),
			"--prompt", "Use ./lisa only.",
			"--nested-policy", "off",
			"--dry-run",
			"--detect-nested",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.Contains(stdout, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected policy off to avoid bypass arg: %q", stdout)
	}
	if !strings.Contains(stdout, `"reason":"nested_policy_off"`) {
		t.Fatalf("expected nested_policy_off reason, got %q", stdout)
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", t.TempDir(),
			"--prompt", "No nesting requested here.",
			"--nested-policy", "force",
			"--dry-run",
			"--detect-nested",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("expected force policy to enable bypass arg: %q", stdout)
	}
	if strings.Contains(stdout, "--full-auto") {
		t.Fatalf("expected force policy to omit full-auto: %q", stdout)
	}
}

func TestCmdSessionSpawnDryRunWithModel(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--model", "GPT-5.3-Codex-Spark",
			"--project-root", t.TempDir(),
			"--prompt", "Run checks",
			"--dry-run",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `--model 'GPT-5.3-Codex-Spark'`) {
		t.Fatalf("expected dry-run command to include codex model, got %q", stdout)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse output json: %v (%q)", err, stdout)
	}
	if payload["model"] != "GPT-5.3-Codex-Spark" {
		t.Fatalf("expected model in dry-run payload, got %v", payload["model"])
	}
}

func TestCmdSessionSpawnModelRejectsClaude(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "claude",
			"--mode", "interactive",
			"--model", "GPT-5.3-Codex-Spark",
			"--project-root", t.TempDir(),
			"--dry-run",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected model/agent validation failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"invalid_model_configuration"`) {
		t.Fatalf("expected invalid_model_configuration, got %q", stdout)
	}
}

func TestCmdSessionKillMissingJSONDoesNotWriteStderr(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionKill([]string{
			"--session", "lisa-missing-json-kill",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected failure for missing session")
		}
	})
	if stderr != "" {
		t.Fatalf("expected no stderr in json mode, got %q", stderr)
	}
	if !strings.Contains(stdout, `"found":false`) {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

func TestRunSmokePromptStyleProbeValidation(t *testing.T) {
	orig := runLisaSubcommandFn
	t.Cleanup(func() { runLisaSubcommandFn = orig })

	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		return `{"command":"codex exec 'Use ./lisa' --dangerously-bypass-approvals-and-sandbox","nestedDetection":{"autoBypass":true,"reason":"prompt_contains_dot_slash_lisa"}}`, "", nil
	}
	probe, err := runSmokePromptStyleProbe("/bin/lisa", "/tmp/project", "dot-slash")
	if err != nil {
		t.Fatalf("expected probe success, got %v", err)
	}
	if probe == nil || probe.Detection.AutoBypass != true {
		t.Fatalf("unexpected probe: %+v", probe)
	}

	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		return `{"command":"codex exec 'neutral' --full-auto","nestedDetection":{"autoBypass":true,"reason":"unexpected"}}`, "", nil
	}
	_, err = runSmokePromptStyleProbe("/bin/lisa", "/tmp/project", "neutral")
	if err == nil {
		t.Fatalf("expected probe mismatch error")
	}
}
