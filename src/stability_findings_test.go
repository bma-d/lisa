package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func resetProcessListCacheForTest() {
	processCache.mu.Lock()
	processCache.fnPtr = 0
	processCache.atNanos = 0
	processCache.procs = nil
	processCache.mu.Unlock()
}

func TestDetectAgentProcessMatchesPaneRootProcess(t *testing.T) {
	origList := listProcessesFn
	t.Cleanup(func() {
		listProcessesFn = origList
		resetProcessListCacheForTest()
	})
	resetProcessListCacheForTest()

	listProcessesFn = func() ([]processInfo, error) {
		return []processInfo{
			{PID: 100, PPID: 1, CPU: 0.1, Command: "codex exec 'task'"},
			{PID: 101, PPID: 100, CPU: 0.0, Command: "sleep 1"},
		}, nil
	}

	pid, cpu, err := detectAgentProcess(100, "codex")
	if err != nil {
		t.Fatalf("unexpected detectAgentProcess error: %v", err)
	}
	if pid != 100 {
		t.Fatalf("expected pane root process PID to match agent, got %d", pid)
	}
	if cpu != 0.1 {
		t.Fatalf("expected root process cpu=0.1, got %f", cpu)
	}
}

func TestCmdSessionKillAllDefaultAlsoCleansCrossHashArtifacts(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
	})

	projectRootA := filepath.Join(t.TempDir(), "root-a")
	projectRootB := filepath.Join(t.TempDir(), "root-b")
	if err := os.MkdirAll(projectRootA, 0o755); err != nil {
		t.Fatalf("failed creating root A: %v", err)
	}
	if err := os.MkdirAll(projectRootB, 0o755); err != nil {
		t.Fatalf("failed creating root B: %v", err)
	}

	session := "lisa-cross-hash-killall"
	metaA := sessionMetaFile(projectRootA, session)
	metaB := sessionMetaFile(projectRootB, session)
	if err := os.WriteFile(metaA, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed seeding root A metadata: %v", err)
	}
	if err := os.WriteFile(metaB, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed seeding root B metadata: %v", err)
	}

	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		if projectOnly {
			t.Fatalf("expected non-project-only listing for default kill-all")
		}
		return []string{session}, nil
	}
	tmuxKillSessionFn = func(session string) error { return nil }

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionKillAll([]string{"--project-root", projectRootA}); code != 0 {
			t.Fatalf("expected kill-all success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if fileExists(metaA) || fileExists(metaB) {
		t.Fatalf("expected kill-all to clean artifacts across hashes; metaA=%t metaB=%t", fileExists(metaA), fileExists(metaB))
	}
}

func TestCmdSessionKillAllProjectOnlyKeepsCrossHashArtifactsByDefault(t *testing.T) {
	origList := tmuxListSessionsFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		tmuxKillSessionFn = origKill
	})

	projectRootA := filepath.Join(t.TempDir(), "root-a")
	projectRootB := filepath.Join(t.TempDir(), "root-b")
	if err := os.MkdirAll(projectRootA, 0o755); err != nil {
		t.Fatalf("failed creating root A: %v", err)
	}
	if err := os.MkdirAll(projectRootB, 0o755); err != nil {
		t.Fatalf("failed creating root B: %v", err)
	}

	session := "lisa-cross-hash-killall-project-only"
	metaA := sessionMetaFile(projectRootA, session)
	metaB := sessionMetaFile(projectRootB, session)
	if err := os.WriteFile(metaA, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed seeding root A metadata: %v", err)
	}
	if err := os.WriteFile(metaB, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed seeding root B metadata: %v", err)
	}

	tmuxListSessionsFn = func(projectOnly bool, projectRoot string) ([]string, error) {
		if !projectOnly {
			t.Fatalf("expected project-only listing")
		}
		return []string{session}, nil
	}
	tmuxKillSessionFn = func(session string) error { return nil }

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionKillAll([]string{"--project-only", "--project-root", projectRootA}); code != 0 {
			t.Fatalf("expected kill-all success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if fileExists(metaA) {
		t.Fatalf("expected project hash artifacts to be cleaned")
	}
	if !fileExists(metaB) {
		t.Fatalf("expected cross-hash artifacts to remain for project-only kill-all")
	}
}

