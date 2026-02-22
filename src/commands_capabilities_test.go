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
		"session capture",
		"session context-pack",
		"session detect-nested",
		"session exists",
		"session explain",
		"session guard",
		"session handoff",
		"session kill",
		"session kill-all",
		"session list",
		"session monitor",
		"session name",
		"session preflight",
		"session route",
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
