package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCmdSessionMonitorAdaptivePollUsesHeartbeatAwareSleep(t *testing.T) {
	origCompute := computeSessionStatusFn
	origSleep := monitorSleepFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		monitorSleepFn = origSleep
	})

	sleepCalls := make([]time.Duration, 0, 2)
	monitorSleepFn = func(duration time.Duration) {
		sleepCalls = append(sleepCalls, duration)
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if pollCount == 1 {
			return sessionStatus{
				Session:            session,
				Status:             "active",
				SessionState:       "in_progress",
				HeartbeatAge:       1,
				HeartbeatFreshSecs: 8,
				Signals: statusSignals{
					HeartbeatFresh: true,
				},
			}, nil
		}
		return sessionStatus{
			Session:      session,
			Status:       "completed",
			SessionState: "completed",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-adaptive-poll",
			"--project-root", t.TempDir(),
			"--max-polls", "2",
			"--poll-interval", "4",
			"--adaptive-poll",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected monitor success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"finalState":"completed"`) {
		t.Fatalf("expected completed payload, got %q", stdout)
	}
	if len(sleepCalls) != 1 {
		t.Fatalf("expected one sleep call, got %d", len(sleepCalls))
	}
	if sleepCalls[0] != 2*time.Second {
		t.Fatalf("expected adaptive 2s sleep, got %s", sleepCalls[0])
	}
}

func TestCmdSessionMonitorDefaultPollIntervalRemainsStable(t *testing.T) {
	origCompute := computeSessionStatusFn
	origSleep := monitorSleepFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		monitorSleepFn = origSleep
	})

	sleepCalls := make([]time.Duration, 0, 2)
	monitorSleepFn = func(duration time.Duration) {
		sleepCalls = append(sleepCalls, duration)
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if pollCount == 1 {
			return sessionStatus{
				Session:      session,
				Status:       "active",
				SessionState: "in_progress",
			}, nil
		}
		return sessionStatus{
			Session:      session,
			Status:       "completed",
			SessionState: "completed",
		}, nil
	}

	_, _ = captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-default-poll",
			"--project-root", t.TempDir(),
			"--max-polls", "2",
			"--poll-interval", "4",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected monitor success, got %d", code)
		}
	})
	if len(sleepCalls) != 1 {
		t.Fatalf("expected one sleep call, got %d", len(sleepCalls))
	}
	if sleepCalls[0] != 4*time.Second {
		t.Fatalf("expected default 4s sleep, got %s", sleepCalls[0])
	}
}

func TestCmdSessionCaptureStripBannerWithKeepNoise(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Join([]string{
			"╭───────────────────────────────╮",
			"│ Codex CLI                     │",
			"│ model: gpt-5-codex            │",
			"│ approval: never               │",
			"╰───────────────────────────────╯",
			"mcp: keep-noise-line",
			"work item",
		}, "\n"), nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-strip-banner",
			"--project-root", t.TempDir(),
			"--raw",
			"--keep-noise",
			"--strip-banner",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected capture success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	capture := mapStringValue(payload, "capture")
	if strings.Contains(strings.ToLower(capture), "codex") || strings.Contains(strings.ToLower(capture), "model:") {
		t.Fatalf("expected banner chrome stripped, got %q", capture)
	}
	if !strings.Contains(capture, "mcp: keep-noise-line") {
		t.Fatalf("expected keep-noise line preserved, got %q", capture)
	}
	if !strings.Contains(capture, "work item") {
		t.Fatalf("expected content line preserved, got %q", capture)
	}
}

func TestCmdSessionCaptureKeepNoiseDefaultPreservedWithoutStripBanner(t *testing.T) {
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return strings.Join([]string{
			"╭───────────────────────────────╮",
			"│ Codex CLI                     │",
			"│ model: gpt-5-codex            │",
			"╰───────────────────────────────╯",
			"work item",
		}, "\n"), nil
	}

	stdout, _ := captureOutput(t, func() {
		code := cmdSessionCapture([]string{
			"--session", "lisa-capture-default-banner",
			"--project-root", t.TempDir(),
			"--raw",
			"--keep-noise",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected capture success, got %d", code)
		}
	})
	payload := parseJSONMap(t, stdout)
	capture := mapStringValue(payload, "capture")
	if !strings.Contains(capture, "Codex CLI") {
		t.Fatalf("expected banner retained without --strip-banner, got %q", capture)
	}
}

func TestFilterCaptureNoiseStripsBannerChromeForSharedCapturePaths(t *testing.T) {
	input := strings.Join([]string{
		"╭───────────────────────────────╮",
		"│ Codex CLI                     │",
		"│ model: gpt-5-codex            │",
		"│ approval: never               │",
		"╰───────────────────────────────╯",
		"mcp: startup chatter",
		"real output",
	}, "\n")
	filtered := filterCaptureNoise(input)
	if strings.Contains(strings.ToLower(filtered), "codex cli") {
		t.Fatalf("expected codex banner removed, got %q", filtered)
	}
	if strings.Contains(filtered, "mcp: startup chatter") {
		t.Fatalf("expected mcp noise removed, got %q", filtered)
	}
	if !strings.Contains(filtered, "real output") {
		t.Fatalf("expected real output retained, got %q", filtered)
	}
}

func TestCmdSessionPacketDeltaJSONUsesCursorAndFieldDeltas(t *testing.T) {
	projectRoot := t.TempDir()
	cursorFile := filepath.Join(projectRoot, "packet.delta.cursor.json")
	session := "lisa-packet-delta-json"

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	reason := "reason-a"
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: reason,
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "line one", nil
	}

	stdout1, stderr1 := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected first packet delta success, got %d", code)
		}
	})
	if stderr1 != "" {
		t.Fatalf("unexpected stderr: %q", stderr1)
	}
	payload1 := parseJSONMap(t, stdout1)
	if payload1["deltaCount"].(float64) == 0 {
		t.Fatalf("expected non-zero initial delta count, got %v", payload1["deltaCount"])
	}
	if _, ok := payload1["summary"]; ok {
		t.Fatalf("expected field-level delta payload only, got %v", payload1)
	}

	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected second packet delta success, got %d", code)
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	payload2 := parseJSONMap(t, stdout2)
	if payload2["deltaCount"].(float64) != 0 {
		t.Fatalf("expected zero delta count on unchanged snapshot, got %v", payload2["deltaCount"])
	}

	reason = "reason-b"
	stdout3, stderr3 := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected third packet delta success, got %d", code)
		}
	})
	if stderr3 != "" {
		t.Fatalf("unexpected stderr: %q", stderr3)
	}
	payload3 := parseJSONMap(t, stdout3)
	if payload3["deltaCount"].(float64) == 0 {
		t.Fatalf("expected changed delta count after reason update, got %v", payload3["deltaCount"])
	}
	delta := payload3["delta"].(map[string]any)
	changed := delta["changed"].([]any)
	foundReason := false
	for _, item := range changed {
		change := item.(map[string]any)
		if mapStringValue(change, "field") == "reason" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected reason field change, got %v", changed)
	}
}

func TestCmdSessionPacketDeltaJSONRequiresCursorFile(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", "lisa-packet-delta-missing-cursor",
			"--delta-json",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected missing cursor-file error")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"errorCode":"cursor_file_required_for_delta_json"`) {
		t.Fatalf("expected cursor_file_required_for_delta_json, got %q", stdout)
	}
}

