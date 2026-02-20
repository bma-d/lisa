package app

import (
	"errors"
	"os"
	"testing"
)

func TestComputeSessionStatusUsesPersistedPollCountForWaitingClassification(t *testing.T) {
	origHas := tmuxHasSessionFn
	origDisplay := tmuxDisplayFn
	origShowEnv := tmuxShowEnvironmentFn
	origDetect := detectAgentProcessFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxDisplayFn = origDisplay
		tmuxShowEnvironmentFn = origShowEnv
		detectAgentProcessFn = origDetect
	})

	tmuxHasSessionFn = func(session string) bool { return true }
	tmuxDisplayFn = func(session, format string) (string, error) {
		switch format {
		case "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}":
			return "0\t0\tzsh\t123", nil
		default:
			return "", nil
		}
	}
	tmuxShowEnvironmentFn = func(session, key string) (string, error) { return "", errors.New("missing") }
	detectAgentProcessFn = func(panePID int, agent string) (int, float64, error) {
		return 987, 0.01, nil
	}

	projectRoot := t.TempDir()
	session := "lisa-status-effective-poll"
	if err := saveSessionMeta(projectRoot, session, sessionMeta{
		Session:     session,
		Agent:       "codex",
		Mode:        "interactive",
		ProjectRoot: projectRoot,
		CreatedAt:   "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("failed to save meta: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(sessionMetaFile(projectRoot, session)) })
	t.Cleanup(func() { _ = os.Remove(sessionStateFile(projectRoot, session)) })
	if err := saveSessionState(sessionStateFile(projectRoot, session), sessionState{PollCount: 5}); err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	status, err := computeSessionStatus(session, projectRoot, "auto", "auto", false, 0)
	if err != nil {
		t.Fatalf("computeSessionStatus failed: %v", err)
	}
	if status.SessionState != "waiting_input" {
		t.Fatalf("expected waiting_input from persisted poll history, got %q", status.SessionState)
	}
	if status.ClassificationReason != "interactive_idle_cpu" {
		t.Fatalf("expected interactive_idle_cpu, got %q", status.ClassificationReason)
	}

	state, err := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}
	if state.PollCount != 6 {
		t.Fatalf("expected persisted poll count increment to 6, got %d", state.PollCount)
	}
}
