package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	claudeOAuthTokenEnv            = "CLAUDE_CODE_OAUTH_TOKEN"
	lisaClaudeOAuthTokenRuntimeEnv = "LISA_CLAUDE_CODE_OAUTH_TOKEN"
	claudeOAuthStoreVersion        = 1
)

var oauthUserHomeDirFn = os.UserHomeDir
var oauthNowFn = time.Now
var selectClaudeOAuthTokenFn = selectClaudeOAuthToken
var peekNextClaudeOAuthTokenFn = peekNextClaudeOAuthToken
var removeClaudeOAuthTokenByIDFn = removeClaudeOAuthTokenByID

type claudeOAuthTokenRecord struct {
	ID                string `json:"id"`
	Token             string `json:"token"`
	AddedAt           string `json:"addedAt"`
	LastUsedAt        string `json:"lastUsedAt,omitempty"`
	UseCount          int    `json:"useCount,omitempty"`
	LastFailureAt     string `json:"lastFailureAt,omitempty"`
	LastFailureReason string `json:"lastFailureReason,omitempty"`
}

type claudeOAuthTokenStore struct {
	Version   int                      `json:"version"`
	NextIndex int                      `json:"nextIndex"`
	Tokens    []claudeOAuthTokenRecord `json:"tokens"`
}

type claudeOAuthTokenSelection struct {
	ID    string
	Token string
}

