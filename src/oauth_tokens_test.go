package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaudeOAuthTokenRoundRobin(t *testing.T) {
	origHome := oauthUserHomeDirFn
	origNow := oauthNowFn
	t.Cleanup(func() {
		oauthUserHomeDirFn = origHome
		oauthNowFn = origNow
	})

	home := t.TempDir()
	oauthUserHomeDirFn = func() (string, error) { return home, nil }
	ts := time.Date(2026, 2, 22, 1, 0, 0, 0, time.UTC)
	oauthNowFn = func() time.Time {
		ts = ts.Add(time.Second)
		return ts
	}

	first, firstAdded, err := addClaudeOAuthToken("token-one")
	if err != nil {
		t.Fatalf("add token one failed: %v", err)
	}
	if !firstAdded {
		t.Fatalf("expected first token add to be true")
	}
	second, secondAdded, err := addClaudeOAuthToken("token-two")
	if err != nil {
		t.Fatalf("add token two failed: %v", err)
	}
	if !secondAdded {
		t.Fatalf("expected second token add to be true")
	}

	s1, ok, err := selectClaudeOAuthToken()
	if err != nil {
		t.Fatalf("select #1 failed: %v", err)
	}
	if !ok || s1.ID != first.ID {
		t.Fatalf("unexpected select #1: %#v ok=%v", s1, ok)
	}

	s2, ok, err := selectClaudeOAuthToken()
	if err != nil {
		t.Fatalf("select #2 failed: %v", err)
	}
	if !ok || s2.ID != second.ID {
		t.Fatalf("unexpected select #2: %#v ok=%v", s2, ok)
	}

	s3, ok, err := selectClaudeOAuthToken()
	if err != nil {
		t.Fatalf("select #3 failed: %v", err)
	}
	if !ok || s3.ID != first.ID {
		t.Fatalf("unexpected select #3: %#v ok=%v", s3, ok)
	}

	rows, err := listClaudeOAuthTokens()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	path, err := claudeOAuthStorePath()
	if err != nil {
		t.Fatalf("store path failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat store failed: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 store permissions, got %o", info.Mode().Perm())
	}
}