func TestCmdSessionPacketDeltaJSONAcceptsLegacyNumericCursorFile(t *testing.T) {
	projectRoot := t.TempDir()
	cursorFile := filepath.Join(projectRoot, "packet.legacy.cursor")
	session := "lisa-packet-delta-legacy-cursor"
	if err := os.WriteFile(cursorFile, []byte("41\n"), 0o644); err != nil {
		t.Fatalf("write legacy cursor: %v", err)
	}

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "legacy_cursor_probe",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "packet delta legacy cursor", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected delta-json success with legacy cursor file, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["deltaCount"].(float64) == 0 {
		t.Fatalf("expected initial delta entries, got %v", payload["deltaCount"])
	}
	updatedRaw, err := os.ReadFile(cursorFile)
	if err != nil {
		t.Fatalf("read updated cursor: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(updatedRaw)), "{") {
		t.Fatalf("expected cursor upgraded to JSON object, got %q", string(updatedRaw))
	}
}

func TestCmdSessionPacketCursorFileRoundTripsBetweenDeltaModes(t *testing.T) {
	projectRoot := t.TempDir()
	cursorFile := filepath.Join(projectRoot, "packet.roundtrip.cursor")
	session := "lisa-packet-cursor-roundtrip"

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "cursor_roundtrip",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "packet cursor roundtrip", nil
	}

	stdoutDelta, stderrDelta := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected delta-json packet success, got %d", code)
		}
	})
	if stderrDelta != "" {
		t.Fatalf("unexpected stderr: %q", stderrDelta)
	}
	deltaPayload := parseJSONMap(t, stdoutDelta)
	if deltaPayload["deltaCount"].(float64) == 0 {
		t.Fatalf("expected initial delta entries, got %v", deltaPayload["deltaCount"])
	}

	stdoutPacket, stderrPacket := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected packet cursor-file success after delta-json cursor write, got %d", code)
		}
	})
	if stderrPacket != "" {
		t.Fatalf("unexpected stderr: %q", stderrPacket)
	}
	packetPayload := parseJSONMap(t, stdoutPacket)
	if mapStringValue(packetPayload, "session") != session {
		t.Fatalf("expected session %q, got %v", session, packetPayload["session"])
	}
}

