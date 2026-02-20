package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestExtractTmuxSocketPathsFromPS(t *testing.T) {
	psOut := strings.Join([]string{
		"/opt/homebrew/bin/tmux -S /tmp/lisa-tmux-a-123.sock new -d",
		"tmux -S /tmp/tmux-1000/default list-sessions",
		"python script.py",
		"/usr/bin/tmux -L dev list-sessions",
		"/usr/bin/tmux -S /tmp/lisa-tmux-a-123.sock has-session -t x",
	}, "\n")

	got := extractTmuxSocketPathsFromPS(psOut)
	want := []string{"/tmp/lisa-tmux-a-123.sock", "/tmp/tmux-1000/default"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected socket extraction; got=%v want=%v", got, want)
	}
}

func TestCmdCleanupApply(t *testing.T) {
	origCandidates := cleanupSocketCandidatesFn
	origProbe := probeTmuxSocketFn
	origKill := killTmuxSocketServerFn
	origRemove := removeSocketPathFn
	t.Cleanup(func() {
		cleanupSocketCandidatesFn = origCandidates
		probeTmuxSocketFn = origProbe
		killTmuxSocketServerFn = origKill
		removeSocketPathFn = origRemove
	})

	cleanupSocketCandidatesFn = func(includeTmuxDefault bool) ([]string, error) {
		if includeTmuxDefault {
			t.Fatalf("did not expect include default in this test")
		}
		return []string{"stale.sock", "detached.sock", "active.sock"}, nil
	}

	probeCalls := map[string]int{}
	probeTmuxSocketFn = func(socketPath string) (tmuxSocketProbe, error) {
		probeCalls[socketPath]++
		switch socketPath {
		case "stale.sock":
			return tmuxSocketProbe{Reachable: false}, nil
		case "detached.sock":
			if probeCalls[socketPath] == 1 {
				return tmuxSocketProbe{Reachable: true, Sessions: 1, Clients: 0}, nil
			}
			return tmuxSocketProbe{Reachable: false}, nil
		case "active.sock":
			return tmuxSocketProbe{Reachable: true, Sessions: 1, Clients: 2}, nil
		default:
			t.Fatalf("unexpected socket path probe: %s", socketPath)
			return tmuxSocketProbe{}, nil
		}
	}

	killCalls := []string{}
	killTmuxSocketServerFn = func(socketPath string) error {
		killCalls = append(killCalls, socketPath)
		return nil
	}
	removeCalls := []string{}
	removeSocketPathFn = func(path string) error {
		removeCalls = append(removeCalls, path)
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdCleanup([]string{"--json"})
		if code != 0 {
			t.Fatalf("expected cleanup success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload cleanupSummary
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to decode cleanup JSON output: %v (%q)", err, stdout)
	}
	if payload.Scanned != 3 || payload.Removed != 2 || payload.Killed != 1 || payload.KeptActive != 1 {
		t.Fatalf("unexpected cleanup summary: %+v", payload)
	}
	if !slices.Equal(killCalls, []string{"detached.sock"}) {
		t.Fatalf("unexpected kill calls: %v", killCalls)
	}
	if !slices.Equal(removeCalls, []string{"stale.sock", "detached.sock"}) {
		t.Fatalf("unexpected remove calls: %v", removeCalls)
	}
}

func TestCmdCleanupDryRunHasNoMutations(t *testing.T) {
	origCandidates := cleanupSocketCandidatesFn
	origProbe := probeTmuxSocketFn
	origKill := killTmuxSocketServerFn
	origRemove := removeSocketPathFn
	t.Cleanup(func() {
		cleanupSocketCandidatesFn = origCandidates
		probeTmuxSocketFn = origProbe
		killTmuxSocketServerFn = origKill
		removeSocketPathFn = origRemove
	})

	cleanupSocketCandidatesFn = func(includeTmuxDefault bool) ([]string, error) {
		if !includeTmuxDefault {
			t.Fatalf("expected include default flag")
		}
		return []string{"stale.sock", "detached.sock", "active.sock"}, nil
	}
	probeTmuxSocketFn = func(socketPath string) (tmuxSocketProbe, error) {
		switch socketPath {
		case "stale.sock":
			return tmuxSocketProbe{Reachable: false}, nil
		case "detached.sock":
			return tmuxSocketProbe{Reachable: true, Sessions: 1, Clients: 0}, nil
		case "active.sock":
			return tmuxSocketProbe{Reachable: true, Sessions: 1, Clients: 1}, nil
		default:
			t.Fatalf("unexpected socket path probe: %s", socketPath)
			return tmuxSocketProbe{}, nil
		}
	}

	killCalled := false
	killTmuxSocketServerFn = func(socketPath string) error {
		killCalled = true
		return nil
	}
	removeCalled := false
	removeSocketPathFn = func(path string) error {
		removeCalled = true
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdCleanup([]string{"--dry-run", "--include-tmux-default", "--json"})
		if code != 0 {
			t.Fatalf("expected cleanup dry-run success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload cleanupSummary
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to decode cleanup JSON output: %v (%q)", err, stdout)
	}
	if !payload.DryRun || payload.WouldRemove != 2 || payload.WouldKill != 1 || payload.Removed != 0 || payload.Killed != 0 {
		t.Fatalf("unexpected cleanup dry-run summary: %+v", payload)
	}
	if killCalled || removeCalled {
		t.Fatalf("dry-run should not mutate state; kill=%t remove=%t", killCalled, removeCalled)
	}
}
