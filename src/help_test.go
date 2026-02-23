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
		{"capabilities --help", []string{"capabilities", "--help"}},
		{"version --help", []string{"version", "--help"}},
		{"help version", []string{"help", "version"}},
		{"--help version", []string{"--help", "version"}},
		{"-h version", []string{"-h", "version"}},
		{"session --help", []string{"session", "--help"}},
		{"session spawn --help", []string{"session", "spawn", "--help"}},
		{"session detect-nested --help", []string{"session", "detect-nested", "--help"}},
		{"session send --help", []string{"session", "send", "--help"}},
		{"session snapshot --help", []string{"session", "snapshot", "--help"}},
		{"session status --help", []string{"session", "status", "--help"}},
		{"session explain --help", []string{"session", "explain", "--help"}},
		{"session monitor --help", []string{"session", "monitor", "--help"}},
		{"session capture --help", []string{"session", "capture", "--help"}},
		{"session contract-check --help", []string{"session", "contract-check", "--help"}},
		{"session context-pack --help", []string{"session", "context-pack", "--help"}},
		{"session schema --help", []string{"session", "schema", "--help"}},
		{"session checkpoint --help", []string{"session", "checkpoint", "--help"}},
		{"session dedupe --help", []string{"session", "dedupe", "--help"}},
		{"session next --help", []string{"session", "next", "--help"}},
		{"session aggregate --help", []string{"session", "aggregate", "--help"}},
		{"session prompt-lint --help", []string{"session", "prompt-lint", "--help"}},
		{"session diff-pack --help", []string{"session", "diff-pack", "--help"}},
		{"session anomaly --help", []string{"session", "anomaly", "--help"}},
		{"session budget-enforce --help", []string{"session", "budget-enforce", "--help"}},
		{"session replay --help", []string{"session", "replay", "--help"}},
		{"session route --help", []string{"session", "route", "--help"}},
		{"session guard --help", []string{"session", "guard", "--help"}},
		{"session tree --help", []string{"session", "tree", "--help"}},
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
		{"skills doctor --help", []string{"skills", "doctor", "--help"}},
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
			"skills doctor",
			[]string{"skills", "doctor", "--help"},
			[]string{"lisa skills doctor", "--repo-root", "--fix", "--contract-check", "--json"},
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
			"capabilities",
			[]string{"capabilities", "--help"},
			[]string{"lisa capabilities", "Usage: lisa capabilities [flags]", "--json"},
		},
		{
			"version help route",
			[]string{"help", "version"},
			[]string{"lisa version", "Usage: lisa version", "Aliases:", "lisa --version", "lisa -v"},
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
			[]string{"lisa session monitor", "--until-jsonpath", "--event-budget", "--timeout-seconds", "--webhook", "--json"},
		},
		{
			"session explain",
			[]string{"session", "explain", "--help"},
			[]string{"lisa session explain", "--events", "--since", "--json"},
		},
		{
			"session contract-check",
			[]string{"session", "contract-check", "--help"},
			[]string{"lisa session contract-check", "--project-root", "--json"},
		},
		{
			"session schema",
			[]string{"session", "schema", "--help"},
			[]string{"lisa session schema", "--command", "--json"},
		},
		{
			"session checkpoint",
			[]string{"session", "checkpoint", "--help"},
			[]string{"lisa session checkpoint", "--file", "--session", "--json"},
		},
		{
			"session dedupe",
			[]string{"session", "dedupe", "--help"},
			[]string{"lisa session dedupe", "--task-hash", "--session", "--json"},
		},
		{
			"session next",
			[]string{"session", "next", "--help"},
			[]string{"lisa session next", "--session", "--budget", "--json"},
		},
		{
			"session aggregate",
			[]string{"session", "aggregate", "--help"},
			[]string{"lisa session aggregate", "--sessions", "--strategy", "--token-budget", "--dedupe", "--json"},
		},
		{
			"session prompt-lint",
			[]string{"session", "prompt-lint", "--help"},
			[]string{"lisa session prompt-lint", "--prompt", "--markers", "--budget", "--strict", "--json"},
		},
		{
			"session diff-pack",
			[]string{"session", "diff-pack", "--help"},
			[]string{"lisa session diff-pack", "--cursor-file", "--redact", "--json"},
		},
		{
			"session anomaly",
			[]string{"session", "anomaly", "--help"},
			[]string{"lisa session anomaly", "--session", "--events", "--json"},
		},
		{
			"session budget-enforce",
			[]string{"session", "budget-enforce", "--help"},
			[]string{"lisa session budget-enforce", "--max-tokens", "--from", "--json"},
		},
		{
			"session replay",
			[]string{"session", "replay", "--help"},
			[]string{"lisa session replay", "--from-checkpoint", "--json"},
		},
		{
			"session context-pack",
			[]string{"session", "context-pack", "--help"},
			[]string{"lisa session context-pack", "--from-handoff", "--redact", "--token-budget", "--json"},
		},
		{
			"session handoff",
			[]string{"session", "handoff", "--help"},
			[]string{"lisa session handoff", "--delta-from", "--compress", "--json"},
		},
		{
			"session route",
			[]string{"session", "route", "--help"},
			[]string{"lisa session route", "--profile", "--budget", "--topology", "--cost-estimate", "--emit-runbook", "--json"},
		},
		{
			"session guard",
			[]string{"session", "guard", "--help"},
			[]string{"lisa session guard", "--shared-tmux", "--command", "--project-root", "--machine-policy", "--json"},
		},
		{
			"session tree",
			[]string{"session", "tree", "--help"},
			[]string{"lisa session tree", "--delta-json", "--cursor-file", "--flat", "--with-state", "--json"},
		},
		{
			"session autopilot",
			[]string{"session", "autopilot", "--help"},
			[]string{"lisa session autopilot", "--capture-lines", "--kill-after", "--json"},
		},
		{
			"session smoke",
			[]string{"session", "smoke", "--help"},
			[]string{"lisa session smoke", "--levels", "--chaos-report", "--poll-interval", "--max-polls", "--export-artifacts", "--json"},
		},
		{
			"session preflight",
			[]string{"session", "preflight", "--help"},
			[]string{"lisa session preflight", "--project-root", "--json"},
		},
		{
			"session list",
			[]string{"session", "list", "--help"},
			[]string{"lisa session list", "--watch-json", "--watch-interval", "--watch-cycles", "--json"},
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

func TestHelpUnknownRoute(t *testing.T) {
	_, stderr := captureOutput(t, func() {
		code := Run([]string{"help", "bogus"})
		if code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
	})
	if !strings.Contains(stderr, "no help available for") {
		t.Fatalf("expected no-help error, got %q", stderr)
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
			"version",
			[]string{"help", "version"},
			[]string{"--help", "version"},
			[]string{"-h", "version"},
		},
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