func TestCmdSessionSpawnFailureEmitsLifecycleReason(t *testing.T) {
	type tc struct {
		name         string
		args         []string
		setup        func(projectRoot string)
		wantReason   string
		wantErrMatch string
	}

	cases := []tc{
		{
			name: "heartbeat prep failure",
			args: []string{"--session", "lisa-spawn-hb-fail", "--command", "echo ok"},
			setup: func(projectRoot string) {
				ensureHeartbeatWritableFn = func(path string) error { return errors.New("permission denied") }
			},
			wantReason:   "spawn_heartbeat_prepare_error",
			wantErrMatch: "failed to prepare heartbeat file",
		},
		{
			name: "command build failure",
			args: []string{"--session", "lisa-spawn-build-fail", "--mode", "exec"},
			setup: func(projectRoot string) {
				ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
			},
			wantReason:   "spawn_command_build_error",
			wantErrMatch: "exec mode requires --prompt",
		},
		{
			name: "tmux new failure",
			args: []string{"--session", "lisa-spawn-tmux-fail", "--command", "echo ok"},
			setup: func(projectRoot string) {
				ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
				tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
					return errors.New("tmux create failed")
				}
			},
			wantReason:   "spawn_tmux_new_error",
			wantErrMatch: "failed to create tmux session",
		},
		{
			name: "startup send failure",
			args: []string{"--session", "lisa-spawn-send-fail", "--command", "echo ok"},
			setup: func(projectRoot string) {
				ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
				tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
					return errors.New("send failed")
				}
			},
			wantReason:   "spawn_tmux_new_error",
			wantErrMatch: "failed to create tmux session",
		},
		{
			name: "metadata persist failure",
			args: []string{"--session", "lisa-spawn-meta-fail", "--command", "echo ok"},
			setup: func(projectRoot string) {
				ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
				tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
					return nil
				}
				saveSessionMetaFn = func(projectRoot, session string, meta sessionMeta) error {
					return errors.New("disk full")
				}
			},
			wantReason:   "spawn_meta_persist_error",
			wantErrMatch: "failed to persist metadata",
		},
	}

	origHas := tmuxHasSessionFn
	origNewWithStartup := tmuxNewSessionWithStartupFn
	origKill := tmuxKillSessionFn
	origEnsure := ensureHeartbeatWritableFn
	origSaveMeta := saveSessionMetaFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNewWithStartup
		tmuxKillSessionFn = origKill
		ensureHeartbeatWritableFn = origEnsure
		saveSessionMetaFn = origSaveMeta
		appendSessionEventFn = origAppend
	})

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			args := append([]string{}, tt.args...)
			args = append(args, "--project-root", projectRoot)

			tmuxHasSessionFn = func(session string) bool { return false }
			tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
				return nil
			}
			tmuxKillSessionFn = func(session string) error { return nil }
			ensureHeartbeatWritableFn = func(path string) error { return os.WriteFile(path, []byte(""), 0o600) }
			saveSessionMetaFn = saveSessionMeta

			var reasons []string
			appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error {
				reasons = append(reasons, event.Reason)
				return nil
			}

			tt.setup(projectRoot)

			_, stderr := captureOutput(t, func() {
				if code := cmdSessionSpawn(args); code == 0 {
					t.Fatalf("expected spawn failure")
				}
			})
			if !strings.Contains(stderr, tt.wantErrMatch) {
				t.Fatalf("expected stderr %q, got %q", tt.wantErrMatch, stderr)
			}
			if len(reasons) == 0 {
				t.Fatalf("expected lifecycle failure event, got none")
			}
			if reasons[len(reasons)-1] != tt.wantReason {
				t.Fatalf("expected lifecycle reason %q, got %q (%v)", tt.wantReason, reasons[len(reasons)-1], reasons)
			}
		})
	}
}

