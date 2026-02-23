package app

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func TestCmdCapabilitiesJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdCapabilities([]string{"--json"})
		if code != 0 {
			t.Fatalf("expected capabilities to succeed, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Date     string `json:"date"`
		Now      string `json:"generatedAt"`
		Commands []struct {
			Name  string   `json:"name"`
			Flags []string `json:"flags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse capabilities json: %v (%q)", err, stdout)
	}
	if payload.Version == "" {
		t.Fatalf("expected version in payload, got %q", stdout)
	}
	if payload.Commit == "" || payload.Date == "" || payload.Now == "" {
		t.Fatalf("expected build metadata fields in payload, got %q", stdout)
	}
	if len(payload.Commands) == 0 {
		t.Fatalf("expected commands list, got empty payload")
	}
	foundList := false
	foundCapabilities := false
	for _, cmd := range payload.Commands {
		if cmd.Name == "capabilities" {
			foundCapabilities = true
		}
		if cmd.Name == "session list" { // spot check a few known commands
			foundList = true
			if len(cmd.Flags) == 0 || !strings.Contains(strings.Join(cmd.Flags, " "), "--json") {
				t.Fatalf("expected session list flags to include --json, got %#v", cmd.Flags)
			}
		}
	}
	if !foundList {
		t.Fatalf("session list entry missing from capabilities payload")
	}
	if !foundCapabilities {
		t.Fatalf("capabilities entry missing from capabilities payload")
	}
}

func TestCmdCapabilitiesUnknownFlagJSON(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdCapabilities([]string{"--json", "--badflag"})
		if code == 0 {
			t.Fatalf("expected capabilities to fail on unknown flag")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var payload struct {
		OK        bool   `json:"ok"`
		ErrorCode string `json:"errorCode"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("failed to parse error json: %v (%q)", err, stdout)
	}
	if payload.OK {
		t.Fatalf("expected ok=false, got true")
	}
	if payload.ErrorCode != "unknown_flag" {
		t.Fatalf("expected unknown_flag, got %q", payload.ErrorCode)
	}
	if !strings.Contains(payload.Error, "--badflag") {
		t.Fatalf("expected error to include unknown flag, got %q", payload.Error)
	}
}

func TestCmdCapabilitiesUnknownFlagText(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdCapabilities([]string{"--badflag"})
		if code == 0 {
			t.Fatalf("expected capabilities to fail on unknown flag")
		}
	})
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "unknown flag: --badflag") {
		t.Fatalf("expected unknown flag stderr, got %q", stderr)
	}
}

func TestCmdCapabilitiesHelp(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdCapabilities([]string{"--help"})
		if code != 0 {
			t.Fatalf("expected help to succeed, got %d", code)
		}
	})
	if stdout != "" {
		t.Fatalf("expected empty stdout, got %q", stdout)
	}
	for _, token := range []string{"lisa capabilities", "Usage: lisa capabilities [flags]", "--json"} {
		if !strings.Contains(stderr, token) {
			t.Fatalf("expected help output token %q, got %q", token, stderr)
		}
	}
}

func TestCommandCapabilitiesCommandSet(t *testing.T) {
	var got []string
	for _, c := range commandCapabilities {
		got = append(got, c.Name)
	}
	sort.Strings(got)

	want := []string{
		"agent build-cmd",
		"capabilities",
		"cleanup",
		"doctor",
		"oauth add",
		"oauth list",
		"oauth remove",
		"session capture",
		"session anomaly",
		"session aggregate",
		"session budget-observe",
		"session budget-enforce",
		"session budget-plan",
		"session checkpoint",
		"session context-pack",
		"session context-cache",
		"session contract-check",
		"session dedupe",
		"session detect-nested",
		"session diff-pack",
		"session exists",
		"session explain",
		"session autopilot",
		"session guard",
		"session handoff",
		"session kill",
		"session kill-all",
		"session lane",
		"session list",
		"session loop",
		"session memory",
		"session monitor",
		"session name",
		"session next",
		"session objective",
		"session packet",
		"session preflight",
		"session prompt-lint",
		"session replay",
		"session route",
		"session schema",
		"session send",
		"session snapshot",
		"session smoke",
		"session spawn",
		"session status",
		"session tree",
		"skills doctor",
		"skills install",
		"skills sync",
		"version",
	}
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected capabilities command set\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCommandCapabilitiesCriticalFlagContracts(t *testing.T) {
	required := map[string][]string{
		"session monitor":        {"--until-jsonpath", "--event-budget", "--webhook", "--timeout-seconds", "--auto-recover"},
		"session explain":        {"--since"},
		"session context-pack":   {"--from-handoff", "--redact"},
		"session route":          {"--budget", "--topology", "--cost-estimate", "--profile", "--queue", "--concurrency"},
		"session packet":         {"--fields"},
		"session capture":        {"--semantic-delta"},
		"session smoke":          {"--chaos-report", "--export-artifacts", "--llm-profile"},
		"session guard":          {"--shared-tmux", "--command", "--project-root", "--machine-policy", "--policy-file"},
		"session list":           {"--priority", "--watch-json", "--watch-interval", "--watch-cycles"},
		"session tree":           {"--delta-json", "--cursor-file"},
		"session schema":         {"--command"},
		"session checkpoint":     {"--file"},
		"session dedupe":         {"--task-hash"},
		"session next":           {"--budget"},
		"session aggregate":      {"--token-budget", "--dedupe", "--delta-json", "--cursor-file"},
		"session prompt-lint":    {"--markers", "--strict", "--rewrite"},
		"session diff-pack":      {"--cursor-file", "--redact", "--semantic-only"},
		"session loop":           {"--session", "--cursor-file", "--schema"},
		"session context-cache":  {"--key", "--refresh", "--from"},
		"session anomaly":        {"--events", "--auto-remediate"},
		"session budget-observe": {"--from", "--tokens"},
		"session budget-enforce": {"--max-tokens", "--from"},
		"session budget-plan":    {"--goal", "--topology"},
		"session replay":         {"--from-checkpoint"},
		"session handoff":        {"--compress", "--schema"},
		"session contract-check": {"--project-root"},
		"session objective":      {"--goal", "--activate", "--ttl-hours"},
		"session memory":         {"--session", "--refresh", "--semantic-diff"},
		"session lane":           {"--name", "--contract", "--clear"},
		"session autopilot":      {"--lane", "--json"},
		"skills doctor":          {"--fix", "--contract-check", "--sync-plan"},
	}

	flagsByCommand := map[string][]string{}
	for _, entry := range commandCapabilities {
		flagsByCommand[entry.Name] = entry.Flags
	}

	for command, requiredFlags := range required {
		gotFlags, ok := flagsByCommand[command]
		if !ok {
			t.Fatalf("missing command %q in capabilities table", command)
		}
		for _, flag := range requiredFlags {
			found := false
			for _, got := range gotFlags {
				if got == flag {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("command %q missing required flag %q in capabilities table: %#v", command, flag, gotFlags)
			}
		}
	}
}
