package app

import (
	"strings"
	"testing"
)

func TestCmdSessionTurnRunsSendMonitorPacket(t *testing.T) {
	origSend := sessionTurnSendFn
	origMonitor := sessionTurnMonitorFn
	origPacket := sessionTurnPacketFn
	t.Cleanup(func() {
		sessionTurnSendFn = origSend
		sessionTurnMonitorFn = origMonitor
		sessionTurnPacketFn = origPacket
	})

	var sendArgsSeen []string
	var monitorArgsSeen []string
	var packetArgsSeen []string

	sessionTurnSendFn = func(args []string) int {
		sendArgsSeen = append([]string{}, args...)
		writeJSON(map[string]any{"session": "lisa-turn", "ok": true})
		return 0
	}
	sessionTurnMonitorFn = func(args []string) int {
		monitorArgsSeen = append([]string{}, args...)
		writeJSON(map[string]any{
			"session":    "lisa-turn",
			"finalState": "waiting_input",
			"exitReason": "waiting_input",
		})
		return 0
	}
	sessionTurnPacketFn = func(args []string) int {
		packetArgsSeen = append([]string{}, args...)
		writeJSON(map[string]any{
			"session":      "lisa-turn",
			"status":       "idle",
			"sessionState": "waiting_input",
			"nextAction":   "session send",
		})
		return 0
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn",
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--enter",
			"--poll-interval", "2",
			"--max-polls", "6",
			"--expect", "marker",
			"--until-marker", "DONE",
			"--lines", "44",
			"--events", "7",
			"--token-budget", "111",
			"--summary-style", "terse",
			"--fields", "session,status",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if payload["session"] != "lisa-turn" {
		t.Fatalf("expected session lisa-turn, got %v", payload["session"])
	}
	if payload["sendExitCode"] != float64(0) || payload["monitorExitCode"] != float64(0) || payload["packetExitCode"] != float64(0) {
		t.Fatalf("expected zero step exit codes, got %v", payload)
	}

	sendJoined := strings.Join(sendArgsSeen, " ")
	if !strings.Contains(sendJoined, "--text Continue") || !strings.Contains(sendJoined, "--enter") {
		t.Fatalf("expected send args passthrough, got %v", sendArgsSeen)
	}
	monitorJoined := strings.Join(monitorArgsSeen, " ")
	for _, token := range []string{"--poll-interval 2", "--max-polls 6", "--expect marker", "--until-marker DONE"} {
		if !strings.Contains(monitorJoined, token) {
			t.Fatalf("monitor args missing %q: %v", token, monitorArgsSeen)
		}
	}
	packetJoined := strings.Join(packetArgsSeen, " ")
	for _, token := range []string{"--lines 44", "--events 7", "--token-budget 111", "--summary-style terse", "--fields session,status"} {
		if !strings.Contains(packetJoined, token) {
			t.Fatalf("packet args missing %q: %v", token, packetArgsSeen)
		}
	}
}

func TestCmdSessionTurnReturnsMonitorFailure(t *testing.T) {
	origSend := sessionTurnSendFn
	origMonitor := sessionTurnMonitorFn
	origPacket := sessionTurnPacketFn
	t.Cleanup(func() {
		sessionTurnSendFn = origSend
		sessionTurnMonitorFn = origMonitor
		sessionTurnPacketFn = origPacket
	})

	packetCalled := false
	sessionTurnSendFn = func(args []string) int {
		writeJSON(map[string]any{"session": "lisa-turn", "ok": true})
		return 0
	}
	sessionTurnMonitorFn = func(args []string) int {
		writeJSON(map[string]any{
			"ok":        false,
			"errorCode": "max_polls_exceeded",
			"error":     "monitor timed out",
		})
		return 2
	}
	sessionTurnPacketFn = func(args []string) int {
		packetCalled = true
		writeJSON(map[string]any{"ok": true})
		return 0
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn",
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--json",
		})
		if code != 2 {
			t.Fatalf("expected monitor step exit code 2, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if packetCalled {
		t.Fatalf("packet step should not run after monitor failure")
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "turn_monitor_failed" {
		t.Fatalf("expected turn_monitor_failed, got %v", payload["errorCode"])
	}
	if payload["failedStep"] != "monitor" {
		t.Fatalf("expected failedStep monitor, got %v", payload["failedStep"])
	}
}

func TestCmdSessionTurnRequiresSendPayload(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn-missing-payload",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "missing_send_payload" {
		t.Fatalf("expected missing_send_payload, got %v", payload["errorCode"])
	}
}

func TestCmdSessionTurnRejectsConflictingSendPayload(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn-conflict-payload",
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--keys", "Enter",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "send_payload_conflict" {
		t.Fatalf("expected send_payload_conflict, got %v", payload["errorCode"])
	}
}

