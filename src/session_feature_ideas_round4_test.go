package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildSessionPackSnapshotUsesProjectRuntimeEnv(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-pack-env"
	expectedSocket := tmuxSocketPathForProjectRoot(projectRoot)

	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		if got := os.Getenv(lisaTmuxSocketEnv); got != expectedSocket {
			return sessionStatus{}, fmt.Errorf("socket env mismatch: got=%s want=%s", got, expectedSocket)
		}
		return sessionStatus{
			Session:              sessionName,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "heartbeat_fresh",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	snapshot, err := buildSessionPackSnapshot(session, projectRoot, "auto", "auto", contextPackStrategyConfig{Name: "balanced"}, 4, 80, 300, nil)
	if err != nil {
		t.Fatalf("expected snapshot build success, got %v", err)
	}
	if snapshot.SessionState != "in_progress" {
		t.Fatalf("expected in_progress snapshot, got %+v", snapshot)
	}
}

func TestCmdSessionAggregateDeltaJSON(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-agg-delta"
	cursor := filepath.Join(projectRoot, "aggregate.cursor.json")

	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})
	call := 0
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		call++
		reason := "reason-a"
		if call > 1 {
			reason = "reason-b"
		}
		return sessionStatus{
			Session:              sessionName,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: reason,
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout1, stderr1 := captureOutput(t, func() {
		code := cmdSessionAggregate([]string{
			"--sessions", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursor,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr1 != "" {
		t.Fatalf("unexpected stderr: %q", stderr1)
	}
	payload1 := parseJSONMap(t, stdout1)
	delta1 := payload1["delta"].(map[string]any)
	if delta1["count"].(float64) != 1 {
		t.Fatalf("expected first delta count=1, got %v", delta1["count"])
	}

	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionAggregate([]string{
			"--sessions", session,
			"--project-root", projectRoot,
			"--delta-json",
			"--cursor-file", cursor,
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected second success, got %d", code)
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	payload2 := parseJSONMap(t, stdout2)
	delta2 := payload2["delta"].(map[string]any)
	if delta2["count"].(float64) != 1 {
		t.Fatalf("expected second delta count=1, got %v", delta2["count"])
	}
}

func TestCmdSessionPromptLintRewriteFlag(t *testing.T) {
	stdoutNoRewrite, stderrNoRewrite := captureOutput(t, func() {
		code := cmdSessionPromptLint([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "Use ./lisa recursively inside lisa.",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderrNoRewrite != "" {
		t.Fatalf("unexpected stderr: %q", stderrNoRewrite)
	}
	if strings.Contains(stdoutNoRewrite, `"rewrites"`) {
		t.Fatalf("did not expect rewrites without --rewrite, got %q", stdoutNoRewrite)
	}

	stdoutRewrite, stderrRewrite := captureOutput(t, func() {
		code := cmdSessionPromptLint([]string{
			"--agent", "codex",
			"--mode", "exec",
			"--prompt", "Use ./lisa recursively inside lisa.",
			"--rewrite",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderrRewrite != "" {
		t.Fatalf("unexpected stderr: %q", stderrRewrite)
	}
	if !strings.Contains(stdoutRewrite, `"rewrites"`) || !strings.Contains(stdoutRewrite, `"recommendedPrompt"`) {
		t.Fatalf("expected rewrite payload with --rewrite, got %q", stdoutRewrite)
	}
}

func TestCmdSessionHandoffSchemaV3IncludesDeterministicIDs(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-handoff-v3"

	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:              sessionName,
			Status:               "idle",
			SessionState:         "waiting_input",
			ClassificationReason: "interactive_idle_cpu",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionHandoff([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--schema", "v3",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["schema"] != "v3" {
		t.Fatalf("expected schema v3, got %v", payload["schema"])
	}
	nextAction := payload["nextAction"].(map[string]any)
	if !strings.HasPrefix(mapStringValue(nextAction, "id"), "hid-") {
		t.Fatalf("expected deterministic id in nextAction, got %v", nextAction)
	}
	risks := payload["risks"].([]any)
	if len(risks) == 0 || !strings.HasPrefix(mapStringValue(risks[0].(map[string]any), "id"), "hid-") {
		t.Fatalf("expected deterministic ids in risks, got %v", payload["risks"])
	}
}

func TestCmdSessionRouteConcurrencyDispatchPlan(t *testing.T) {
	projectRoot := t.TempDir()

	origList := tmuxListSessionsFn
	origCompute := computeSessionStatusFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		computeSessionStatusFn = origCompute
	})
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{"lisa-r1", "lisa-r2", "lisa-r3"}, nil
	}
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		switch sessionName {
		case "lisa-r1":
			return sessionStatus{Session: sessionName, Status: "idle", SessionState: "waiting_input"}, nil
		case "lisa-r2":
			return sessionStatus{Session: sessionName, Status: "active", SessionState: "in_progress"}, nil
		default:
			return sessionStatus{Session: sessionName, Status: "completed", SessionState: "completed"}, nil
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--queue",
			"--concurrency", "2",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	if payload["concurrency"].(float64) != 2 {
		t.Fatalf("expected concurrency=2, got %v", payload["concurrency"])
	}
	queue := payload["queue"].([]any)
	if len(queue) != 3 {
		t.Fatalf("expected queue size 3, got %d", len(queue))
	}
	last := queue[2].(map[string]any)
	if last["dispatchWave"].(float64) != 2 {
		t.Fatalf("expected third item in wave 2, got %v", last)
	}
	dispatchPlan := payload["dispatchPlan"].([]any)
	if len(dispatchPlan) != 2 {
		t.Fatalf("expected 2 dispatch waves, got %v", dispatchPlan)
	}
}

func TestCmdSessionRouteQueueNonJSONPrintsAmbiguousWarning(t *testing.T) {
	projectRoot := t.TempDir()

	origList := tmuxListSessionsFn
	origMeta := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		loadSessionMetaByGlobFn = origMeta
	})
	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{"lisa-route-ambiguous"}, nil
	}
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{}, &sessionMetaAmbiguousError{Session: session}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionRoute([]string{
			"--goal", "analysis",
			"--project-root", projectRoot,
			"--queue",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if !strings.Contains(stderr, "multiple metadata files found") || !strings.Contains(stderr, "lisa-route-ambiguous") {
		t.Fatalf("expected ambiguous warning in stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "goal=analysis") {
		t.Fatalf("expected command output, got %q", stdout)
	}
}

func TestCmdSessionMemorySemanticDiff(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-memory-semantic-diff"
	if err := saveSessionMemory(projectRoot, session, sessionMemoryRecord{
		Session:   session,
		UpdatedAt: "2026-02-20T00:00:00Z",
		ExpiresAt: "2099-01-01T00:00:00Z",
		MaxLines:  80,
		Lines:     []string{"old-a", "shared-b"},
	}); err != nil {
		t.Fatalf("save memory: %v", err)
	}
	if err := os.WriteFile(sessionOutputFile(projectRoot, session), []byte("shared-b\nnew-c\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionMemory([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--semantic-diff",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"semanticDiff"`) || !strings.Contains(stdout, `"added"`) || !strings.Contains(stdout, `"removed"`) {
		t.Fatalf("expected semantic diff payload, got %q", stdout)
	}
}

func TestCmdSessionAnomalyAutoRemediate(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-anomaly-remediate"

	origCompute := computeSessionStatusFn
	origReadTail := readSessionEventTailFn
	t.Cleanup(func() {
		computeSessionStatusFn = origCompute
		readSessionEventTailFn = origReadTail
	})
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{Session: sessionName, Status: "crashed", SessionState: "crashed"}, nil
	}
	readSessionEventTailFn = func(projectRoot, session string, max int) (sessionEventTail, error) {
		return sessionEventTail{Events: []sessionEvent{{State: "crashed", Status: "failed", Reason: "boom"}}}, nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionAnomaly([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--auto-remediate",
			"--json",
		})
		if code == 0 {
			t.Fatalf("expected anomaly findings to fail")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"autoRemediate"`) || !strings.Contains(stdout, "session explain") {
		t.Fatalf("expected remediation plan in payload, got %q", stdout)
	}
}

func TestCmdSessionBudgetObserveFromPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obs.json")
	if err := os.WriteFile(path, []byte(`{"totalTokens":120,"totalSeconds":33,"steps":[{"id":"a"},{"id":"b"}]}`), 0o644); err != nil {
		t.Fatalf("write observe payload: %v", err)
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionBudgetObserve([]string{"--from", path, "--json"})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	observed := payload["observed"].(map[string]any)
	if observed["tokens"].(float64) != 120 || observed["seconds"].(float64) != 33 || observed["steps"].(float64) != 2 {
		t.Fatalf("unexpected observed payload: %v", observed)
	}
}

func TestCmdSessionContextCacheRefreshAndRead(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-context-cache-refresh"
	if err := os.WriteFile(sessionOutputFile(projectRoot, session), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	origHas := tmuxHasSessionFn
	t.Cleanup(func() { tmuxHasSessionFn = origHas })
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout1, stderr1 := captureOutput(t, func() {
		code := cmdSessionContextCache([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--refresh",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected refresh success, got %d", code)
		}
	})
	if stderr1 != "" {
		t.Fatalf("unexpected stderr: %q", stderr1)
	}
	if !strings.Contains(stdout1, `"action":"updated"`) {
		t.Fatalf("expected updated action, got %q", stdout1)
	}

	stdout2, stderr2 := captureOutput(t, func() {
		code := cmdSessionContextCache([]string{
			"--key", "session:" + session,
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected read success, got %d", code)
		}
	})
	if stderr2 != "" {
		t.Fatalf("unexpected stderr: %q", stderr2)
	}
	if !strings.Contains(stdout2, `"exists":true`) || !strings.Contains(stdout2, `"lineCount"`) {
		t.Fatalf("expected cache hit payload, got %q", stdout2)
	}
}

func TestCmdSkillsDoctorSyncPlan(t *testing.T) {
	origHome := osUserHomeDirFn
	origVersion := BuildVersion
	t.Cleanup(func() {
		osUserHomeDirFn = origHome
		BuildVersion = origVersion
	})
	BuildVersion = "dev"

	repoRoot := t.TempDir()
	repoSkill := filepath.Join(repoRoot, "skills", lisaSkillName)
	writeSkillFixture(t, repoSkill, "2.0.0")

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	codexPath, err := defaultSkillInstallPath("codex")
	if err != nil {
		t.Fatalf("codex path: %v", err)
	}
	writeSkillFixture(t, codexPath, "1.0.0")

	stdout, stderr := captureOutput(t, func() {
		code := cmdSkillsDoctor([]string{"--repo-root", repoRoot, "--sync-plan", "--json"})
		if code == 0 {
			t.Fatalf("expected drift failure")
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"syncPlan"`) || !strings.Contains(stdout, "skills install") {
		t.Fatalf("expected sync plan payload, got %q", stdout)
	}
}

func TestCmdSessionLoopJSONMin(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-loop"

	origRun := runLisaSubcommandFn
	origExe := osExecutableFn
	t.Cleanup(func() {
		runLisaSubcommandFn = origRun
		osExecutableFn = origExe
	})
	osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
	calls := make([][]string, 0, 8)
	runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
		copied := append([]string{}, args...)
		calls = append(calls, copied)
		if len(args) < 2 {
			return "", "", fmt.Errorf("invalid args")
		}
		switch args[1] {
		case "monitor":
			return `{"finalState":"waiting_input","exitReason":"waiting_input"}`, "", nil
		case "diff-pack":
			return `{"changed":true,"tokenBudget":120}`, "", nil
		case "handoff":
			return `{"deltaCount":1,"nextDeltaOffset":4}`, "", nil
		case "next":
			return `{"nextAction":"session send","recommendedCommand":"./lisa session send --json-min"}`, "", nil
		default:
			return "", "", fmt.Errorf("unexpected subcommand %q", args[1])
		}
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionLoop([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--steps", "2",
			"--max-tokens", "1000",
			"--json-min",
		})
		if code != 0 {
			t.Fatalf("expected success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"completedSteps":2`) {
		t.Fatalf("expected completed steps in payload, got %q", stdout)
	}
	foundDiffCursor := false
	for _, call := range calls {
		if len(call) >= 2 && call[0] == "session" && call[1] == "diff-pack" {
			for i := 0; i < len(call); i++ {
				if call[i] == "--cursor-file" && i+1 < len(call) && strings.TrimSpace(call[i+1]) != "" {
					foundDiffCursor = true
					break
				}
			}
		}
	}
	if !foundDiffCursor {
		t.Fatalf("expected loop to pass --cursor-file to diff-pack, calls=%v", calls)
	}
}

func TestCmdSessionLoopBudgetCapsStopAtLimit(t *testing.T) {
	baseNow := time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		limitFlag    string
		limitValue   string
		tokenBudget  int
		expectedRisk string
	}{
		{name: "tokens", limitFlag: "--max-tokens", limitValue: "100", tokenBudget: 100, expectedRisk: "tokens"},
		{name: "seconds", limitFlag: "--max-seconds", limitValue: "1", tokenBudget: 40, expectedRisk: "seconds"},
		{name: "steps", limitFlag: "--max-steps", limitValue: "1", tokenBudget: 40, expectedRisk: "steps"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot := t.TempDir()
			session := "lisa-loop-limit-" + tc.name

			origRun := runLisaSubcommandFn
			origExe := osExecutableFn
			origNow := nowFn
			t.Cleanup(func() {
				runLisaSubcommandFn = origRun
				osExecutableFn = origExe
				nowFn = origNow
			})

			osExecutableFn = func() (string, error) { return "/tmp/lisa-bin", nil }
			runLisaSubcommandFn = func(binPath string, args ...string) (string, string, error) {
				if len(args) < 2 {
					return "", "", fmt.Errorf("invalid args")
				}
				switch args[1] {
				case "monitor":
					return `{"finalState":"waiting_input","exitReason":"waiting_input"}`, "", nil
				case "diff-pack":
					return fmt.Sprintf(`{"changed":true,"tokenBudget":%d}`, tc.tokenBudget), "", nil
				case "handoff":
					return `{"deltaCount":1,"nextDeltaOffset":1}`, "", nil
				case "next":
					return `{"nextAction":"session send","recommendedCommand":"./lisa session send --json-min"}`, "", nil
				default:
					return "", "", fmt.Errorf("unexpected subcommand %q", args[1])
				}
			}

			tick := 0
			nowFn = func() time.Time {
				ts := baseNow.Add(time.Duration(tick) * time.Second)
				tick++
				return ts
			}

			stdout, stderr := captureOutput(t, func() {
				args := []string{
					"--session", session,
					"--project-root", projectRoot,
					"--steps", "3",
					tc.limitFlag, tc.limitValue,
					"--json-min",
				}
				code := cmdSessionLoop(args)
				if code == 0 {
					t.Fatalf("expected budget failure for %s cap", tc.name)
				}
			})
			if stderr != "" {
				t.Fatalf("unexpected stderr: %q", stderr)
			}

			payload := parseJSONMap(t, stdout)
			if mapStringValue(payload, "errorCode") != "budget_limit_exceeded" {
				t.Fatalf("expected budget_limit_exceeded, got %v", payload["errorCode"])
			}
			if int(payload["completedSteps"].(float64)) != 1 {
				t.Fatalf("expected completedSteps=1 for hard-stop cap, got %v", payload["completedSteps"])
			}
			violations, ok := payload["violations"].([]any)
			if !ok || len(violations) == 0 {
				t.Fatalf("expected violations payload, got %v", payload["violations"])
			}
			first := violations[0].(map[string]any)
			if mapStringValue(first, "metric") != tc.expectedRisk {
				t.Fatalf("expected first violation metric %q, got %v", tc.expectedRisk, first["metric"])
			}
		})
	}
}

func TestCmdSessionContextCacheLockModeByAction(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-cache-lock-mode"

	if err := os.WriteFile(sessionOutputFile(projectRoot, session), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := saveSessionContextCacheStore(projectRoot, sessionContextCacheStore{
		Items: map[string]sessionContextCacheRecord{
			"session:" + session: {
				Key:       "session:" + session,
				UpdatedAt: "2026-02-23T00:00:00Z",
				ExpiresAt: "2099-01-01T00:00:00Z",
				Lines:     []string{"alpha"},
			},
		},
	}); err != nil {
		t.Fatalf("seed cache store: %v", err)
	}

	origHas := tmuxHasSessionFn
	origLockFn := withSessionContextCacheLockFn
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		withSessionContextCacheLockFn = origLockFn
	})
	tmuxHasSessionFn = func(session string) bool { return false }

	lockModes := make([]bool, 0, 2)
	withSessionContextCacheLockFn = func(projectRoot string, exclusive bool, fn func() error) error {
		lockModes = append(lockModes, exclusive)
		return fn()
	}

	stdoutRefresh, stderrRefresh := captureOutput(t, func() {
		code := cmdSessionContextCache([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--refresh",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected refresh success, got %d", code)
		}
	})
	if stderrRefresh != "" {
		t.Fatalf("unexpected refresh stderr: %q", stderrRefresh)
	}
	if !strings.Contains(stdoutRefresh, `"action":"updated"`) {
		t.Fatalf("expected updated action, got %q", stdoutRefresh)
	}

	stdoutRead, stderrRead := captureOutput(t, func() {
		code := cmdSessionContextCache([]string{
			"--key", "session:" + session,
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected read success, got %d", code)
		}
	})
	if stderrRead != "" {
		t.Fatalf("unexpected read stderr: %q", stderrRead)
	}
	if !strings.Contains(stdoutRead, `"exists":true`) {
		t.Fatalf("expected cache hit payload, got %q", stdoutRead)
	}

	if len(lockModes) != 2 {
		t.Fatalf("expected 2 lock calls, got %d", len(lockModes))
	}
	if !lockModes[0] {
		t.Fatalf("expected refresh to use exclusive lock")
	}
	if lockModes[1] {
		t.Fatalf("expected read to use shared lock")
	}
}

func TestBuildRouteQueueRetainsAmbiguousSessions(t *testing.T) {
	projectRoot := t.TempDir()

	origList := tmuxListSessionsFn
	origMeta := loadSessionMetaByGlobFn
	t.Cleanup(func() {
		tmuxListSessionsFn = origList
		loadSessionMetaByGlobFn = origMeta
	})

	tmuxListSessionsFn = func(projectOnly bool, root string) ([]string, error) {
		return []string{"lisa-ambiguous-queue"}, nil
	}
	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{}, &sessionMetaAmbiguousError{Session: session}
	}

	queue, err := buildRouteQueue(projectRoot, "", 0, 0, 1)
	if err != nil {
		t.Fatalf("expected queue build success, got %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("expected ambiguous session to be retained, got queue=%v", queue)
	}
	item := queue[0]
	if mapStringValue(item, "sessionState") != "ambiguous_project_root" {
		t.Fatalf("expected ambiguous_project_root state, got %v", item["sessionState"])
	}
	if mapStringValue(item, "nextAction") != "provide_project_root" {
		t.Fatalf("expected provide_project_root next action, got %v", item["nextAction"])
	}
}

func TestCmdSessionAggregateReportsAmbiguousProjectRootWarning(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-aggregate-ambiguous"

	origMeta := loadSessionMetaByGlobFn
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMeta
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})

	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{}, &sessionMetaAmbiguousError{Session: session}
	}
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:              sessionName,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "heartbeat_fresh",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionAggregate([]string{
			"--sessions", session,
			"--project-root", projectRoot,
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected aggregate success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	payload := parseJSONMap(t, stdout)
	warnings, ok := payload["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected warnings payload, got %v", payload["warnings"])
	}
	firstWarning := warnings[0].(map[string]any)
	if mapStringValue(firstWarning, "errorCode") != "ambiguous_project_root" {
		t.Fatalf("expected ambiguous_project_root warning, got %v", firstWarning)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected aggregate items, got %v", payload["items"])
	}
	item := items[0].(map[string]any)
	if _, hasWarning := item["warning"]; !hasWarning {
		t.Fatalf("expected per-item warning, got %v", item)
	}
}

func TestCmdSessionAggregateNonJSONPrintsAmbiguousWarning(t *testing.T) {
	projectRoot := t.TempDir()
	session := "lisa-aggregate-ambiguous-text"

	origMeta := loadSessionMetaByGlobFn
	origCompute := computeSessionStatusFn
	origHas := tmuxHasSessionFn
	t.Cleanup(func() {
		loadSessionMetaByGlobFn = origMeta
		computeSessionStatusFn = origCompute
		tmuxHasSessionFn = origHas
	})

	loadSessionMetaByGlobFn = func(session string) (sessionMeta, error) {
		return sessionMeta{}, &sessionMetaAmbiguousError{Session: session}
	}
	computeSessionStatusFn = func(sessionName, root, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
		return sessionStatus{
			Session:              sessionName,
			Status:               "active",
			SessionState:         "in_progress",
			ClassificationReason: "heartbeat_fresh",
		}, nil
	}
	tmuxHasSessionFn = func(session string) bool { return false }

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionAggregate([]string{
			"--sessions", session,
			"--project-root", projectRoot,
		})
		if code != 0 {
			t.Fatalf("expected aggregate success, got %d", code)
		}
	})
	if !strings.Contains(stderr, "multiple metadata files found") || !strings.Contains(stderr, session) {
		t.Fatalf("expected ambiguous warning in stderr, got %q", stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("expected combined pack output")
	}
}
