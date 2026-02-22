package app

import (
	"strings"
	"testing"
)

func TestHelpExitZero(t *testing.T) {
	commands := []struct {
		name string
		args []string
	}{
		{"top-level --help", []string{"--help"}},
		{"top-level -h", []string{"-h"}},
		{"top-level help", []string{"help"}},
		{"doctor --help", []string{"doctor", "--help"}},
		{"doctor -h", []string{"doctor", "-h"}},
		{"cleanup --help", []string{"cleanup", "--help"}},
		{"session --help", []string{"session", "--help"}},
		{"session spawn --help", []string{"session", "spawn", "--help"}},
		{"session detect-nested --help", []string{"session", "detect-nested", "--help"}},
		{"session send --help", []string{"session", "send", "--help"}},
		{"session snapshot --help", []string{"session", "snapshot", "--help"}},
		{"session status --help", []string{"session", "status", "--help"}},
		{"session explain --help", []string{"session", "explain", "--help"}},
		{"session monitor --help", []string{"session", "monitor", "--help"}},
		{"session capture --help", []string{"session", "capture", "--help"}},
		{"session context-pack --help", []string{"session", "context-pack", "--help"}},
		{"session route --help", []string{"session", "route", "--help"}},
		{"session autopilot --help", []string{"session", "autopilot", "--help"}},
		{"session smoke --help", []string{"session", "smoke", "--help"}},
		{"session preflight --help", []string{"session", "preflight", "--help"}},
		{"session list --help", []string{"session", "list", "--help"}},
		{"session exists --help", []string{"session", "exists", "--help"}},
		{"session kill --help", []string{"session", "kill", "--help"}},
		{"session kill-all --help", []string{"session", "kill-all", "--help"}},
		{"session name --help", []string{"session", "name", "--help"}},
		{"agent --help", []string{"agent", "--help"}},
		{"agent build-cmd --help", []string{"agent", "build-cmd", "--help"}},
		{"oauth --help", []string{"oauth", "--help"}},
		{"oauth add --help", []string{"oauth", "add", "--help"}},
		{"oauth list --help", []string{"oauth", "list", "--help"}},
		{"oauth remove --help", []string{"oauth", "remove", "--help"}},
		{"skills --help", []string{"skills", "--help"}},
		{"skills sync --help", []string{"skills", "sync", "--help"}},
		{"skills install --help", []string{"skills", "install", "--help"}},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = captureOutput(t, func() {
				code := Run(tc.args)
				if code != 0 {
					t.Fatalf("expected exit 0, got %d", code)
				}
			})
		})
	}
}