func TestCmdSessionPacketCursorFileSupportsJsonMinThenDeltaJSONSequence(t *testing.T) {
	projectRoot := t.TempDir()
	cursorFile := filepath.Join(projectRoot, "packet.sequence.cursor")
	session := "lisa-packet-sequence-jsonmin-delta"

	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "jsonmin_then_delta",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "packet json-min then delta-json sequence", nil
	}

	stdoutMin, stderrMin := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--cursor-file", cursorFile,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected json-min packet success, got %d", code)
		}
	})
	if stderrMin != "" {
		t.Fatalf("unexpected stderr: %q", stderrMin)
	}
	minPayload := parseJSONMap(t, stdoutMin)
	if mapStringValue(minPayload, "session") != session {
		t.Fatalf("expected session %q, got %v", session, minPayload["session"])
	}

	stdoutDelta, stderrDelta := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--cursor-file", cursorFile,
			"--delta-json",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected delta-json packet success after json-min cursor write, got %d", code)
		}
	})
	if stderrDelta != "" {
		t.Fatalf("unexpected stderr: %q", stderrDelta)
	}
	deltaPayload := parseJSONMap(t, stdoutDelta)
	if deltaPayload["deltaCount"].(float64) == 0 {
		t.Fatalf("expected non-zero field delta count on first delta-json call, got %v", deltaPayload["deltaCount"])
	}
	if _, ok := deltaPayload["errorCode"]; ok {
		t.Fatalf("did not expect errorCode in successful delta payload, got %v", deltaPayload["errorCode"])
	}
}

func TestCmdSessionPacketLegacyJSONDefaultStillAvailable(t *testing.T) {
	origStatus := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origStatus
		tmuxHasSessionFn = origHas
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, poll int) (sessionStatus, error) {
		return sessionStatus{
			Session:              session,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "heartbeat_fresh",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "legacy path", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionPacket([]string{
			"--session", "lisa-packet-legacy-json",
			"--project-root", t.TempDir(),
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected legacy packet success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"summary"`) || !strings.Contains(stdout, `"recent"`) {
		t.Fatalf("expected legacy packet payload fields, got %q", stdout)
	}
	if strings.Contains(stdout, `"delta":{"`) {
		t.Fatalf("did not expect delta payload in legacy mode, got %q", stdout)
	}
}

func TestCmdSessionBudgetObserveFromJSONLUsesLatestValidObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.log")
	content := strings.Join([]string{
		"startup noise",
		`{"tokens":11,"seconds":3}`,
		`INFO runtime {"tokens":29,"seconds":7,"steps":2}`,
		"trailing noise",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionBudgetObserve([]string{
			"--from-jsonl", path,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected budget-observe success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	observed := payload["observed"].(map[string]any)
	if observed["tokens"].(float64) != 29 || observed["seconds"].(float64) != 7 || observed["steps"].(float64) != 2 {
		t.Fatalf("expected latest valid json object values, got %v", observed)
	}
}

func TestCmdSessionBudgetEnforceFromJSONLCompatibleParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed-enforce.log")
	content := strings.Join([]string{
		"noise",
		`{"tokens":9}`,
		`TRACE {"costEstimate":{"totalTokens":55},"steps":[1,2,3]}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionBudgetEnforce([]string{
			"--from-jsonl", path,
			"--max-tokens", "40",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected budget enforce violation exit 1, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	observed := payload["observed"].(map[string]any)
	if observed["tokens"].(float64) != 55 {
		t.Fatalf("expected latest parsed tokens=55, got %v", observed)
	}
	if payload["errorCode"] != "budget_limit_exceeded" {
		t.Fatalf("expected budget_limit_exceeded, got %v", payload["errorCode"])
	}
}
