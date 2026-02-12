package app

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func parseSingleCSVLine(input string) ([]string, error) {
	return csv.NewReader(strings.NewReader(input)).Read()
}

func TestParseAgentAndModeHints(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "auto", want: "auto", ok: true},
		{raw: "claude", want: "claude", ok: true},
		{raw: "codex", want: "codex", ok: true},
		{raw: "bad", ok: false},
	} {
		got, err := parseAgentHint(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("parseAgentHint(%q) unexpected err: %v", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("parseAgentHint(%q) expected error", tc.raw)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("parseAgentHint(%q)=%q, want %q", tc.raw, got, tc.want)
		}
	}

	for _, tc := range []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "auto", want: "auto", ok: true},
		{raw: "interactive", want: "interactive", ok: true},
		{raw: "exec", want: "exec", ok: true},
		{raw: "bad", ok: false},
	} {
		got, err := parseModeHint(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("parseModeHint(%q) unexpected err: %v", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("parseModeHint(%q) expected error", tc.raw)
		}
		if tc.ok && got != tc.want {
			t.Fatalf("parseModeHint(%q)=%q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestCmdSessionStatusJSONAndHintValidation(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if session != "lisa-status-json" {
			t.Fatalf("unexpected session: %q", session)
		}
		if full {
			t.Fatalf("cmdSessionStatus should request full=false, got full=%t", full)
		}
		if agentHint != "auto" || modeHint != "auto" {
			t.Fatalf("unexpected hints agent=%q mode=%q", agentHint, modeHint)
		}
		return sessionStatus{
			Session:      session,
			Status:       "active",
			SessionState: "in_progress",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-json", "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse JSON: %v (%q)", err, stdout)
	}
	if payload["sessionState"] != "in_progress" {
		t.Fatalf("unexpected sessionState payload: %v", payload)
	}

	_, stderr = captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-json", "--agent", "bad"})
		if code == 0 {
			t.Fatalf("expected invalid agent hint failure")
		}
	})
	if !strings.Contains(stderr, "invalid --agent") {
		t.Fatalf("expected invalid agent hint stderr, got %q", stderr)
	}
}

func TestCmdSessionStatusFullTextIncludesSignals(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if !full {
			t.Fatalf("expected full status request")
		}
		return sessionStatus{
			Session:              session,
			Status:               "idle",
			SessionState:         "degraded",
			ClassificationReason: "tmux_snapshot_error",
			PaneStatus:           "alive",
			AgentPID:             0,
			AgentCPU:             0,
			OutputAgeSeconds:     120,
			HeartbeatAge:         5,
			Signals: statusSignals{
				PromptWaiting:     false,
				HeartbeatFresh:    false,
				StateLockTimedOut: true,
				StateLockWaitMS:   2500,
				TMUXReadError:     "display failed",
			},
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-full", "--full"})
		if code != 0 {
			t.Fatalf("expected full text status success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	fields, err := parseSingleCSVLine(stdout)
	if err != nil {
		t.Fatalf("failed to parse CSV status output: %v (%q)", err, stdout)
	}
	if len(fields) < 10 {
		t.Fatalf("expected expanded full status output columns, got %d (%q)", len(fields), stdout)
	}
	if fields[0] != "status_full_v1" {
		t.Fatalf("expected schema prefix in full status output, got %v", fields)
	}
	if fields[6] != "degraded" || fields[7] != "tmux_snapshot_error" {
		t.Fatalf("unexpected state/reason columns: %v", fields)
	}
	if fields[18] != "display failed" {
		t.Fatalf("expected tmux read error column, got %v", fields)
	}
}

func TestCmdSessionSendValidationAndLifecycle(t *testing.T) {
	origHas := tmuxHasSessionFn
	origSendKeys := tmuxSendKeysFn
	origSendText := tmuxSendTextFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxSendKeysFn = origSendKeys
		tmuxSendTextFn = origSendText
		appendSessionEventFn = origAppend
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxSendTextFn = func(session, text string, enter bool) error { return nil }
	tmuxSendKeysFn = func(session string, keys []string, enter bool) error { return nil }

	_, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{"--session", "lisa-send"})
		if code == 0 {
			t.Fatalf("expected validation failure")
		}
	})
	if !strings.Contains(stderr, "provide --text or --keys") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	_, stderr = captureOutput(t, func() {
		code := cmdSessionSend([]string{"--session", "lisa-send", "--text", "a", "--keys", "b"})
		if code == 0 {
			t.Fatalf("expected mutually exclusive validation failure")
		}
	})
	if !strings.Contains(stderr, "use either --text or --keys") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var gotReason string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		gotReason = event.Reason
		return nil
	}
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{
			"--session", "lisa-send",
			"--project-root", t.TempDir(),
			"--keys", "C-c",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected send success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("unexpected JSON output: %q", stdout)
	}
	if gotReason != "send_keys" {
		t.Fatalf("expected lifecycle reason send_keys, got %q", gotReason)
	}
}

