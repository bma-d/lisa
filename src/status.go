package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func computeSessionStatus(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
	status := sessionStatus{
		Session:            session,
		Status:             "error",
		SessionState:       "error",
		WaitEstimate:       30,
		OutputFreshSeconds: defaultOutputStaleSeconds,
	}

	if session == "" {
		status.ActiveTask = "no_session"
		return status, nil
	}
	if !tmuxHasSessionFn(session) {
		status.Status = "not_found"
		status.SessionState = "not_found"
		status.WaitEstimate = 0
		return status, nil
	}

	meta, _ := loadSessionMeta(projectRoot, session)
	agent := resolveAgent(agentHint, meta, session)
	mode := resolveMode(modeHint, meta, session)
	status.Agent = agent
	status.Mode = mode

	capture, err := tmuxCapturePaneFn(session, 220)
	if err != nil {
		return status, fmt.Errorf("failed to capture tmux pane: %w", err)
	}
	capture = filterInputBox(capture)
	lines := trimLines(capture)
	capture = strings.Join(lines, "\n")

	statePath := sessionStateFile(projectRoot, session)
	state := loadSessionState(statePath)
	now := time.Now().Unix()
	hash := md5Hex8(capture)
	if hash != "" && hash != state.LastOutputHash {
		state.LastOutputHash = hash
		state.LastOutputAt = now
	}
	if state.LastOutputAt == 0 {
		state.LastOutputAt = now
	}
	outputAge := int(now - state.LastOutputAt)
	staleAfter := getIntEnv("LISA_OUTPUT_STALE_SECONDS", defaultOutputStaleSeconds)
	outputFresh := outputAge <= staleAfter
	status.OutputAgeSeconds = outputAge
	status.OutputFreshSeconds = staleAfter

	paneStatus, err := tmuxPaneStatusFn(session)
	if err != nil {
		return status, fmt.Errorf("failed to read tmux pane status: %w", err)
	}
	paneCommand, err := tmuxDisplayFn(session, "#{pane_current_command}")
	if err != nil {
		return status, fmt.Errorf("failed to read tmux pane command: %w", err)
	}
	status.PaneStatus = paneStatus
	status.PaneCommand = paneCommand

	todoDone, todoTotal := parseTodos(capture)
	status.TodosDone = todoDone
	status.TodosTotal = todoTotal
	status.ActiveTask = extractActiveTask(capture)
	status.WaitEstimate = estimateWait(status.ActiveTask, todoDone, todoTotal)
	execDone, execExitCode := parseExecCompletion(capture)

	panePIDRaw, err := tmuxDisplayFn(session, "#{pane_pid}")
	if err != nil {
		return status, fmt.Errorf("failed to read tmux pane pid: %w", err)
	}
	panePIDText := strings.TrimSpace(panePIDRaw)
	panePID := 0
	if panePIDText != "" {
		panePID, err = strconv.Atoi(panePIDText)
		if err != nil {
			return status, fmt.Errorf("failed to parse tmux pane pid %q: %w", panePIDText, err)
		}
	}
	agentPID, agentCPU := detectAgentProcessFn(panePID, agent)
	status.AgentPID = agentPID
	status.AgentCPU = agentCPU

	switch {
	case strings.HasPrefix(paneStatus, "crashed:"):
		status.Status = "idle"
		status.SessionState = "crashed"
		status.WaitEstimate = 0
	case strings.HasPrefix(paneStatus, "exited:"):
		exitCode := strings.TrimPrefix(paneStatus, "exited:")
		if exitCode == "0" {
			status.Status = "idle"
			status.SessionState = "completed"
		} else {
			status.Status = "idle"
			status.SessionState = "crashed"
		}
		status.WaitEstimate = 0
	default:
		status.Status = "active"
		status.SessionState = "in_progress"

		interactiveWaiting := mode == "interactive" && agentPID > 0 && agentCPU < 0.2 && !outputFresh
		if mode == "exec" && execDone {
			status.Status = "idle"
			if execExitCode == 0 {
				status.SessionState = "completed"
			} else {
				status.SessionState = "crashed"
			}
			status.WaitEstimate = 0
		} else if interactiveWaiting || looksLikePromptWaiting(agent, capture) {
			status.Status = "idle"
			status.SessionState = "waiting_input"
			status.WaitEstimate = 0
		} else if agentPID > 0 || outputFresh || !isShellCommand(paneCommand) {
			status.Status = "active"
			status.SessionState = "in_progress"
			if status.ActiveTask == "" {
				status.ActiveTask = fmt.Sprintf("%s running", strings.Title(agent))
			}
		} else {
			status.Status = "idle"
			if pollCount > 0 && pollCount <= 3 {
				status.SessionState = "just_started"
			} else {
				status.SessionState = "stuck"
			}
			status.WaitEstimate = 0
		}
	}

	if status.Status == "active" {
		state.HasEverBeenActive = true
	}
	if pollCount > 0 {
		state.PollCount = pollCount
	} else {
		state.PollCount++
	}
	_ = saveSessionState(statePath, state)

	if full && (status.SessionState == "completed" || status.SessionState == "crashed" || status.SessionState == "stuck") {
		outputFile, err := writeSessionOutputFile(projectRoot, session)
		if err == nil {
			status.OutputFile = outputFile
			if status.ActiveTask == "" {
				status.ActiveTask = outputFile
			}
		}
	}

	return status, nil
}

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
	checkRe := regexp.MustCompile(`(?i)\[( |x)\]`)
	for _, line := range lines {
		matches := checkRe.FindAllString(line, -1)
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

	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(working|running|checking|planning|writing|editing|creating|fixing|executing|reviewing)\b`),
		regexp.MustCompile(`(?i)^current task[:\s]+(.+)$`),
		regexp.MustCompile(`(?i)^active task[:\s]+(.+)$`),
	}

	ignorePrefixes := []string{
		"$", ">", "To get started", "Usage:", "Error:", "warning:", "note:",
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if containsAnyPrefix(line, ignorePrefixes) {
			continue
		}
		for _, p := range patterns {
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
		line := strings.TrimSpace(lines[i])
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

	// If recent output still looks like active work, avoid classifying as waiting.
	if regexp.MustCompile(`(?i)\b(working|running|checking|planning|writing|editing|creating|fixing|executing|reviewing|loading|reading|searching|parsing|building|compiling|installing)\b`).MatchString(last) {
		return false
	}

	if agent == "codex" {
		if regexp.MustCompile(`❯\s*([0-9]+[smh]\s*)?[0-9]{1,2}:[0-9]{2}:[0-9]{2}\s*$`).MatchString(last) {
			return true
		}
		if strings.Contains(lowerTail, "tokens used") && strings.Contains(last, "❯") {
			return true
		}
	}

	if agent == "claude" {
		if strings.HasSuffix(last, ">") || strings.HasSuffix(last, "›") {
			return true
		}
		if strings.Contains(lowerTail, "press enter to send") {
			return true
		}
	}
	return false
}

func estimateWait(task string, done, total int) int {
	lower := strings.ToLower(task)
	switch {
	case regexp.MustCompile(`loading|reading|searching|parsing`).MatchString(lower):
		return 30
	case regexp.MustCompile(`running tests|testing|building|compiling|installing`).MatchString(lower):
		return 120
	case regexp.MustCompile(`writing|editing|updating|creating|fixing`).MatchString(lower):
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
	markerRe := regexp.MustCompile(`^__LISA_EXEC_DONE__:(-?\d+)\s*$`)
	lines := trimLines(capture)
	if len(lines) == 0 {
		return false, 0
	}

	// Only trust completion markers that are still at the tail of the pane output.
	// Historical markers can linger in tmux history and should not end a new run.
	tail := make([]string, 0, 24)
	for i := len(lines) - 1; i >= 0 && len(tail) < 24; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		tail = append([]string{line}, tail...)
	}
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

func isLikelyShellPromptLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.HasSuffix(line, "$") ||
		strings.HasSuffix(line, "#") ||
		strings.HasSuffix(line, "%") ||
		strings.HasSuffix(line, "❯") {
		return true
	}

	// Fish-style prompts commonly end with '>' but should not match arbitrary
	// output like XML/HTML tags.
	if strings.HasSuffix(line, ">") {
		if strings.Contains(line, "<") {
			return false
		}
		return strings.Contains(line, "/") ||
			strings.Contains(line, "~") ||
			strings.Contains(line, `\`)
	}
	return false
}
