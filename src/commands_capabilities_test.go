package app

import (
	"encoding/json"
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
	if len(payload.Commands) == 0 {
		t.Fatalf("expected commands list, got empty payload")
	}
	found := false
	for _, cmd := range payload.Commands {
		if cmd.Name == "session list" { // spot check a few known commands
			found = true
			if len(cmd.Flags) == 0 || !strings.Contains(strings.Join(cmd.Flags, " "), "--json") {
				t.Fatalf("expected session list flags to include --json, got %#v", cmd.Flags)
			}
		}
	}
	if !found {
		t.Fatalf("session list entry missing from capabilities payload")
	}
}