func TestHelpOutputContainsExpectedTokens(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		tokens []string
	}{
		{
			"top-level",
			[]string{"--help"},
			[]string{"lisa <command> [args]", "doctor", "cleanup", "session spawn", "session explain", "session smoke", "session preflight", "agent build-cmd", "skills sync"},
		},
		{
			"oauth",
			[]string{"oauth", "--help"},
			[]string{"lisa oauth", "add", "list", "remove"},
		},
		{
			"oauth add",
			[]string{"oauth", "add", "--help"},
			[]string{"lisa oauth add", "--token", "--stdin", "--json"},
		},
		{
			"skills",
			[]string{"skills", "--help"},
			[]string{"lisa skills", "sync", "install"},
		},
		{
			"skills sync",
			[]string{"skills", "sync", "--help"},
			[]string{"lisa skills sync", "--from", "--path", "--repo-root", "--json"},
		},
		{
			"skills install",
			[]string{"skills", "install", "--help"},
			[]string{"lisa skills install", "--to", "--project-path", "--path", "--repo-root", "--json"},
		},
		{
			"cleanup",
			[]string{"cleanup", "--help"},
			[]string{"lisa cleanup", "--dry-run", "--include-tmux-default", "--json"},
		},
		{
			"session",
			[]string{"session", "--help"},
			[]string{"lisa session", "spawn", "send", "status", "smoke", "preflight", "kill"},
		},
		{
			"session spawn",
			[]string{"session", "spawn", "--help"},
			[]string{"lisa session spawn", "--agent", "--mode", "--session", "--prompt", "--width", "--height", "--json"},
		},
		{
			"session monitor",
			[]string{"session", "monitor", "--help"},
			[]string{"lisa session monitor", "--until-jsonpath", "--json"},
		},
		{
			"session context-pack",
			[]string{"session", "context-pack", "--help"},
			[]string{"lisa session context-pack", "--from-handoff", "--token-budget", "--json"},
		},
		{
			"session route",
			[]string{"session", "route", "--help"},
			[]string{"lisa session route", "--budget", "--emit-runbook", "--json"},
		},
		{
			"session autopilot",
			[]string{"session", "autopilot", "--help"},
			[]string{"lisa session autopilot", "--capture-lines", "--kill-after", "--json"},
		},
		{
			"session smoke",
			[]string{"session", "smoke", "--help"},
			[]string{"lisa session smoke", "--levels", "--poll-interval", "--max-polls", "--json"},
		},
		{
			"session preflight",
			[]string{"session", "preflight", "--help"},
			[]string{"lisa session preflight", "--project-root", "--json"},
		},
		{
			"doctor",
			[]string{"doctor", "--help"},
			[]string{"lisa doctor", "--json"},
		},
		{
			"agent build-cmd",
			[]string{"agent", "build-cmd", "--help"},
			[]string{"lisa agent build-cmd", "--agent", "--mode", "--prompt", "--json"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr := captureOutput(t, func() {
				code := Run(tc.args)
				if code != 0 {
					t.Fatalf("expected exit 0, got %d", code)
				}
			})
			for _, token := range tc.tokens {
				if !strings.Contains(stderr, token) {
					t.Errorf("expected stderr to contain %q, got:\n%s", token, stderr)
				}
			}
		})
	}
}

func TestHelpRoutingEquivalence(t *testing.T) {
	cases := []struct {
		name string
		a    []string
		b    []string
		c    []string
	}{
		{
			"session spawn",
			[]string{"help", "session", "spawn"},
			[]string{"session", "spawn", "--help"},
			[]string{"session", "help", "spawn"},
		},
		{
			"session status",
			[]string{"help", "session", "status"},
			[]string{"session", "status", "-h"},
			[]string{"session", "help", "status"},
		},
		{
			"session preflight",
			[]string{"help", "session", "preflight"},
			[]string{"session", "preflight", "-h"},
			[]string{"session", "help", "preflight"},
		},
		{
			"agent build-cmd",
			[]string{"help", "agent", "build-cmd"},
			[]string{"agent", "build-cmd", "--help"},
			[]string{"agent", "help", "build-cmd"},
		},
		{
			"oauth add",
			[]string{"help", "oauth", "add"},
			[]string{"oauth", "add", "--help"},
			[]string{"oauth", "help", "add"},
		},
		{
			"skills sync",
			[]string{"help", "skills", "sync"},
			[]string{"skills", "sync", "--help"},
			[]string{"skills", "help", "sync"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderrA := captureOutput(t, func() {
				if code := Run(tc.a); code != 0 {
					t.Fatalf("route A exit %d", code)
				}
			})
			_, stderrB := captureOutput(t, func() {
				if code := Run(tc.b); code != 0 {
					t.Fatalf("route B exit %d", code)
				}
			})
			_, stderrC := captureOutput(t, func() {
				if code := Run(tc.c); code != 0 {
					t.Fatalf("route C exit %d", code)
				}
			})
			if stderrA != stderrB {
				t.Errorf("route A != route B:\nA: %s\nB: %s", stderrA, stderrB)
			}
			if stderrA != stderrC {
				t.Errorf("route A != route C:\nA: %s\nC: %s", stderrA, stderrC)
			}
		})
	}
}