func TestCmdSessionExplainRejectsInvalidEventsFlag(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := cmdSessionExplain([]string{"--session", "lisa-explain", "--events", "0"})
		if code == 0 {
			t.Fatalf("expected invalid events flag failure")
		}
	})
	if !strings.Contains(stderr, "invalid --events") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestCmdSessionExplainReadEventError(t *testing.T) {
	origCompute := computeSessionStatusFn
	origReadTail := readSessionEventTailFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		readSessionEventTailFn = origReadTail
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
		}, nil
	}
	readSessionEventTailFn = func(projectRoot, session string, max int) (sessionEventTail, error) {
		return sessionEventTail{}, errors.New("read tail boom")
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSessionExplain([]string{"--session", "lisa-explain"})
		if code == 0 {
			t.Fatalf("expected explain failure when tail read fails")
		}
	})
	if !strings.Contains(stderr, "failed reading session events") {
		t.Fatalf("expected read events error stderr, got %q", stderr)
	}
}

func TestCmdSessionMonitorEmitsLifecycleEvent(t *testing.T) {
	origCompute := computeSessionStatusFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		appendSessionEventFn = origAppend
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
		}, nil
	}

	var gotReason string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		gotReason = event.Reason
		return nil
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-event",
			"--max-polls", "1",
			"--poll-interval", "1",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected monitor completion success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if gotReason != "monitor_completed" {
		t.Fatalf("expected lifecycle reason monitor_completed, got %q", gotReason)
	}
}

