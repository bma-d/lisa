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

func TestCmdSessionStatusFailNotFound(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "not_found",
			SessionState: "not_found",
		}, nil
	}

	_, _ = captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-missing"})
		if code != 0 {
			t.Fatalf("expected default status behavior to stay zero, got %d", code)
		}
	})

	_, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-missing", "--fail-not-found", "--json"})
		if code == 0 {
			t.Fatal("expected non-zero status exit with --fail-not-found")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
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

func TestNormalizeStatusForSessionStatusOutput(t *testing.T) {
	cases := []struct {
		name         string
		status       string
		sessionState string
		wantStatus   string
	}{
		{name: "in progress unchanged", status: "active", sessionState: "in_progress", wantStatus: "active"},
		{name: "waiting unchanged", status: "idle", sessionState: "waiting_input", wantStatus: "idle"},
		{name: "completed normalized", status: "idle", sessionState: "completed", wantStatus: "completed"},
		{name: "crashed normalized", status: "idle", sessionState: "crashed", wantStatus: "crashed"},
		{name: "stuck normalized", status: "idle", sessionState: "stuck", wantStatus: "stuck"},
		{name: "not found normalized", status: "not_found", sessionState: "not_found", wantStatus: "not_found"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeStatusForSessionStatusOutput(sessionStatus{
				Status:       tc.status,
				SessionState: tc.sessionState,
			})
			if got.Status != tc.wantStatus {
				t.Fatalf("normalizeStatusForSessionStatusOutput(%q,%q) status=%q want=%q", tc.status, tc.sessionState, got.Status, tc.wantStatus)
			}
		})
	}
}

func TestCmdSessionStatusNormalizesTerminalStatusOutput(t *testing.T) {
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "idle",
			SessionState: "crashed",
		}, nil
	}

	jsonOut, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-normalized", "--json"})
		if code != 0 {
			t.Fatalf("expected json status success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("failed to parse JSON status payload: %v (%q)", err, jsonOut)
	}
	if payload["status"] != "crashed" || payload["sessionState"] != "crashed" {
		t.Fatalf("expected normalized terminal status in JSON payload, got %v", payload)
	}

	csvOut, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-normalized"})
		if code != 0 {
			t.Fatalf("expected csv status success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	record, err := parseSingleCSVLine(csvOut)
	if err != nil {
		t.Fatalf("failed to parse CSV status output: %v (%q)", err, csvOut)
	}
	if len(record) != 6 {
		t.Fatalf("expected 6 csv fields, got %d (%q)", len(record), csvOut)
	}
	if record[0] != "crashed" || record[5] != "crashed" {
		t.Fatalf("expected normalized terminal status in CSV output, got %v", record)
	}

	fullOut, stderr := captureOutput(t, func() {
		code := cmdSessionStatus([]string{"--session", "lisa-status-normalized", "--full"})
		if code != 0 {
			t.Fatalf("expected full status success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	fullRecord, err := parseSingleCSVLine(fullOut)
	if err != nil {
		t.Fatalf("failed to parse full CSV status output: %v (%q)", err, fullOut)
	}
	if len(fullRecord) < 7 {
		t.Fatalf("expected expanded full status output columns, got %d (%q)", len(fullRecord), fullOut)
	}
	if fullRecord[1] != "crashed" || fullRecord[6] != "crashed" {
		t.Fatalf("expected normalized terminal status in full CSV output, got %v", fullRecord)
	}
}

func TestNormalizeMonitorFinalStatus(t *testing.T) {
	cases := []struct {
		name       string
		finalState string
		finalStat  string
		want       string
	}{
		{name: "completed normalized", finalState: "completed", finalStat: "idle", want: "completed"},
		{name: "crashed normalized", finalState: "crashed", finalStat: "idle", want: "crashed"},
		{name: "stuck normalized", finalState: "stuck", finalStat: "idle", want: "stuck"},
		{name: "not found normalized", finalState: "not_found", finalStat: "idle", want: "not_found"},
		{name: "timeout passthrough", finalState: "timeout", finalStat: "active", want: "active"},
		{name: "waiting passthrough", finalState: "waiting_input", finalStat: "idle", want: "idle"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeMonitorFinalStatus(tc.finalState, tc.finalStat)
			if got != tc.want {
				t.Fatalf("normalizeMonitorFinalStatus(%q,%q)=%q want=%q", tc.finalState, tc.finalStat, got, tc.want)
			}
		})
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

func TestCmdSessionSendValidatesArgsBeforeSessionLookup(t *testing.T) {
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
	})

	lookupCalled := false
	tmuxHasSessionFn = func(session string) bool {
		lookupCalled = true
		return false
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSessionSend([]string{"--session", "lisa-send", "--text", "a", "--keys", "b"})
		if code == 0 {
			t.Fatalf("expected mutually exclusive validation failure")
		}
	})
	if !strings.Contains(stderr, "use either --text or --keys") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if lookupCalled {
		t.Fatal("expected argument validation before tmux session lookup")
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

	var gotReason, gotStatus string
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
		gotReason = event.Reason
		gotStatus = event.Status
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
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
	if gotStatus != "completed" {
		t.Fatalf("expected normalized lifecycle status completed, got %q", gotStatus)
	}
	if !strings.Contains(stdout, `"finalStatus":"completed"`) {
		t.Fatalf("expected normalized monitor finalStatus in json output, got %q", stdout)
	}
}

func TestCmdSessionSpawnAndKillEmitLifecycleEvents(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	origKill := tmuxKillSessionFn
	origEnsure := ensureHeartbeatWritableFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
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
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return nil
	}
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
	origNewWithStartup := tmuxNewSessionWithStartupFn
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
		tmuxNewSessionWithStartupFn = origNewWithStartup
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
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		if session == "lisa-route-spawn" {
			routeSessionAlive = true
		}
		return nil
	}
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
		{args: []string{"exists", "--session", "lisa-route-spawn", "--project-root", projectRoot}, want: 0},
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
