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
		{"session --help", []string{"session", "--help"}},
		{"session spawn --help", []string{"session", "spawn", "--help"}},
		{"session send --help", []string{"session", "send", "--help"}},
		{"session status --help", []string{"session", "status", "--help"}},
		{"session explain --help", []string{"session", "explain", "--help"}},
		{"session monitor --help", []string{"session", "monitor", "--help"}},
		{"session capture --help", []string{"session", "capture", "--help"}},
		{"session list --help", []string{"session", "list", "--help"}},
		{"session exists --help", []string{"session", "exists", "--help"}},
		{"session kill --help", []string{"session", "kill", "--help"}},
		{"session kill-all --help", []string{"session", "kill-all", "--help"}},
		{"session name --help", []string{"session", "name", "--help"}},
		{"agent --help", []string{"agent", "--help"}},
		{"agent build-cmd --help", []string{"agent", "build-cmd", "--help"}},
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
			[]string{"lisa <command> [args]", "doctor", "session spawn", "session explain", "agent build-cmd"},
		},
		{
			"session",
			[]string{"session", "--help"},
			[]string{"lisa session", "spawn", "send", "status", "kill"},
		},
		{
			"session spawn",
			[]string{"session", "spawn", "--help"},
			[]string{"lisa session spawn", "--agent", "--mode", "--session", "--prompt", "--width", "--height", "--json"},
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
			"agent build-cmd",
			[]string{"help", "agent", "build-cmd"},
			[]string{"agent", "build-cmd", "--help"},
			[]string{"agent", "help", "build-cmd"},
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
