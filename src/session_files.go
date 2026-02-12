package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var saveSessionMetaFn = saveSessionMeta
var loadSessionMetaByGlobFn = loadSessionMetaByGlob

type cleanupOptions struct {
	AllHashes  bool
	KeepEvents bool
}

func generateSessionName(projectRoot, agent, mode, tag string) string {
	now := time.Now()
	stamp := fmt.Sprintf("%s-%09d", now.Format("060102-150405"), now.Nanosecond())
	slug := projectSlug(projectRoot)
	parsedAgent, err := parseAgent(agent)
	if err != nil {
		parsedAgent = "claude"
	}
	parsedMode, err := parseMode(mode)
	if err != nil {
		parsedMode = "interactive"
	}
	name := fmt.Sprintf("lisa-%s-%s-%s-%s", slug, stamp, parsedAgent, parsedMode)
	if tag != "" {
		name += "-" + sanitizeID(tag, 16)
	}
	return name
}

func projectSlug(projectRoot string) string {
	projectRoot = canonicalProjectRoot(projectRoot)
	base := filepath.Base(projectRoot)
	return sanitizeID(base, 10)
}

func sanitizeID(s string, max int) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		out = "project"
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func sessionStateFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-state.json", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionMetaFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-meta.json", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionOutputFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/lisa-%s-output-%s.txt", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionHeartbeatFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-heartbeat.txt", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionDoneFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-done.txt", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionEventsFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-events.jsonl", projectHash(projectRoot), sessionArtifactID(session))
}

func sessionStateLockFile(projectRoot, session string) string {
	return fmt.Sprintf("%s.lock", sessionStateFile(projectRoot, session))
}

func projectHash(projectRoot string) string {
	return md5Hex8(canonicalProjectRoot(projectRoot))
}

func canonicalProjectRoot(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = getPWD()
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	eval, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = eval
	}
	return filepath.Clean(root)
}

func sessionArtifactID(session string) string {
	session = strings.TrimSpace(session)
	if session == "" {
		return "session-" + md5Hex8(session)
	}
	safe := sanitizeSessionToken(session)
	if safe == session {
		return safe
	}
	return fmt.Sprintf("%s-%s", safe, md5Hex8(session))
}

func sanitizeSessionToken(session string) string {
	var b strings.Builder
	for _, r := range session {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "session"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func sessionCommandScriptPath(projectRoot, session string, createdAtNanos int64) string {
	return fmt.Sprintf("/tmp/lisa-cmd-%s-%s-%d.sh", projectHash(projectRoot), sessionArtifactID(session), createdAtNanos)
}

func sessionCommandScriptPattern(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/lisa-cmd-%s-%s-*.sh", projectHash(projectRoot), sessionArtifactID(session))
}

func loadSessionState(path string) sessionState {
	state, err := loadSessionStateWithError(path)
	if err != nil {
		return sessionState{}
	}
	return state
}

func loadSessionStateWithError(path string) (sessionState, error) {
	if !fileExists(path) {
		return sessionState{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionState{}, err
	}
	var state sessionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return sessionState{}, err
	}
	return state, nil
}

func saveSessionState(path string, state sessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func saveSessionMeta(projectRoot, session string, meta sessionMeta) error {
	path := sessionMetaFile(projectRoot, session)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func loadSessionMeta(projectRoot, session string) (sessionMeta, error) {
	path := sessionMetaFile(projectRoot, session)
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionMeta{}, err
	}
	var meta sessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return sessionMeta{}, err
	}
	return meta, nil
}

func loadSessionMetaByGlob(session string) (sessionMeta, error) {
	aid := sessionArtifactID(session)
	pattern := fmt.Sprintf("/tmp/.lisa-*-session-%s-meta.json", aid)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return sessionMeta{}, fmt.Errorf("glob failed: %w", err)
	}
	if len(matches) == 0 {
		return sessionMeta{}, fmt.Errorf("no metadata file found for session %q", session)
	}
	// Use the first match (most common case: single project)
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return sessionMeta{}, err
	}
	var meta sessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return sessionMeta{}, err
	}
	return meta, nil
}

func cleanupSessionArtifacts(projectRoot, session string) error {
	return cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOptions{
		AllHashes: cleanupAllHashesEnabled(),
	})
}

func cleanupSessionArtifactsWithOptions(projectRoot, session string, opts cleanupOptions) error {
	var errs []string
	files := make(map[string]struct{}, 8)
	for _, path := range []string{
		sessionStateFile(projectRoot, session),
		sessionMetaFile(projectRoot, session),
		sessionOutputFile(projectRoot, session),
		sessionHeartbeatFile(projectRoot, session),
		sessionDoneFile(projectRoot, session),
		sessionStateLockFile(projectRoot, session),
	} {
		files[path] = struct{}{}
	}
	if !opts.KeepEvents {
		eventsPath := sessionEventsFile(projectRoot, session)
		files[eventsPath] = struct{}{}
		files[sessionEventCountFile(eventsPath)] = struct{}{}
	}

	sid := sessionArtifactID(session)
	globPatterns := []string{
		sessionCommandScriptPattern(projectRoot, session),
		fmt.Sprintf("/tmp/lisa-cmd-%s-*.sh", sid), // legacy pattern
	}
	if opts.AllHashes {
		globPatterns = append(globPatterns,
			fmt.Sprintf("/tmp/.lisa-*-session-%s-state.json", sid),
			fmt.Sprintf("/tmp/.lisa-*-session-%s-meta.json", sid),
			fmt.Sprintf("/tmp/lisa-*-output-%s.txt", sid),
			fmt.Sprintf("/tmp/.lisa-*-session-%s-heartbeat.txt", sid),
			fmt.Sprintf("/tmp/.lisa-*-session-%s-done.txt", sid),
			fmt.Sprintf("/tmp/.lisa-*-session-%s-state.json.lock", sid),
			fmt.Sprintf("/tmp/lisa-cmd-*-%s-*.sh", sid),
		)
		if !opts.KeepEvents {
			globPatterns = append(globPatterns,
				fmt.Sprintf("/tmp/.lisa-*-session-%s-events.jsonl", sid),
				fmt.Sprintf("/tmp/.lisa-*-session-%s-events.jsonl.lines", sid),
			)
		}
	}
	for _, pattern := range globPatterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, m := range matches {
			files[m] = struct{}{}
		}
	}

	for path := range files {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func cleanupAllHashesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LISA_CLEANUP_ALL_HASHES"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeSessionOutputFile(projectRoot, session string) (string, error) {
	capture, err := tmuxCapturePaneFn(session, 320)
	if err != nil {
		return "", err
	}
	return writeSessionOutputFileFromCapture(projectRoot, session, capture)
}

func writeSessionOutputFileFromCapture(projectRoot, session, capture string) (string, error) {
	lines := tailLines(trimLines(capture), 260)
	path := sessionOutputFile(projectRoot, session)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func tailLines(lines []string, max int) []string {
	if max <= 0 || len(lines) == 0 {
		return []string{}
	}
	if len(lines) <= max {
		return lines
	}
	return lines[len(lines)-max:]
}
