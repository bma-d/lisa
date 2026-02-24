package app

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestSessionSmokeInvalidContractProfile(t *testing.T) {
	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--contract-profile", "bad-profile",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "invalid_contract_profile" {
		t.Fatalf("expected invalid_contract_profile, got %v", payload["errorCode"])
	}
}

func TestSessionSmokeContractProfileFullSuccess(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 || args[0] != "session" {
			return "", "", fmt.Errorf("unexpected args: %v", args)
		}
		switch args[1] {
		case "spawn":
			return `{"session":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"completed","session":"lisa-smoke-l1","exitReason":"completed","polls":1,"finalStatus":"completed"}`, "", nil
		case "capture":
			return `{"capture":"LISA_SMOKE_L1_DONE=1\n"}`, "", nil
		case "tree":
			return `{"roots":[]}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		case "status":
			return `{"session":"lisa-smoke-l1","status":"idle","sessionState":"waiting_input"}`, "", nil
		case "packet":
			return `{"session":"lisa-smoke-l1","status":"idle","sessionState":"waiting_input","nextAction":"session send"}`, "", nil
		case "schema":
			return `{"command":"session packet","schema":{"type":"object"}}`, "", nil
		case "turn":
			return `{"ok":false,"errorCode":"missing_required_flag","error":"--session is required"}`, "", smokeExitErr(1)
		case "state-sandbox":
			if len(args) >= 3 && args[2] == "list" {
				return `{"action":"list","objectiveCount":0,"laneCount":0}`, "", nil
			}
			if len(args) >= 3 && args[2] == "restore" {
				return `{"ok":false,"errorCode":"missing_required_flag","error":"--file is required for restore"}`, "", smokeExitErr(1)
			}
			return "", "", fmt.Errorf("unexpected state-sandbox args: %v", args)
		default:
			return "", "", fmt.Errorf("unexpected session subcommand: %s (%v)", args[1], args)
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--contract-profile", "full",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"contractProfile":"full"`) {
		t.Fatalf("expected contractProfile in output, got %q", stdout)
	}
	payload := parseJSONMap(t, stdout)
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	checks, ok := payload["contractChecks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("expected contractChecks payload, got %v", payload["contractChecks"])
	}
}

func TestSessionSmokeContractProfileFullFailure(t *testing.T) {
	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})

	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		if len(args) < 2 || args[0] != "session" {
			return "", "", fmt.Errorf("unexpected args: %v", args)
		}
		switch args[1] {
		case "spawn":
			return `{"session":"ok"}`, "", nil
		case "monitor":
			return `{"finalState":"completed","session":"lisa-smoke-l1","exitReason":"completed","polls":1,"finalStatus":"completed"}`, "", nil
		case "capture":
			return `{"capture":"LISA_SMOKE_L1_DONE=1\n"}`, "", nil
		case "tree":
			return `{"roots":[]}`, "", nil
		case "kill":
			return `{"ok":true}`, "", nil
		case "status":
			return `{"session":"lisa-smoke-l1","status":"idle","sessionState":"waiting_input"}`, "", nil
		case "packet":
			// Missing nextAction -> contract failure.
			return `{"session":"lisa-smoke-l1","status":"idle","sessionState":"waiting_input"}`, "", nil
		case "schema":
			return `{"command":"session packet","schema":{"type":"object"}}`, "", nil
		case "turn":
			return `{"ok":false,"errorCode":"missing_required_flag","error":"--session is required"}`, "", smokeExitErr(1)
		case "state-sandbox":
			if len(args) >= 3 && args[2] == "list" {
				return `{"action":"list","objectiveCount":0,"laneCount":0}`, "", nil
			}
			if len(args) >= 3 && args[2] == "restore" {
				return `{"ok":false,"errorCode":"missing_required_flag","error":"--file is required for restore"}`, "", smokeExitErr(1)
			}
			return "", "", fmt.Errorf("unexpected state-sandbox args: %v", args)
		default:
			return "", "", fmt.Errorf("unexpected session subcommand: %s (%v)", args[1], args)
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSmoke([]string{
			"--project-root", t.TempDir(),
			"--levels", "1",
			"--contract-profile", "full",
			"--json",
		})
		if code != 1 {
			t.Fatalf("expected failure, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["errorCode"] != "smoke_contract_profile_failed" {
		t.Fatalf("expected smoke_contract_profile_failed, got %v", payload["errorCode"])
	}
}

func smokeExitErr(code int) error {
	cmd := exec.Command("bash", "-lc", fmt.Sprintf("exit %d", code))
	return cmd.Run()
}
