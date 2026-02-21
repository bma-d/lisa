package app

import (
	"encoding/json"
	"testing"
)

func TestCmdSessionNameJSONOutput(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionName([]string{
			"--agent", "codex",
			"--mode", "interactive",
			"--project-root", t.TempDir(),
			"--tag", "audit",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected session name json success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse session name json: %v (%q)", err, stdout)
	}
	if payload["agent"] != "codex" {
		t.Fatalf("unexpected agent value: %v", payload["agent"])
	}
	if _, ok := payload["session"].(string); !ok {
		t.Fatalf("expected session string in payload: %v", payload)
	}
}

func TestCmdSessionListJSONOutput(t *testing.T) {
	origList := tmuxListSessionsFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
	})
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", t.TempDir(), "--json"})
		if code != 0 {
			t.Fatalf("expected session list json success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload struct {
		Sessions []string `json:"sessions"`
		Count    int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse list json: %v (%q)", err, stdout)
	}
	if payload.Count != 2 || len(payload.Sessions) != 2 {
		t.Fatalf("unexpected list payload: %+v", payload)
	}
}

func TestCmdSessionExistsJSONOutput(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
	})
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionExists([]string{
			"--session", "lisa-missing",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected missing session exit 1, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse exists json: %v (%q)", err, stdout)
	}
	if payload.Exists {
		t.Fatalf("expected exists=false payload")
	}
}

func TestCmdSessionKillAndKillAllJSONOutput(t *testing.T) {
	origHas := tmuxHasSessionFn
	origKill := tmuxKillSessionFn
	origList := tmuxListSessionsFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxKillSessionFn = origKill
		tmuxListSessionsFn = origList
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxKillSessionFn = func(session string) error { return nil }
	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		return []string{"lisa-a", "lisa-b"}, nil
	}

	projectRoot := t.TempDir()
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionKill([]string{
			"--session", "lisa-a",
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected kill json success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var killPayload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(stdout), &killPayload); err != nil {
		t.Fatalf("failed to parse kill json: %v (%q)", err, stdout)
	}
	if !killPayload.OK {
		t.Fatalf("expected kill ok payload")
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdSessionKillAll([]string{
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected kill-all json success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var killAllPayload struct {
		OK     bool `json:"ok"`
		Killed int  `json:"killed"`
		Total  int  `json:"total"`
	}
	if err := json.Unmarshal([]byte(stdout), &killAllPayload); err != nil {
		t.Fatalf("failed to parse kill-all json: %v (%q)", err, stdout)
	}
	if !killAllPayload.OK || killAllPayload.Killed != 2 || killAllPayload.Total != 2 {
		t.Fatalf("unexpected kill-all payload: %+v", killAllPayload)
	}
}
