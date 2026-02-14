package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var doneFileCompletionLineRe = regexp.MustCompile(`^([A-Za-z0-9._-]+):(-?\d+)\s*$`)

func resolveAgent(agentHint string, meta sessionMeta, session string, cached string) string {
	agentHint = strings.ToLower(strings.TrimSpace(agentHint))
	if agentHint == "claude" || agentHint == "codex" {
		return agentHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Agent)); v == "claude" || v == "codex" {
		return v
	}
	if cached == "claude" || cached == "codex" {
		return cached
	}
	name := strings.ToLower(strings.TrimSpace(session))
	if strings.Contains(name, "-codex-") || strings.HasSuffix(name, "-codex") {
		return "codex"
	}
	if strings.Contains(name, "-claude-") || strings.HasSuffix(name, "-claude") {
		return "claude"
	}
	if envAgent, err := tmuxShowEnvironmentFn(session, "LISA_AGENT"); err == nil {
		envAgent = strings.ToLower(strings.TrimSpace(envAgent))
		if envAgent == "claude" || envAgent == "codex" {
			return envAgent
		}
	}
	if envAgent, err := tmuxShowEnvironmentFn(session, "AI_AGENT"); err == nil {
		envAgent = strings.ToLower(strings.TrimSpace(envAgent))
		if envAgent == "claude" || envAgent == "codex" {
			return envAgent
		}
	}
	return "claude"
}

func resolveMode(modeHint string, meta sessionMeta, session string, cached string) string {
	modeHint = strings.ToLower(strings.TrimSpace(modeHint))
	if modeHint == "interactive" || modeHint == "exec" {
		return modeHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Mode)); v == "interactive" || v == "exec" {
		return v
	}
	if cached == "interactive" || cached == "exec" {
		return cached
	}
	name := strings.ToLower(strings.TrimSpace(session))
	if strings.HasSuffix(name, "-exec") || strings.Contains(name, "-exec-") {
		return "exec"
	}
	if strings.HasSuffix(name, "-interactive") || strings.Contains(name, "-interactive-") {
		return "interactive"
	}
	if v, err := tmuxShowEnvironmentFn(session, "LISA_MODE"); err == nil {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "interactive" || v == "exec" {
			return v
		}
	}
	return "interactive"
}

func parseExecCompletion(capture string) (bool, int) {
	return parseCompletionMarker(capture, execCompletionLineRe)
}

func parseSessionCompletion(capture string) (bool, int) {
	done, code, _, _ := parseSessionCompletionForRun(capture, "")
	return done, code
}

func parseSessionCompletionForRun(capture, runID string) (bool, int, string, bool) {
	marker := parseSessionCompletionMarker(capture)
	if !marker.Seen {
		return false, 0, "", false
	}
	runMismatch := runID != "" && marker.RunID != "" && marker.RunID != runID
	if runMismatch {
		return false, marker.ExitCode, marker.RunID, true
	}
	return true, marker.ExitCode, marker.RunID, false
}

func readSessionDoneFile(projectRoot, session, runID string) (bool, int, string, bool, error) {
	raw, err := os.ReadFile(sessionDoneFile(projectRoot, session))
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, "", false, nil
		}
		return false, 0, "", false, err
	}
	line := strings.TrimSpace(string(raw))
	if line == "" {
		return false, 0, "", false, nil
	}
	match := doneFileCompletionLineRe.FindStringSubmatch(line)
	if len(match) != 3 {
		return false, 0, "", false, fmt.Errorf("invalid done file marker in %s", filepath.Base(sessionDoneFile(projectRoot, session)))
	}
	fileRunID := strings.TrimSpace(match[1])
	code, err := strconv.Atoi(match[2])
	if err != nil {
		code = 1
	}
	runMismatch := runID != "" && fileRunID != "" && fileRunID != runID
	if runMismatch {
		return false, code, fileRunID, true, nil
	}
	return true, code, fileRunID, false, nil
}

func parseSessionCompletionMarker(capture string) sessionCompletionMarker {
	tail := nonEmptyTailLines(capture, 24)
	if len(tail) == 0 {
		return sessionCompletionMarker{}
	}

	markerIdx := -1
	markerRunID := ""
	markerCode := ""
	for i := len(tail) - 1; i >= 0; i-- {
		match := sessionCompletionLineRe.FindStringSubmatch(tail[i])
		if len(match) == 3 {
			markerIdx = i
			markerRunID = strings.TrimSpace(match[1])
			markerCode = match[2]
			break
		}
	}
	if markerIdx < 0 {
		return sessionCompletionMarker{}
	}
	for i := markerIdx + 1; i < len(tail); i++ {
		if !isLikelyShellPromptLine(tail[i]) {
			return sessionCompletionMarker{}
		}
	}
	code, err := strconv.Atoi(markerCode)
	if err != nil {
		code = 1
	}
	return sessionCompletionMarker{Seen: true, RunID: markerRunID, ExitCode: code}
}

func parseCompletionMarker(capture string, markerRe *regexp.Regexp) (bool, int) {
	tail := nonEmptyTailLines(capture, 24)
	if len(tail) == 0 {
		return false, 0
	}

	markerIdx := -1
	markerCode := ""
	for i := len(tail) - 1; i >= 0; i-- {
		match := markerRe.FindStringSubmatch(tail[i])
		if len(match) == 2 {
			markerIdx = i
			markerCode = match[1]
			break
		}
	}
	if markerIdx < 0 {
		return false, 0
	}
	for i := markerIdx + 1; i < len(tail); i++ {
		if !isLikelyShellPromptLine(tail[i]) {
			return false, 0
		}
	}

	code, err := strconv.Atoi(markerCode)
	if err != nil {
		return true, 1
	}
	return true, code
}

func nonEmptyTailLines(capture string, max int) []string {
	lines := trimLines(capture)
	tail := make([]string, 0, max)
	for i := len(lines) - 1; i >= 0 && len(tail) < max; i-- {
		line := strings.TrimSpace(stripANSIEscape(lines[i]))
		if line == "" {
			continue
		}
		tail = append([]string{line}, tail...)
	}
	return tail
}

func stripANSIEscape(line string) string {
	return ansiEscapeRe.ReplaceAllString(line, "")
}

func sessionHeartbeatAge(projectRoot, session string, now int64) (int, bool) {
	info, err := os.Stat(sessionHeartbeatFile(projectRoot, session))
	if err != nil {
		return 0, false
	}
	age := int(now - info.ModTime().Unix())
	if age < 0 {
		age = 0
	}
	return age, true
}

func isAgeFresh(age, staleAfter int) bool {
	if age < 0 {
		return false
	}
	return age <= staleAfter
}

func isLikelyShellPromptLine(line string) bool {
	line = strings.TrimSpace(stripANSIEscape(line))
	if line == "" {
		return true
	}
	// Starship and similar prompts often include duration/time segments after the prompt token.
	if shellPromptTrailerRe.MatchString(line) {
		return true
	}
	if strings.HasSuffix(line, "$") ||
		strings.HasSuffix(line, "#") ||
		strings.HasSuffix(line, "%") ||
		strings.HasSuffix(line, "❯") {
		return true
	}

	if strings.HasSuffix(line, ">") {
		if strings.Contains(line, "<") {
			return false
		}
		return strings.Contains(line, "/") ||
			strings.Contains(line, "~") ||
			strings.Contains(line, `\\`)
	}
	return false
}