func TestCmdSessionSpawnAndKillEmitLifecycleEvents(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	origSendCmd := tmuxSendCommandWithFallbackFn
	origKill := tmuxKillSessionFn
	origEnsure := ensureHeartbeatWritableFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		tmuxSendCommandWithFallbackFn = origSendCmd
		tmuxKillSessionFn = origKill
		ensureHeartbeatWritableFn = origEnsure
		appendSessionEventFn = origAppend
	})

	var reasons []string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		reasons = append(reasons, event.Reason)
		return nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error { return nil }
	tmuxSendCommandWithFallbackFn = func(projectRoot, session, command string, enter bool) error { return nil }
	tmuxKillSessionFn = func(session string) error { return nil }
	ensureHeartbeatWritableFn = func(path string) error {
		return os.WriteFile(path, []byte(""), 0o600)
	}

	projectRoot := t.TempDir()
	session := "lisa-lifecycle-test"
	_, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "claude",
			"--mode", "interactive",
			"--project-root", projectRoot,
			"--session", session,
			"--command", "echo hi",
		})
		if code != 0 {
			t.Fatalf("expected spawn success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected spawn stderr: %q", stderr)
	}

	tmuxHasSessionFn = func(session string) bool { return true }
	_, stderr = captureOutput(t, func() {
		code := cmdSessionKill([]string{"--session", session, "--project-root", projectRoot})
		if code != 0 {
			t.Fatalf("expected kill success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected kill stderr: %q", stderr)
	}

	if len(reasons) < 2 {
		t.Fatalf("expected spawn+kill lifecycle events, got %v", reasons)
	}
	if reasons[0] != "spawn_success" {
		t.Fatalf("expected first lifecycle reason spawn_success, got %q", reasons[0])
	}
	if reasons[1] != "kill_success" {
		t.Fatalf("expected second lifecycle reason kill_success, got %q", reasons[1])
	}

	countPath := sessionEventCountFile(sessionEventsFile(projectRoot, session))
	if _, err := os.Stat(countPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected event line counter path error: %v", err)
	}
}

func TestCmdSessionRoutesCoreSubcommands(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionFn
	origSendCmd := tmuxSendCommandWithFallbackFn
	origKill := tmuxKillSessionFn
	origEnsure := ensureHeartbeatWritableFn
	origSendText := tmuxSendTextFn
	origCapture := tmuxCapturePaneFn
	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	origReadTail := readSessionEventTailFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionFn = origNew
		tmuxSendCommandWithFallbackFn = origSendCmd
		tmuxKillSessionFn = origKill
		ensureHeartbeatWritableFn = origEnsure
		tmuxSendTextFn = origSendText
		tmuxCapturePaneFn = origCapture
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
		readSessionEventTailFn = origReadTail
		appendSessionEventFn = origAppend
	})

	projectRoot := t.TempDir()
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }
	routeSessionAlive := false
	tmuxHasSessionFn = func(session string) bool {
		if session == "lisa-route-spawn" {
			return routeSessionAlive
		}
		return true
	}
	tmuxNewSessionFn = func(session, projectRoot, agent, mode string, width, height int) error {
		if session == "lisa-route-spawn" {
			routeSessionAlive = true
		}
		return nil
	}
	tmuxSendCommandWithFallbackFn = func(projectRoot, session, command string, enter bool) error { return nil }
	tmuxKillSessionFn = func(session string) error {
		if session == "lisa-route-spawn" {
			routeSessionAlive = false
		}
		return nil
	}
	ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
	tmuxSendTextFn = func(session, text string, enter bool) error { return nil }
	tmuxCapturePaneFn = func(session string, lines int) (string, error) { return "capture", nil }
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) { return []string{}, nil }
	computeSessionStatusFn = func(session, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "completed",
		}, nil
	}
	readSessionEventTailFn = func(projectRoot, session string, max int) (sessionEventTail, error) {
		return sessionEventTail{}, os.ErrNotExist
	}

	type routeCase struct {
		args []string
		want int
	}
	cases := []routeCase{
		{args: []string{"name", "--project-root", projectRoot}, want: 0},
		{args: []string{"spawn", "--agent", "claude", "--mode", "interactive", "--session", "lisa-route-spawn", "--project-root", projectRoot, "--command", "echo hi"}, want: 0},
		{args: []string{"send", "--session", "lisa-route-spawn", "--project-root", projectRoot, "--text", "ok"}, want: 0},
		{args: []string{"status", "--session", "lisa-route-spawn", "--project-root", projectRoot}, want: 0},
		{args: []string{"explain", "--session", "lisa-route-spawn", "--project-root", projectRoot}, want: 0},
		{args: []string{"monitor", "--session", "lisa-route-spawn", "--project-root", projectRoot, "--max-polls", "1", "--poll-interval", "1", "--json"}, want: 0},
		{args: []string{"capture", "--session", "lisa-route-spawn", "--raw"}, want: 0},
		{args: []string{"list", "--project-root", projectRoot}, want: 0},
		{args: []string{"exists", "--session", "lisa-route-spawn"}, want: 0},
		{args: []string{"kill", "--session", "lisa-route-spawn", "--project-root", projectRoot}, want: 0},
		{args: []string{"kill-all", "--project-root", projectRoot}, want: 0},
	}

	for _, tc := range cases {
		stdout, stderr := captureOutput(t, func() {
			code := cmdSession(tc.args)
			if code != tc.want {
				t.Fatalf("cmdSession(%v) exit=%d want=%d", tc.args, code, tc.want)
			}
		})
		if strings.Contains(stderr, "unknown") {
			t.Fatalf("cmdSession(%v) unexpected stderr=%q stdout=%q", tc.args, stderr, stdout)
		}
	}
}

func TestCmdSessionKillAllEmitsEventAfterCleanup(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
		appendSessionEventFn = origAppend
	})

	projectRoot := t.TempDir()
	session := "lisa-killall-order"
	metaPath := sessionMetaFile(projectRoot, session)
	if err := os.WriteFile(metaPath, []byte(`{"session":"lisa-killall-order"}`), 0o600); err != nil {
		t.Fatalf("failed to seed meta file: %v", err)
	}

	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) { return []string{session}, nil }
	tmuxKillSessionFn = func(session string) error { return nil }

	var observedReason string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		if fileExists(metaPath) {
			t.Fatalf("expected cleanup to run before lifecycle event append")
		}
		observedReason = event.Reason
		return nil
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSessionKillAll([]string{"--project-root", projectRoot})
		if code != 0 {
			t.Fatalf("expected kill-all success")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected kill-all stderr: %q", stderr)
	}
	if observedReason != "kill_all_success" {
		t.Fatalf("expected lifecycle reason kill_all_success, got %q", observedReason)
	}
}