func TestCmdOAuthLifecycleJSON(t *testing.T) {
	origHome := oauthUserHomeDirFn
	t.Cleanup(func() { oauthUserHomeDirFn = origHome })
	home := t.TempDir()
	oauthUserHomeDirFn = func() (string, error) { return home, nil }

	stdout, stderr := captureOutput(t, func() {
		code := cmdOAuth([]string{"add", "--token", "abc123", "--json"})
		if code != 0 {
			t.Fatalf("expected add success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var addPayload struct {
		ID    string `json:"id"`
		Added bool   `json:"added"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &addPayload); err != nil {
		t.Fatalf("failed parsing add payload: %v (%q)", err, stdout)
	}
	if addPayload.ID == "" || !addPayload.Added || addPayload.Count != 1 {
		t.Fatalf("unexpected add payload: %q", stdout)
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdOAuth([]string{"list", "--json"})
		if code != 0 {
			t.Fatalf("expected list success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	var listPayload struct {
		Count  int                    `json:"count"`
		Tokens []claudeOAuthTokenView `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(stdout), &listPayload); err != nil {
		t.Fatalf("failed parsing list payload: %v (%q)", err, stdout)
	}
	if listPayload.Count != 1 || len(listPayload.Tokens) != 1 {
		t.Fatalf("unexpected list payload: %q", stdout)
	}
	if listPayload.Tokens[0].ID != addPayload.ID {
		t.Fatalf("expected listed token id %q, got %q", addPayload.ID, listPayload.Tokens[0].ID)
	}

	stdout, stderr = captureOutput(t, func() {
		code := cmdOAuth([]string{"remove", "--id", addPayload.ID, "--json"})
		if code != 0 {
			t.Fatalf("expected remove success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"removed":true`) {
		t.Fatalf("expected remove payload to confirm removal, got %q", stdout)
	}
}

func TestSessionSpawnUsesManagedOAuthToken(t *testing.T) {
	origHas := tmuxHasSessionFn
	origNew := tmuxNewSessionWithStartupFn
	origSaveMeta := saveSessionMetaFn
	origSelect := selectClaudeOAuthTokenFn
	origPeek := peekNextClaudeOAuthTokenFn
	origEnv := os.Getenv(lisaClaudeOAuthTokenRuntimeEnv)
	t.Cleanup(func() {
		tmuxHasSessionFn = origHas
		tmuxNewSessionWithStartupFn = origNew
		saveSessionMetaFn = origSaveMeta
		selectClaudeOAuthTokenFn = origSelect
		peekNextClaudeOAuthTokenFn = origPeek
		if origEnv == "" {
			_ = os.Unsetenv(lisaClaudeOAuthTokenRuntimeEnv)
		} else {
			_ = os.Setenv(lisaClaudeOAuthTokenRuntimeEnv, origEnv)
		}
	})

	projectRoot := t.TempDir()
	session := "lisa-oauth-managed-spawn"
	tmuxHasSessionFn = func(string) bool { return false }
	selectClaudeOAuthTokenFn = func() (claudeOAuthTokenSelection, bool, error) {
		return claudeOAuthTokenSelection{ID: "oauth-test-id", Token: "oauth-test-token"}, true, nil
	}
	peekNextClaudeOAuthTokenFn = func() (string, bool, error) {
		return "oauth-test-id", true, nil
	}

	seenManagedEnv := ""
	tmuxNewSessionWithStartupFn = func(session, projectRoot, agent, mode string, width, height int, startupCommand string) error {
		seenManagedEnv = os.Getenv(lisaClaudeOAuthTokenRuntimeEnv)
		return nil
	}

	var capturedMeta sessionMeta
	saveSessionMetaFn = func(projectRoot, session string, meta sessionMeta) error {
		capturedMeta = meta
		return nil
	}

	stdout, stderr := captureOutput(t, func() {
		code := cmdSessionSpawn([]string{
			"--session", session,
			"--project-root", projectRoot,
			"--agent", "claude",
			"--mode", "interactive",
			"--command", "echo ready",
			"--json",
		})
		if code != 0 {
			t.Fatalf("expected spawn success, got %d", code)
		}
	})
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	if !strings.Contains(stdout, `"oauthTokenId":"oauth-test-id"`) {
		t.Fatalf("expected spawn JSON to include oauth token id, got %q", stdout)
	}
	if seenManagedEnv != "oauth-test-token" {
		t.Fatalf("expected managed env during spawn, got %q", seenManagedEnv)
	}
	if capturedMeta.OAuthTokenID != "oauth-test-id" {
		t.Fatalf("expected meta oauth token id, got %q", capturedMeta.OAuthTokenID)
	}
	if got := os.Getenv(lisaClaudeOAuthTokenRuntimeEnv); got != origEnv {
		t.Fatalf("expected managed env restored after spawn, got %q", got)
	}
}

func TestMaybePruneInvalidClaudeOAuthToken(t *testing.T) {
	origCapture := tmuxCapturePaneFn
	origRemove := removeClaudeOAuthTokenByIDFn
	t.Cleanup(func() {
		tmuxCapturePaneFn = origCapture
		removeClaudeOAuthTokenByIDFn = origRemove
	})

	projectRoot := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	meta := sessionMeta{Agent: "claude", OAuthTokenID: "oauth-dead"}
	status := sessionStatus{
		Session:      "lisa-oauth-prune",
		Agent:        "claude",
		Status:       "idle",
		SessionState: "crashed",
	}

	tmuxCapturePaneFn = func(session string, lines int) (string, error) {
		return "Auth error: OAuth token refresh failed: invalid_grant: Invalid refresh token", nil
	}
	calledID := ""
	calledReason := ""
	removeClaudeOAuthTokenByIDFn = func(id, reason string) (bool, error) {
		calledID = id
		calledReason = reason
		return true, nil
	}

	updated := maybePruneInvalidClaudeOAuthToken(projectRoot, "lisa-oauth-prune", meta, status, statePath, sessionState{})
	if updated.ClassificationReason != "oauth_invalid_refresh_token" {
		t.Fatalf("unexpected classification reason: %q", updated.ClassificationReason)
	}
	if calledID != "oauth-dead" || calledReason != "invalid_refresh_token" {
		t.Fatalf("unexpected prune call: id=%q reason=%q", calledID, calledReason)
	}

	state, err := loadSessionStateWithError(statePath)
	if err != nil {
		t.Fatalf("failed loading state after prune: %v", err)
	}
	if !state.OAuthTokenPruned || state.OAuthTokenPruneReason != "invalid_refresh_token" {
		t.Fatalf("expected prune state marker, got %#v", state)
	}
}