func TestCmdSessionSpawnCapturesParentSessionFromEnv(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionWithStartupFn
	origSave := saveSessionMetaFn
	origParent := os.Getenv("LISA_SESSION_NAME")
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNew
		saveSessionMetaFn = origSave
		_ = os.Setenv("LISA_SESSION_NAME", origParent)
	})

	tmuxHasSessionFn = func(session string) bool { return false }
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		return nil
	}
	var saved sessionMeta
	saveSessionMetaFn = func(projectRoot, session string, meta sessionMeta) error {
		saved = meta
		return nil
	}
	if err := os.Setenv("LISA_SESSION_NAME", "lisa-parent-session"); err != nil {
		t.Fatalf("failed to set parent session env: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		if code := cmdSessionSpawn([]string{
			"--session", "lisa-child-session",
			"--project-root", t.TempDir(),
			"--command", "echo hello",
		}); code != 0 {
			t.Fatalf("expected spawn success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if saved.ParentSession != "lisa-parent-session" {
		t.Fatalf("expected parent session metadata, got %q", saved.ParentSession)
	}
}

func TestCmdSessionKillAlsoKillsNestedDescendants(t *testing.T) {
	origHas := tmuxHasSessionFn
	origKill := tmuxKillSessionFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxKillSessionFn = origKill
	})

	projectRoot := t.TempDir()
	root := "lisa-parent-root"
	child := "lisa-parent-child"
	grandchild := "lisa-parent-grandchild"
	for _, meta := range []sessionMeta{
		{Session: root, ProjectRoot: projectRoot, Agent: "claude", Mode: "interactive"},
		{Session: child, ParentSession: root, ProjectRoot: projectRoot, Agent: "claude", Mode: "interactive"},
		{Session: grandchild, ParentSession: child, ProjectRoot: projectRoot, Agent: "codex", Mode: "exec"},
	} {
		if err := saveSessionMeta(projectRoot, meta.Session, meta); err != nil {
			t.Fatalf("failed to seed metadata for %s: %v", meta.Session, err)
		}
	}

	alive := map[string]bool{
		root:       true,
		child:      true,
		grandchild: true,
	}
	var killed []string
	tmuxHasSessionFn = func(session string) bool {
		return alive[session]
	}
	tmuxKillSessionFn = func(session string) error {
		killed = append(killed, session)
		alive[session] = false
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		if code := cmdSessionKill([]string{"--session", root, "--project-root", projectRoot}); code != 0 {
			t.Fatalf("expected cascading kill success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	wantOrder := []string{grandchild, child, root}
	if len(killed) != len(wantOrder) {
		t.Fatalf("expected %d kills, got %d (%v)", len(wantOrder), len(killed), killed)
	}
	for i := range wantOrder {
		if killed[i] != wantOrder[i] {
			t.Fatalf("unexpected kill order at %d: got %q want %q (%v)", i, killed[i], wantOrder[i], killed)
		}
	}
}

func TestReadSessionEventTailRespectsWriterLockTimeout(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-read-tail-lock-timeout"
	path := sessionEventsFile(projectRoot, session)
	event := sessionEvent{
		At:      "2026-02-11T00:00:00Z",
		Type:    "snapshot",
		Session: session,
		State:   "in_progress",
		Status:  "active",
		Reason:  "ok",
		Poll:    1,
		Signals: statusSignals{},
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("failed writing event file: %v", err)
	}

	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("failed opening lock file: %v", err)
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed acquiring lock: %v", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck

	origTimeout := os.Getenv("LISA_EVENT_LOCK_TIMEOUT_MS")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENT_LOCK_TIMEOUT_MS", origTimeout)
	})
	if err := os.Setenv("LISA_EVENT_LOCK_TIMEOUT_MS", "50"); err != nil {
		t.Fatalf("failed setting lock timeout env: %v", err)
	}

	_, err = readSessionEventTail(projectRoot, session, 5)
	if err == nil {
		t.Fatalf("expected read tail timeout while writer lock is held")
	}
	if !strings.Contains(err.Error(), "event read lock timeout") {
		t.Fatalf("unexpected lock timeout error: %v", err)
	}
}

