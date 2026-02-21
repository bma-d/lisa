package app

import (
	"encoding/json"
	"testing"
)

func TestCmdSessionTreeActiveOnlyFiltersInactiveSessions(t *testing.T) {
	projectRoot := t.TempDir()
	inactive := "lisa-tree-inactive"
	active := "lisa-tree-active"

	if err := saveSessionMeta(projectRoot, inactive, sessionMeta{
		Session:     inactive,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo inactive",
		CreatedAt:   "2026-02-21T00:00:00Z",
	}); err != nil {
		t.Fatalf("save inactive meta failed: %v", err)
	}
	if err := saveSessionMeta(projectRoot, active, sessionMeta{
		Session:       active,
		ParentSession: inactive,
		Agent:         "codex",
		Mode:          "interactive",
		ProjectRoot:   projectRoot,
		StartCmd:      "echo active",
		CreatedAt:     "2026-02-21T00:00:01Z",
	}); err != nil {
		t.Fatalf("save active meta failed: %v", err)
	}

	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return session == active }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTree([]string{
			"--project-root", projectRoot,
			"--active-only",
			"--flat",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected active-only tree success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		ActiveOnly bool `json:"activeOnly"`
		NodeCount  int  `json:"nodeCount"`
		Rows       []struct {
			Session string `json:"session"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse tree json: %v (%q)", err, stdout)
	}
	if !payload.ActiveOnly {
		t.Fatalf("expected activeOnly=true payload")
	}
	if payload.NodeCount != 1 || len(payload.Rows) != 1 || payload.Rows[0].Session != active {
		t.Fatalf("unexpected active-only payload: %+v", payload)
	}
}

func TestCmdSessionMonitorJSONMinOutput(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "completed",
			SessionState: "completed",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-json-min",
			"--project-root", t.TempDir(),
			"--poll-interval", "1",
			"--max-polls", "1",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected json-min monitor success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse json-min monitor output: %v (%q)", err, stdout)
	}
	if payload["session"] != "lisa-monitor-json-min" || payload["finalState"] != "completed" || payload["exitReason"] != "completed" {
		t.Fatalf("unexpected json-min payload: %v", payload)
	}
	if _, ok := payload["todosDone"]; ok {
		t.Fatalf("json-min payload should not include todosDone: %v", payload)
	}
}

func TestCmdSessionSpawnDetectNestedDryRunJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", t.TempDir(),
			"--prompt", "Use ./lisa for child orchestration",
			"--dry-run",
			"--detect-nested",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected spawn dry-run success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse spawn dry-run json: %v (%q)", err, stdout)
	}
	detection, ok := payload["nestedDetection"].(map[string]any)
	if !ok {
		t.Fatalf("expected nestedDetection payload, got %v", payload)
	}
	if detection["autoBypass"] != true || detection["reason"] != "prompt_contains_dot_slash_lisa" {
		t.Fatalf("unexpected nested detection payload: %v", detection)
	}
	if detection["effectiveBypass"] != true {
		t.Fatalf("expected effectiveBypass=true, got %v", detection)
	}
}

func TestCmdSessionMonitorJSONValidationFailureIncludesErrorCode(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-error-json",
			"--expect", "marker",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected validation failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse error payload: %v (%q)", err, stdout)
	}
	if payload["errorCode"] != "expect_marker_requires_until_marker" {
		t.Fatalf("unexpected errorCode: %v", payload)
	}
}

func TestCmdSkillsInstallJSONFailureIncludesErrorCode(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsInstall([]string{
			"--to", "project",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected skills install validation failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse error payload: %v (%q)", err, stdout)
	}
	if payload["errorCode"] != "skills_destination_resolve_failed" {
		t.Fatalf("unexpected errorCode: %v", payload)
	}
}