type claudeOAuthTokenView struct {
	ID         string `json:"id"`
	AddedAt    string `json:"addedAt"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	UseCount   int    `json:"useCount,omitempty"`
	Next       bool   `json:"next"`
}

func claudeOAuthStorePath() (string, error) {
	home, err := oauthUserHomeDirFn()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".lisa", "oauth_tokens.json"), nil
}

func claudeOAuthStoreLockTimeoutMS() int {
	timeout := getIntEnv("LISA_STATE_LOCK_TIMEOUT_MS", defaultStateLockTimeoutMS)
	if timeout <= 0 {
		timeout = defaultStateLockTimeoutMS
	}
	return timeout
}

func loadClaudeOAuthStore(path string) (claudeOAuthTokenStore, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return claudeOAuthTokenStore{
				Version: claudeOAuthStoreVersion,
				Tokens:  []claudeOAuthTokenRecord{},
			}, nil
		}
		return claudeOAuthTokenStore{}, err
	}
	var store claudeOAuthTokenStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return claudeOAuthTokenStore{}, fmt.Errorf("failed parsing oauth token store: %w", err)
	}
	normalizeClaudeOAuthStore(&store)
	return store, nil
}

func normalizeClaudeOAuthStore(store *claudeOAuthTokenStore) {
	if store.Version <= 0 {
		store.Version = claudeOAuthStoreVersion
	}
	filtered := make([]claudeOAuthTokenRecord, 0, len(store.Tokens))
	for _, tok := range store.Tokens {
		tok.Token = strings.TrimSpace(tok.Token)
		if tok.Token == "" {
			continue
		}
		if strings.TrimSpace(tok.ID) == "" {
			tok.ID = claudeOAuthTokenID(tok.Token)
		}
		filtered = append(filtered, tok)
	}
	store.Tokens = filtered
	if store.NextIndex < 0 || store.NextIndex >= len(store.Tokens) {
		store.NextIndex = 0
	}
}

func saveClaudeOAuthStore(path string, store claudeOAuthTokenStore) error {
	normalizeClaudeOAuthStore(&store)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(store)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func withClaudeOAuthStoreShared(fn func(store claudeOAuthTokenStore) error) error {
	path, err := claudeOAuthStorePath()
	if err != nil {
		return err
	}
	return withSharedFileLock(path+".lock", claudeOAuthStoreLockTimeoutMS(), func() error {
		store, loadErr := loadClaudeOAuthStore(path)
		if loadErr != nil {
			return loadErr
		}
		return fn(store)
	})
}

func withClaudeOAuthStoreExclusive(fn func(store *claudeOAuthTokenStore) error) error {
	path, err := claudeOAuthStorePath()
	if err != nil {
		return err
	}
	return withExclusiveFileLock(path+".lock", claudeOAuthStoreLockTimeoutMS(), func() error {
		store, loadErr := loadClaudeOAuthStore(path)
		if loadErr != nil {
			return loadErr
		}
		if err := fn(&store); err != nil {
			return err
		}
		return saveClaudeOAuthStore(path, store)
	})
}

func claudeOAuthTokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "oauth-" + hex.EncodeToString(sum[:])[:12]
}

func addClaudeOAuthToken(rawToken string) (claudeOAuthTokenRecord, bool, error) {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return claudeOAuthTokenRecord{}, false, fmt.Errorf("oauth token cannot be empty")
	}

	now := oauthNowFn().UTC().Format(time.RFC3339)
	id := claudeOAuthTokenID(token)
	added := false
	record := claudeOAuthTokenRecord{}
	err := withClaudeOAuthStoreExclusive(func(store *claudeOAuthTokenStore) error {
		for _, existing := range store.Tokens {
			if strings.TrimSpace(existing.Token) == token {
				record = existing
				added = false
				return nil
			}
		}
		record = claudeOAuthTokenRecord{
			ID:      id,
			Token:   token,
			AddedAt: now,
		}
		store.Tokens = append(store.Tokens, record)
		if len(store.Tokens) == 1 {
			store.NextIndex = 0
		}
		added = true
		return nil
	})
	return record, added, err
}

func listClaudeOAuthTokens() ([]claudeOAuthTokenView, error) {
	out := []claudeOAuthTokenView{}
	err := withClaudeOAuthStoreShared(func(store claudeOAuthTokenStore) error {
		nextIdx := 0
		if len(store.Tokens) > 0 && store.NextIndex >= 0 && store.NextIndex < len(store.Tokens) {
			nextIdx = store.NextIndex
		}
		out = make([]claudeOAuthTokenView, 0, len(store.Tokens))
		for idx, tok := range store.Tokens {
			out = append(out, claudeOAuthTokenView{
				ID:         tok.ID,
				AddedAt:    tok.AddedAt,
				LastUsedAt: tok.LastUsedAt,
				UseCount:   tok.UseCount,
				Next:       idx == nextIdx,
			})
		}
		return nil
	})
	return out, err
}

func claudeOAuthTokenCount() (int, error) {
	count := 0
	err := withClaudeOAuthStoreShared(func(store claudeOAuthTokenStore) error {
		count = len(store.Tokens)
		return nil
	})
	return count, err
}

func peekNextClaudeOAuthToken() (string, bool, error) {
	nextID := ""
	found := false
	err := withClaudeOAuthStoreShared(func(store claudeOAuthTokenStore) error {
		if len(store.Tokens) == 0 {
			return nil
		}
		idx := store.NextIndex
		if idx < 0 || idx >= len(store.Tokens) {
			idx = 0
		}
		nextID = strings.TrimSpace(store.Tokens[idx].ID)
		found = nextID != ""
		return nil
	})
	return nextID, found, err
}

func selectClaudeOAuthToken() (claudeOAuthTokenSelection, bool, error) {
	selection := claudeOAuthTokenSelection{}
	found := false
	now := oauthNowFn().UTC().Format(time.RFC3339)
	err := withClaudeOAuthStoreExclusive(func(store *claudeOAuthTokenStore) error {
		if len(store.Tokens) == 0 {
			return nil
		}
		idx := store.NextIndex
		if idx < 0 || idx >= len(store.Tokens) {
			idx = 0
		}
		token := store.Tokens[idx]
		token.LastUsedAt = now
		token.UseCount++
		store.Tokens[idx] = token
		store.NextIndex = (idx + 1) % len(store.Tokens)
		selection = claudeOAuthTokenSelection{
			ID:    token.ID,
			Token: token.Token,
		}
		found = strings.TrimSpace(token.Token) != ""
		return nil
	})
	return selection, found, err
}

func removeClaudeOAuthTokenByID(id, reason string) (bool, error) {
	targetID := strings.TrimSpace(id)
	if targetID == "" {
		return false, fmt.Errorf("oauth token id cannot be empty")
	}
	removed := false
	now := oauthNowFn().UTC().Format(time.RFC3339)
	err := withClaudeOAuthStoreExclusive(func(store *claudeOAuthTokenStore) error {
		for idx, tok := range store.Tokens {
			if strings.TrimSpace(tok.ID) != targetID {
				continue
			}
			if reason != "" {
				tok.LastFailureAt = now
				tok.LastFailureReason = reason
			}
			store.Tokens = append(store.Tokens[:idx], store.Tokens[idx+1:]...)
			if len(store.Tokens) == 0 {
				store.NextIndex = 0
			} else {
				if idx < store.NextIndex {
					store.NextIndex--
				}
				if store.NextIndex < 0 || store.NextIndex >= len(store.Tokens) {
					store.NextIndex = 0
				}
			}
			removed = true
			return nil
		}
		return nil
	})
	return removed, err
}

func detectClaudeOAuthTokenFailure(capture string) (string, bool) {
	lower := strings.ToLower(capture)
	switch {
	case strings.Contains(lower, "invalid_grant: invalid refresh token"):
		return "invalid_refresh_token", true
	case strings.Contains(lower, "oauth token refresh failed") && strings.Contains(lower, "expired"):
		return "expired_token", true
	case strings.Contains(lower, "oauth token refresh failed"):
		return "refresh_failed", true
	case strings.Contains(lower, "auth(tokenrefreshfailed(") && strings.Contains(lower, "expired"):
		return "expired_token", true
	case strings.Contains(lower, "auth(tokenrefreshfailed("):
		return "refresh_failed", true
	case strings.Contains(lower, "oauth") && strings.Contains(lower, "expired token"):
		return "expired_token", true
	case strings.Contains(lower, "oauth") && strings.Contains(lower, "invalid_token"):
		return "invalid_token", true
	default:
		return "", false
	}
}

func markSessionOAuthTokenPruned(statePath, reason string) error {
	_, err := withStateFileLockFn(statePath, func() error {
		state, loadErr := loadSessionStateWithError(statePath)
		if loadErr != nil {
			return loadErr
		}
		state.OAuthTokenPruned = true
		state.OAuthTokenPruneReason = reason
		return saveSessionState(statePath, state)
	})
	return err
}

func maybePruneInvalidClaudeOAuthToken(projectRoot, session string, meta sessionMeta, status sessionStatus, statePath string, stateHint sessionState) sessionStatus {
	if strings.ToLower(strings.TrimSpace(status.Agent)) != "claude" {
		return status
	}
	tokenID := strings.TrimSpace(meta.OAuthTokenID)
	if tokenID == "" || stateHint.OAuthTokenPruned {
		return status
	}
	switch status.SessionState {
	case "crashed", "stuck", "degraded":
		// continue
	default:
		return status
	}

	capture, err := tmuxCapturePaneFn(session, 320)
	if err != nil {
		return status
	}
	reason, matched := detectClaudeOAuthTokenFailure(capture)
	if !matched {
		return status
	}

	if _, removeErr := removeClaudeOAuthTokenByIDFn(tokenID, reason); removeErr != nil {
		fmt.Fprintf(os.Stderr, "oauth token prune warning: %v\n", removeErr)
		return status
	}
	if markErr := markSessionOAuthTokenPruned(statePath, reason); markErr != nil {
		fmt.Fprintf(os.Stderr, "oauth token prune warning: failed to persist state marker: %v\n", markErr)
	}

	status.Status = "idle"
	status.SessionState = "crashed"
	status.WaitEstimate = 0
	status.ClassificationReason = "oauth_" + reason
	if err := appendLifecycleEvent(projectRoot, session, "lifecycle", status.SessionState, status.Status, "oauth_token_pruned_"+reason); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}
	return status
}
