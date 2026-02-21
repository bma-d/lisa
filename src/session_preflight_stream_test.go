package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCmdSessionPreflightJSON(t *testing.T) {
	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })
	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPreflight([]string{"--project-root", t.TempDir(), "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		OK        bool `json:"ok"`
		Contracts []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse preflight JSON: %v (%q)", err, stdout)
	}
	if !payload.OK {
		t.Fatalf("expected preflight ok=true, got false: %s", stdout)
	}
	if len(payload.Contracts) == 0 {
		t.Fatalf("expected contract checks, got none")
	}
	for _, c := range payload.Contracts {
		if !c.OK {
			t.Fatalf("expected passing contract %q, got failure in payload: %s", c.Name, stdout)
		}
	}
}

func TestCmdSessionPreflightJSONEnvFailure(t *testing.T) {
	origLookPath := lookPathFn
	t.Cleanup(func() { lookPathFn = origLookPath })
	lookPathFn = func(file string) (string, error) {
		if file == "tmux" {
			return "", errors.New("missing")
		}
		return "/usr/bin/" + file, nil
	}

	stdout, _ := captureOutput(t, func() {
		code := cmdSessionPreflight([]string{"--json"})
		if code == 0 {
			t.Fatalf("expected failure when tmux is missing")
		}
	})
	if !strings.Contains(stdout, `"errorCode":"session_preflight_failed"`) {
		t.Fatalf("expected session_preflight_failed error code, got %q", stdout)
	}
}

func TestCmdSessionMonitorStreamJSONMin(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() { computeSessionStatusFn = origCompute })
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
			TodosDone:    2,
			TodosTotal:   2,
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-stream-json",
			"--project-root", t.TempDir(),
			"--max-polls", "1",
			"--poll-interval", "1",
			"--json-min",
			"--stream-json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("expected poll stream line + final line, got %d lines (%q)", len(lines), stdout)
	}

	var pollPayload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &pollPayload); err != nil {
		t.Fatalf("failed to parse poll payload: %v (%q)", err, lines[0])
	}
	if pollPayload["type"] != "poll" {
		t.Fatalf("expected first line to be poll payload, got %v", pollPayload)
	}

	var finalPayload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalPayload); err != nil {
		t.Fatalf("failed to parse final payload: %v (%q)", err, lines[len(lines)-1])
	}
	if finalPayload["finalState"] != "completed" {
		t.Fatalf("expected finalState completed, got %v", finalPayload)
	}
}

func TestCmdSessionCaptureJSONMinRaw(t *testing.T) {
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

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-min",
			"--project-root", t.TempDir(),
			"--raw",
			"--delta-from", "4",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.Contains(stdout, `"deltaMode"`) || strings.Contains(stdout, `"deltaFrom"`) {
		t.Fatalf("expected json-min payload to omit delta metadata fields, got %q", stdout)
	}
	if !strings.Contains(stdout, `"nextOffset":13`) {
		t.Fatalf("expected nextOffset in json-min payload, got %q", stdout)
	}
}

func TestCmdSessionCaptureJSONMinTranscript(t *testing.T) {
	origShouldUseTranscript := shouldUseTranscriptCaptureFn
	origCaptureTranscript := captureSessionTranscriptFn
	t.Cleanup(func() {
		shouldUseTranscriptCaptureFn = origShouldUseTranscript
		captureSessionTranscriptFn = origCaptureTranscript
	})
	shouldUseTranscriptCaptureFn = func(session, projectRoot string) bool { return true }
	captureSessionTranscriptFn = func(session, projectRoot string) (string, []transcriptMessage, error) {
		return "claude-123", []transcriptMessage{
			{Role: "user", Text: "hello"},
			{Role: "assistant", Text: "world"},
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-transcript-min",
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
	if !strings.Contains(stdout, `"messageCount":2`) {
		t.Fatalf("expected messageCount in json-min transcript payload, got %q", stdout)
	}
	if strings.Contains(stdout, `"messages"`) {
		t.Fatalf("expected json-min transcript payload to omit full messages array, got %q", stdout)
	}
}

func TestCmdAgentBuildCmdAcceptsProjectRoot(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdAgentBuildCmd([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", t.TempDir(),
			"--prompt", "ship it",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected build-cmd success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"projectRoot":`) {
		t.Fatalf("expected projectRoot in JSON payload, got %q", stdout)
	}
	if !strings.Contains(stdout, `"command":"codex exec`) {
		t.Fatalf("expected codex exec command in payload, got %q", stdout)
	}
}
