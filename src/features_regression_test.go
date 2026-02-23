package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCmdSessionTreeJSONIncludesHierarchy(t *testing.T) {
	projectRoot := t.TempDir()
	rootSession := "lisa-tree-root"
	childSession := "lisa-tree-child"
	grandchildSession := "lisa-tree-grandchild"

	now := "2026-02-20T09:10:00Z"
	if err := saveSessionMeta(projectRoot, rootSession, sessionMeta{
		Session:     rootSession,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		StartCmd:    "echo root",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save root meta failed: %v", err)
	}
	if err := saveSessionMeta(projectRoot, childSession, sessionMeta{
		Session:       childSession,
		ParentSession: rootSession,
		Agent:         "codex",
		Mode:          "interactive",
		ProjectRoot:   projectRoot,
		StartCmd:      "echo child",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("save child meta failed: %v", err)
	}
	if err := saveSessionMeta(projectRoot, grandchildSession, sessionMeta{
		Session:       grandchildSession,
		ParentSession: childSession,
		Agent:         "codex",
		Mode:          "interactive",
		ProjectRoot:   projectRoot,
		StartCmd:      "echo grandchild",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("save grandchild meta failed: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSession([]string{"tree", "--project-root", projectRoot, "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"session":"lisa-tree-root"`) {
		t.Fatalf("expected root in JSON output, got %q", stdout)
	}
	if !strings.Contains(stdout, `"session":"lisa-tree-child"`) {
		t.Fatalf("expected child in JSON output, got %q", stdout)
	}
	if !strings.Contains(stdout, `"session":"lisa-tree-grandchild"`) {
		t.Fatalf("expected grandchild in JSON output, got %q", stdout)
	}
}

func TestCmdSessionSpawnDryRunJSONSkipsSessionCreation(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionWithStartupFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNew
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		t.Fatalf("tmux session should not be created during --dry-run")
		return nil
	}

	projectRoot := t.TempDir()
	session := "lisa-dry-run-test"
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "run ./lisa session spawn nested test",
			"--project-root", projectRoot,
			"--session", session,
			"--dry-run",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected dry-run success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed parsing dry-run JSON: %v (%q)", err, stdout)
	}
	if payload["session"] != session {
		t.Fatalf("expected session %q, got %v", session, payload["session"])
	}
	if payload["dryRun"] != true {
		t.Fatalf("expected dryRun=true payload, got %v", payload["dryRun"])
	}
	if _, ok := payload["socketPath"]; !ok {
		t.Fatalf("expected socketPath field in dry-run payload: %v", payload)
	}
	if _, err := os.Stat(sessionMetaFile(projectRoot, session)); err == nil {
		t.Fatalf("did not expect metadata file to be written for dry-run session")
	}
}

func TestCmdSessionListAllSocketsFindsCrossRootSessions(t *testing.T) {
	origHas := tmuxHasSessionFn
	origList := tmuxListSessionsFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxListSessionsFn = origList
	})
	tmuxHasSessionFn = func(session string) bool {
		return session == "lisa-cross-a" || session == "lisa-cross-b"
	}
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{}, nil
	}

	rootA := t.TempDir()
	rootB := t.TempDir()
	now := "2026-02-20T09:12:00Z"

	if err := saveSessionMeta(rootA, "lisa-cross-a", sessionMeta{
		Session:     "lisa-cross-a",
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: rootA,
		StartCmd:    "echo A",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save meta A failed: %v", err)
	}
	if err := saveSessionMeta(rootB, "lisa-cross-b", sessionMeta{
		Session:     "lisa-cross-b",
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: rootB,
		StartCmd:    "echo B",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save meta B failed: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", rootA, "--all-sockets"})
		if code != 0 {
			t.Fatalf("expected list --all-sockets success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, "lisa-cross-a") || !strings.Contains(stdout, "lisa-cross-b") {
		t.Fatalf("expected both cross-root sessions in output, got %q", stdout)
	}
}

func TestCmdSessionListWithNextActionIncludesSocketPathFromMeta(t *testing.T) {
	origList := tmuxListSessionsFn
	origStatus := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origStatus
	})

	projectRoot := t.TempDir()
	session := "lisa-socket-meta"
	socketPath := "/tmp/custom-lisa-socket.sock"
	now := "2026-02-23T10:00:00Z"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		SocketPath:  socketPath,
		StartCmd:    "echo",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("save session meta failed: %v", err)
	}

	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{session}, nil
	}
	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "active",
			SessionState: "in_progress",
		}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionList([]string{"--project-root", projectRoot, "--with-next-action", "--json"})
		if code != 0 {
			t.Fatalf("expected session list success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		Items []sessionListItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed parsing session list JSON: %v (%q)", err, stdout)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items len = %d", len(payload.Items))
	}
	if payload.Items[0].SocketPath != socketPath {
		t.Fatalf("socketPath = %q, want %q", payload.Items[0].SocketPath, socketPath)
	}
}

func TestCmdSessionMonitorStopsOnUntilMarker(t *testing.T) {
	origCompute := computeSessionStatusFn
	origCapture := tmuxCapturePaneFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxCapturePaneFn = origCapture
	})

	computeSessionStatusFn = func(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:      session,
			Status:       "active",
			SessionState: "in_progress",
		}, nil
	}
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "work...\nDONE_MARKER_123\n", nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMonitor([]string{
			"--session", "lisa-monitor-marker",
			"--project-root", t.TempDir(),
			"--max-polls", "2",
			"--poll-interval", "1",
			"--until-marker", "DONE_MARKER_123",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected marker-based monitor success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"exitReason":"marker_found"`) {
		t.Fatalf("expected marker_found exit reason, got %q", stdout)
	}
}
