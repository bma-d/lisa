package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTmuxPaneStatusBranches(t *testing.T) {
	origDisplay := tmuxDisplayFn
	t.Cleanup(func() {
		tmuxDisplayFn = origDisplay
	})

	tmuxDisplayFn = func(session, format string) (string, error) {
		if format == "#{pane_dead}" {
			return "1", nil
		}
		return "7", nil
	}
	got, err := tmuxPaneStatus("lisa-pane")
	if err != nil {
		t.Fatalf("tmuxPaneStatus error: %v", err)
	}
	if got != "exited:7" {
		t.Fatalf("expected exited:7, got %q", got)
	}

	tmuxDisplayFn = func(session, format string) (string, error) {
		if format == "#{pane_dead}" {
			return "0", nil
		}
		return "9", nil
	}
	got, err = tmuxPaneStatus("lisa-pane")
	if err != nil {
		t.Fatalf("tmuxPaneStatus error: %v", err)
	}
	if got != "crashed:9" {
		t.Fatalf("expected crashed:9, got %q", got)
	}
}

func TestApplyTerminalPaneStatusBranches(t *testing.T) {
	in := sessionStatus{Status: "active", SessionState: "in_progress"}

	out, terminal := applyTerminalPaneStatus(in, "crashed:1")
	if !terminal || out.SessionState != "crashed" {
		t.Fatalf("expected crashed terminal classification, got %+v terminal=%t", out, terminal)
	}

	out, terminal = applyTerminalPaneStatus(in, "exited:0")
	if !terminal || out.SessionState != "completed" {
		t.Fatalf("expected completed terminal classification, got %+v terminal=%t", out, terminal)
	}

	out, terminal = applyTerminalPaneStatus(in, "alive")
	if terminal {
		t.Fatalf("expected non-terminal for alive, got %+v", out)
	}
}

func TestResolveAgentModeAndEstimateWaitBranches(t *testing.T) {
	origShowEnv := tmuxShowEnvironmentFn
	t.Cleanup(func() {
		tmuxShowEnvironmentFn = origShowEnv
	})

	tmuxShowEnvironmentFn = func(session, key string) (string, error) {
		if key == "LISA_AGENT" {
			return "codex", nil
		}
		if key == "LISA_MODE" {
			return "exec", nil
		}
		return "", errors.New("not set")
	}
	if got := resolveAgent("auto", sessionMeta{}, "lisa", ""); got != "codex" {
		t.Fatalf("expected env-resolved agent codex, got %q", got)
	}
	if got := resolveMode("auto", sessionMeta{}, "lisa", ""); got != "exec" {
		t.Fatalf("expected env-resolved mode exec, got %q", got)
	}
	if got := resolveAgent("auto", sessionMeta{}, "", ""); got == "" {
		t.Fatalf("expected non-empty default agent")
	}

	if got := estimateWait("reading docs", 0, 0); got != 30 {
		t.Fatalf("expected read wait 30, got %d", got)
	}
	if got := estimateWait("running tests", 0, 0); got != 120 {
		t.Fatalf("expected build wait 120, got %d", got)
	}
	if got := estimateWait("writing patch", 0, 0); got != 60 {
		t.Fatalf("expected write wait 60, got %d", got)
	}
	if got := estimateWait("", 1, 10); got != 90 {
		t.Fatalf("expected low progress wait 90, got %d", got)
	}
	if got := estimateWait("", 8, 10); got != 30 {
		t.Fatalf("expected high progress wait 30, got %d", got)
	}
}

func TestParseBoolFlagAllBranches(t *testing.T) {
	if v, err := parseBoolFlag("true"); err != nil || !v {
		t.Fatalf("expected parseBoolFlag(true)=true,nil got %t,%v", v, err)
	}
	if v, err := parseBoolFlag("false"); err != nil || v {
		t.Fatalf("expected parseBoolFlag(false)=false,nil got %t,%v", v, err)
	}
	if _, err := parseBoolFlag("yes"); err == nil {
		t.Fatalf("expected parseBoolFlag invalid error")
	}
}

