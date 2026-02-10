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

func sessionCommandScriptPath(session string, createdAtNanos int64) string {
	return fmt.Sprintf("/tmp/lisa-cmd-%s-%d.sh", sessionArtifactID(session), createdAtNanos)
}

func sessionCommandScriptPattern(session string) string {
	return fmt.Sprintf("/tmp/lisa-cmd-%s-*.sh", sessionArtifactID(session))
}

func loadSessionState(path string) sessionState {
	if !fileExists(path) {
		return sessionState{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionState{}
	}
	var state sessionState
	_ = json.Unmarshal(raw, &state)
	return state
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

func cleanupSessionArtifacts(projectRoot, session string) error {
	var errs []string
	files := make(map[string]struct{}, 8)
	for _, path := range []string{
		sessionStateFile(projectRoot, session),
		sessionMetaFile(projectRoot, session),
		sessionOutputFile(projectRoot, session),
	} {
		files[path] = struct{}{}
	}

	sid := sessionArtifactID(session)
	globPatterns := []string{
		fmt.Sprintf("/tmp/.lisa-*-session-%s-state.json", sid),
		fmt.Sprintf("/tmp/.lisa-*-session-%s-meta.json", sid),
		fmt.Sprintf("/tmp/lisa-*-output-%s.txt", sid),
		sessionCommandScriptPattern(session),
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

func writeSessionOutputFile(projectRoot, session string) (string, error) {
	capture, err := tmuxCapturePane(session, 320)
	if err != nil {
		return "", err
	}
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
