package app

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestE2ESessionSmokeFourLevels(t *testing.T) {
	for _, bin := range []string{"go", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available in PATH: %v", bin, err)
		}
	}

	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(t.TempDir(), "lisa")
	runAndRequireSuccess(t, repoRoot, nil, "go", "build", "-o", binPath, ".")

	out := runAndRequireSuccess(t, repoRoot, nil,
		binPath, "session", "smoke",
		"--project-root", repoRoot,
		"--levels", "4",
		"--poll-interval", "1",
		"--max-polls", "180",
		"--json",
	)

	var payload struct {
		OK             bool          `json:"ok"`
		Levels         int           `json:"levels"`
		Sessions       []string      `json:"sessions"`
		MissingMarkers []string      `json:"missingMarkers"`
		Monitor        monitorResult `json:"monitor"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("failed to parse smoke json: %v (%q)", err, out)
	}
	if !payload.OK {
		t.Fatalf("expected smoke success payload, got %q", out)
	}
	if payload.Levels != 4 {
		t.Fatalf("expected levels=4, got %d", payload.Levels)
	}
	if len(payload.Sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d (%q)", len(payload.Sessions), out)
	}
	if len(payload.MissingMarkers) != 0 {
		t.Fatalf("expected no missing markers, got %v (%q)", payload.MissingMarkers, out)
	}
	if payload.Monitor.FinalState != "completed" {
		t.Fatalf("expected completed monitor state, got %q (%q)", payload.Monitor.FinalState, out)
	}
}