func TestComputeSessionStatusFullUsesSingleCapture(t *testing.T) {
	origHas := tmuxHasSessionFn
	origDisplay := tmuxDisplayFn
	origCapture := tmuxCapturePaneFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	origAppend := appendSessionEventFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxDisplayFn = origDisplay
		tmuxCapturePaneFn = origCapture
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
		appendSessionEventFn = origAppend
	})

	projectRoot := t.TempDir()
	session := "lisa-single-capture"
	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxDisplayFn = func(session, format string) (string, error) {
		if format == "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}" {
			return "1\t0\tzsh\t123", nil
		}
		if format == "#{pane_current_command}" {
			return "zsh", nil
		}
		if format == "#{pane_pid}" {
			return "123", nil
		}
		return "", nil
	}
	captureCalls := 0
	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		captureCalls++
		return "done line", nil
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) { return 0, 0, nil }
	appendSessionEventFn = func(projectRoot, session string, event sessionEvent) error { return nil }

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", true, 1)
	if err != nil {
		t.Fatalf("computeSessionStatus error: %v", err)
	}
	if status.SessionState != "completed" {
		t.Fatalf("expected completed from pane exited:0, got %s", status.SessionState)
	}
	if captureCalls != 1 {
		t.Fatalf("expected one capture call with full output write, got %d", captureCalls)
	}
	if status.OutputFile == "" {
		t.Fatalf("expected output file path on terminal full status")
	}
	if _, err := os.Stat(status.OutputFile); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
}

func TestAppendSessionEventMaintainsLineCounter(t *testing.T) {
	origMaxBytes := os.Getenv("LISA_EVENTS_MAX_BYTES")
	origMaxLines := os.Getenv("LISA_EVENTS_MAX_LINES")
	t.Cleanup(func() {
		_ = os.Setenv("LISA_EVENTS_MAX_BYTES", origMaxBytes)
		_ = os.Setenv("LISA_EVENTS_MAX_LINES", origMaxLines)
	})
	_ = os.Setenv("LISA_EVENTS_MAX_BYTES", "100000")
	_ = os.Setenv("LISA_EVENTS_MAX_LINES", "3")

	projectRoot := t.TempDir()
	session := "lisa-counted-events"
	for i := 0; i < 5; i++ {
		err := appendSessionEvent(projectRoot, session, sessionEvent{
			At:      time.Now().UTC().Format(time.RFC3339Nano),
			Type:    "snapshot",
			Session: session,
			State:   "in_progress",
			Status:  "active",
			Reason:  "line_counter",
			Poll:    i + 1,
		})
		if err != nil {
			t.Fatalf("appendSessionEvent failed: %v", err)
		}
	}

	countPath := sessionEventCountFile(sessionEventsFile(projectRoot, session))
	raw, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("failed to read event line counter file: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "3" {
		t.Fatalf("expected retained line count 3, got %q", string(raw))
	}

	_ = os.Remove(countPath)
	if err := appendSessionEvent(projectRoot, session, sessionEvent{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Type:    "snapshot",
		Session: session,
		State:   "in_progress",
		Status:  "active",
		Reason:  "line_counter_bootstrap",
		Poll:    99,
	}); err != nil {
		t.Fatalf("append after counter file removal failed: %v", err)
	}
	raw, err = os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("failed to read rebuilt counter file: %v", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		t.Fatalf("expected rebuilt counter file to have a value")
	}

	tail, err := readSessionEventTail(projectRoot, session, 10)
	if err != nil {
		t.Fatalf("readSessionEventTail failed: %v", err)
	}
	if len(tail.Events) > 3 {
		t.Fatalf("expected bounded tail <= 3 events, got %d", len(tail.Events))
	}
}

func TestStateLockTimeoutErrorString(t *testing.T) {
	err := (&stateLockTimeoutError{WaitMS: 123}).Error()
	if !strings.Contains(err, "123ms") {
		t.Fatalf("unexpected state lock timeout error string: %q", err)
	}
}

func TestWriteFileAtomicConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic.txt")
	payloads := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		payloads = append(payloads, fmt.Sprintf("value-%02d", i))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(payloads))
	for i := range payloads {
		wg.Add(1)
		data := payloads[i]
		go func() {
			defer wg.Done()
			errCh <- writeFileAtomic(path, []byte(data))
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("writeFileAtomic concurrent write failed: %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read atomically written file: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	if got == "" {
		t.Fatalf("expected non-empty final content")
	}

	valid := false
	for _, expected := range payloads {
		if got == expected {
			valid = true
			break
		}
	}
	if !valid {
		t.Fatalf("final content %q did not match any writer payload", got)
	}
}