func TestReadSessionEventTailHandlesVeryLargeLine(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-large-tail-line"
	path := sessionEventsFile(projectRoot, session)

	largeReason := strings.Repeat("x", 3*1024*1024)
	eventLine := fmt.Sprintf(`{"at":"2026-02-11T00:00:00Z","type":"snapshot","session":"%s","state":"in_progress","status":"active","reason":"%s","poll":1,"signals":{}}`,
		session,
		largeReason,
	)
	if err := os.WriteFile(path, []byte(eventLine+"\n"), 0o600); err != nil {
		t.Fatalf("failed writing large-line event file: %v", err)
	}

	tail, err := readSessionEventTail(projectRoot, session, 10)
	if err != nil {
		t.Fatalf("expected large-line event tail read to succeed, got %v", err)
	}
	if len(tail.Events) != 1 {
		t.Fatalf("expected one decoded event, got %d", len(tail.Events))
	}
	if len(tail.Events[0].Reason) != len(largeReason) {
		t.Fatalf("expected large reason length %d, got %d", len(largeReason), len(tail.Events[0].Reason))
	}
	if tail.DroppedLines != 0 {
		t.Fatalf("expected no dropped lines, got %d", tail.DroppedLines)
	}
}

func TestPruneStaleSessionEventArtifactsRemovesOldLogs(t *testing.T) {
	origRetention := os.Getenv("LISA_EVENT_RETENTION_DAYS")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENT_RETENTION_DAYS", origRetention)
	})
	if err := os.Setenv("LISA_EVENT_RETENTION_DAYS", "1"); err != nil {
		t.Fatalf("failed setting retention env: %v", err)
	}

	oldPath := "/tmp/.lisa-old-session-lisa-old-events.jsonl"
	newPath := "/tmp/.lisa-new-session-lisa-new-events.jsonl"
	for _, path := range []string{oldPath, newPath} {
		if err := os.WriteFile(path, []byte(`{"type":"snapshot"}`+"\n"), 0o600); err != nil {
			t.Fatalf("failed seeding event file %s: %v", path, err)
		}
		if err := os.WriteFile(sessionEventCountFile(path), []byte("1"), 0o600); err != nil {
			t.Fatalf("failed seeding count file for %s: %v", path, err)
		}
		if err := os.WriteFile(path+".lock", []byte{}, 0o600); err != nil {
			t.Fatalf("failed seeding lock file for %s: %v", path, err)
		}
	}
	t.Cleanup(func() {
		for _, path := range []string{
			oldPath, sessionEventCountFile(oldPath), oldPath + ".lock",
			newPath, sessionEventCountFile(newPath), newPath + ".lock",
		} {
			_ = os.Remove(path)
		}
	})

	oldTime := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed setting old event mtime: %v", err)
	}
	if err := os.Chtimes(sessionEventCountFile(oldPath), oldTime, oldTime); err != nil {
		t.Fatalf("failed setting old count mtime: %v", err)
	}
	if err := os.Chtimes(oldPath+".lock", oldTime, oldTime); err != nil {
		t.Fatalf("failed setting old lock mtime: %v", err)
	}

	if err := pruneStaleSessionEventArtifacts(); err != nil {
		t.Fatalf("unexpected prune error: %v", err)
	}

	if fileExists(oldPath) || fileExists(sessionEventCountFile(oldPath)) || fileExists(oldPath+".lock") {
		t.Fatalf("expected stale event artifacts to be removed")
	}
	if !fileExists(newPath) || !fileExists(sessionEventCountFile(newPath)) || !fileExists(newPath+".lock") {
		t.Fatalf("expected fresh event artifacts to remain")
	}
}
