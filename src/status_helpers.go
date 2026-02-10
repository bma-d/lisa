package app

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

func resolveAgent(agentHint string, meta sessionMeta, session string) string {
	agentHint = strings.ToLower(strings.TrimSpace(agentHint))
	if agentHint == "claude" || agentHint == "codex" {
		return agentHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Agent)); v == "claude" || v == "codex" {
		return v
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

func resolveMode(modeHint string, meta sessionMeta, session string) string {
	modeHint = strings.ToLower(strings.TrimSpace(modeHint))
	if modeHint == "interactive" || modeHint == "exec" {
		return modeHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Mode)); v == "interactive" || v == "exec" {
		return v
	}
	if v, err := tmuxShowEnvironmentFn(session, "LISA_MODE"); err == nil {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "interactive" || v == "exec" {
			return v
		}
	}
	return "interactive"
}

func parseTodos(capture string) (int, int) {
	lines := trimLines(capture)
	done := 0
	total := 0
	for _, line := range lines {
		matches := todoCheckboxRe.FindAllString(line, -1)
		for _, m := range matches {
			total++
			if strings.EqualFold(m, "[x]") {
				done++
			}
		}
	}
	return done, total
}

func extractActiveTask(capture string) string {
	lines := trimLines(capture)
	if len(lines) == 0 {
		return ""
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if containsAnyPrefix(line, activeTaskIgnorePrefixes) {
			continue
		}
		for _, p := range activeTaskPatterns {
			if p.MatchString(line) {
				if len(line) > 140 {
					return strings.TrimSpace(line[:140])
				}
				return line
			}
		}
	}
	return ""
}

func looksLikePromptWaiting(agent, capture string) bool {
	lines := trimLines(capture)
	if len(lines) == 0 {
		return false
	}

	tail := make([]string, 0, 8)
	for i := len(lines) - 1; i >= 0 && len(tail) < 8; i-- {
		line := strings.TrimSpace(stripANSIEscape(lines[i]))
		if line == "" {
			continue
		}
		tail = append(tail, line)
	}
	if len(tail) == 0 {
		return false
	}
	last := tail[0]
	lowerTail := strings.ToLower(strings.Join(tail, "\n"))

	if promptBusyKeywordRe.MatchString(last) {
		return false
	}

	if agent == "codex" {
		if codexPromptRe.MatchString(last) {
			return true
		}
		if strings.Contains(lowerTail, "tokens used") && strings.Contains(last, "❯") {
			return true
		}
	}

	if agent == "claude" {
		if strings.HasSuffix(last, "›") {
			return true
		}
		if strings.TrimSpace(last) == ">" {
			return true
		}
		if strings.Contains(lowerTail, "press enter to send") && (strings.TrimSpace(last) == ">" || strings.HasSuffix(last, "›")) {
			return true
		}
	}
	return false
}

func estimateWait(task string, done, total int) int {
	lower := strings.ToLower(task)
	switch {
	case waitReadLikeRe.MatchString(lower):
		return 30
	case waitBuildLikeRe.MatchString(lower):
		return 120
	case waitWriteLikeRe.MatchString(lower):
		return 60
	}
	if total > 0 {
		progress := 100 * done / total
		switch {
		case progress < 25:
			return 90
		case progress < 50:
			return 75
		case progress < 75:
			return 60
		default:
			return 30
		}
	}
	return 60
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