func TestCmdSessionTurnRequiresSession(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "missing_required_flag" {
		t.Fatalf("expected missing_required_flag, got %v", payload["errorCode"])
	}
}

func TestCmdSessionTurnJSONMinSuccessPayload(t *testing.T) {
	origSend := sessionTurnSendFn
	origMonitor := sessionTurnMonitorFn
	origPacket := sessionTurnPacketFn
	t.Cleanup(func() {
		sessionTurnSendFn = origSend
		sessionTurnMonitorFn = origMonitor
		sessionTurnPacketFn = origPacket
	})

	sendCalled := false
	monitorCalled := false
	packetCalled := false

	sessionTurnSendFn = func(args []string) int {
		sendCalled = true
		writeJSON(map[string]any{"session": "lisa-turn-json-min", "ok": true})
		return 0
	}
	sessionTurnMonitorFn = func(args []string) int {
		monitorCalled = true
		writeJSON(map[string]any{
			"session":    "lisa-turn-json-min",
			"finalState": "waiting_input",
			"exitReason": "waiting_input",
		})
		return 0
	}
	sessionTurnPacketFn = func(args []string) int {
		packetCalled = true
		writeJSON(map[string]any{
			"session":      "lisa-turn-json-min",
			"sessionState": "waiting_input",
			"status":       "idle",
			"nextAction":   "session send",
		})
		return 0
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn-json-min",
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !sendCalled || !monitorCalled || !packetCalled {
		t.Fatalf("expected send/monitor/packet to run, got send=%t monitor=%t packet=%t", sendCalled, monitorCalled, packetCalled)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if payload["session"] != "lisa-turn-json-min" {
		t.Fatalf("expected session lisa-turn-json-min, got %v", payload["session"])
	}
	if payload["finalState"] != "waiting_input" {
		t.Fatalf("expected finalState waiting_input, got %v", payload["finalState"])
	}
	if payload["exitReason"] != "waiting_input" {
		t.Fatalf("expected exitReason waiting_input, got %v", payload["exitReason"])
	}
	if payload["sessionState"] != "waiting_input" {
		t.Fatalf("expected sessionState waiting_input, got %v", payload["sessionState"])
	}
	if payload["status"] != "idle" {
		t.Fatalf("expected status idle, got %v", payload["status"])
	}
	if payload["nextAction"] != "session send" {
		t.Fatalf("expected nextAction session send, got %v", payload["nextAction"])
	}
}

func TestCmdSessionTurnJSONMinSendFailureCompactPayload(t *testing.T) {
	origSend := sessionTurnSendFn
	origMonitor := sessionTurnMonitorFn
	origPacket := sessionTurnPacketFn
	t.Cleanup(func() {
		sessionTurnSendFn = origSend
		sessionTurnMonitorFn = origMonitor
		sessionTurnPacketFn = origPacket
	})

	monitorCalled := false
	packetCalled := false

	sessionTurnSendFn = func(args []string) int {
		writeJSON(map[string]any{
			"ok":        false,
			"errorCode": "send_failed",
			"error":     "send blew up",
		})
		return 7
	}
	sessionTurnMonitorFn = func(args []string) int {
		monitorCalled = true
		writeJSON(map[string]any{"ok": true})
		return 0
	}
	sessionTurnPacketFn = func(args []string) int {
		packetCalled = true
		writeJSON(map[string]any{"ok": true})
		return 0
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionTurn([]string{
			"--session", "lisa-turn-json-min-send-fail",
			"--project-root", t.TempDir(),
			"--text", "Continue",
			"--json-min",
		})
		if code != 7 {
			t.Fatalf("expected send step exit code 7, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if monitorCalled || packetCalled {
		t.Fatalf("expected monitor/packet not called, got monitor=%t packet=%t", monitorCalled, packetCalled)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", payload["ok"])
	}
	if payload["failedStep"] != "send" {
		t.Fatalf("expected failedStep send, got %v", payload["failedStep"])
	}
	if payload["errorCode"] != "turn_send_failed" {
		t.Fatalf("expected turn_send_failed, got %v", payload["errorCode"])
	}
	if _, exists := payload["send"]; exists {
		t.Fatalf("expected json-min payload to omit send, got %v", payload["send"])
	}
	if _, exists := payload["monitor"]; exists {
		t.Fatalf("expected json-min payload to omit monitor, got %v", payload["monitor"])
	}
	if _, exists := payload["packet"]; exists {
		t.Fatalf("expected json-min payload to omit packet, got %v", payload["packet"])
	}
}

func TestCmdSessionStateSandboxSnapshotClearRestoreRoundTrip(t *testing.T) {
	projectRoot := t.TempDir()
	snapshotFile := projectRoot + "/sandbox/snapshot.json"

	objectiveStore := sessionObjectiveStore{
		CurrentID: "main",
		Objectives: map[string]sessionObjectiveRecord{
			"main": {
				ID:        "main",
				Goal:      "Ship cluster C",
				Status:    "open",
				CreatedAt: "2026-02-24T00:00:00Z",
				UpdatedAt: "2026-02-24T00:00:00Z",
			},
		},
		UpdatedAt: "2026-02-24T00:00:00Z",
	}
	laneStore := sessionLaneStore{
		Lanes: map[string]sessionLaneRecord{
			"planner": {
				Name:      "planner",
				Goal:      "Plan + execute",
				Agent:     "codex",
				Mode:      "interactive",
				UpdatedAt: "2026-02-24T00:00:00Z",
			},
		},
		UpdatedAt: "2026-02-24T00:00:00Z",
	}
	if err := saveObjectiveStoreExact(projectRoot, objectiveStore); err != nil {
		t.Fatalf("seed objective store: %v", err)
	}
	if err := saveLaneStoreExact(projectRoot, laneStore); err != nil {
		t.Fatalf("seed lane store: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		code := cmdSessionStateSandbox([]string{
			"snapshot",
			"--project-root", projectRoot,
			"--file", snapshotFile,
			"--json",
		})
		if code != 0 {
			t.Fatalf("snapshot failed: %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr during snapshot: %q", stderr)
	}
	_, stderr = captureOutput(t, func() {
		code := cmdSessionStateSandbox([]string{
			"clear",
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("clear failed: %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr during clear: %q", stderr)
	}
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionStateSandbox([]string{
			"restore",
			"--project-root", projectRoot,
			"--file", snapshotFile,
			"--json",
		})
		if code != 0 {
			t.Fatalf("restore failed: %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr during restore: %q", stderr)
	}
	restorePayload := parseJSONMap(t, stdout)
	if restorePayload["objectiveCount"] != float64(1) || restorePayload["laneCount"] != float64(1) {
		t.Fatalf("expected restored counts 1/1, got %v", restorePayload)
	}

	restoredObjectives, err := loadObjectiveStore(projectRoot)
	if err != nil {
		t.Fatalf("load restored objective store: %v", err)
	}
	restoredLanes, err := loadLaneStore(projectRoot)
	if err != nil {
		t.Fatalf("load restored lane store: %v", err)
	}
	if restoredObjectives.UpdatedAt != objectiveStore.UpdatedAt {
		t.Fatalf("expected objective UpdatedAt preserved, got %q want %q", restoredObjectives.UpdatedAt, objectiveStore.UpdatedAt)
	}
	if restoredLanes.UpdatedAt != laneStore.UpdatedAt {
		t.Fatalf("expected lane UpdatedAt preserved, got %q want %q", restoredLanes.UpdatedAt, laneStore.UpdatedAt)
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdSessionStateSandbox([]string{
			"list",
			"--project-root", projectRoot,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("list failed: %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr during list: %q", stderr)
	}
	listPayload := parseJSONMap(t, stdout)
	if listPayload["objectiveCount"] != float64(1) || listPayload["laneCount"] != float64(1) {
		t.Fatalf("expected list counts 1/1, got %v", listPayload)
	}
}

func TestCmdSessionStateSandboxRestoreRequiresFile(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionStateSandbox([]string{
			"restore",
			"--project-root", t.TempDir(),
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "missing_required_flag" {
		t.Fatalf("expected missing_required_flag, got %v", payload["errorCode"])
	}
}
