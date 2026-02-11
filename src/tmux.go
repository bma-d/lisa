package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var tmuxShowEnvironmentFn = tmuxShowEnvironment
var tmuxHasSessionFn = tmuxHasSession
var tmuxKillSessionFn = tmuxKillSession
var tmuxListSessionsFn = tmuxListSessions
var tmuxCapturePaneFn = tmuxCapturePane
var tmuxSendTextFn = tmuxSendText
var tmuxSendKeysFn = tmuxSendKeys
var tmuxNewSessionFn = tmuxNewSession
var tmuxSendCommandWithFallbackFn = tmuxSendCommandWithFallback
var tmuxDisplayFn = tmuxDisplay
var tmuxPaneStatusFn = tmuxPaneStatus
var detectAgentProcessFn = detectAgentProcess
var listProcessesFn = listProcesses
var listProcessesCachedFn = listProcessesCached

var processCache = struct {
	mu      sync.Mutex
	fnPtr   uintptr
	atNanos int64
	procs   []processInfo
}{}

func tmuxNewSession(session, projectRoot, agent, mode string, width, height int) error {
	_, err := runCmd("tmux", "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width),
		"-y", strconv.Itoa(height),
		"-c", projectRoot,
		"-e", "LISA_SESSION=true",
		"-e", "LISA_SESSION_NAME="+session,
		"-e", "LISA_AGENT="+agent,
		"-e", "LISA_MODE="+mode,
		"-e", "LISA_PROJECT_HASH="+projectHash(projectRoot),
		"-e", "LISA_HEARTBEAT_FILE="+sessionHeartbeatFile(projectRoot, session),
		"-e", "LISA_DONE_FILE="+sessionDoneFile(projectRoot, session),
	)
	return err
}

func tmuxSendCommandWithFallback(session, command string, enter bool) error {
	if len(command) <= maxInlineSendLength {
		return tmuxSendKeys(session, []string{command}, enter)
	}

	scriptPath := sessionCommandScriptPath(session, time.Now().UnixNano())
	body := buildFallbackScriptBody(command)
	if err := os.WriteFile(scriptPath, []byte(body), 0o700); err != nil {
		return fmt.Errorf("failed to write long command script: %w", err)
	}
	return tmuxSendKeys(session, []string{"bash " + shellQuote(scriptPath)}, enter)
}

func buildFallbackScriptBody(command string) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	// Preserve exec completion markers even when wrapped commands fail.
	if strings.Contains(command, execDonePrefix) {
		b.WriteString("set +e\n")
	}
	b.WriteString(command)
	if !strings.HasSuffix(command, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func tmuxSendText(session, text string, enter bool) error {
	bufName := fmt.Sprintf("lisa-send-%d", time.Now().UnixNano())
	out, err := runCmdInput(text, "tmux", "load-buffer", "-b", bufName, "-")
	if err != nil {
		return fmt.Errorf("failed to load tmux buffer: %v (%s)", err, strings.TrimSpace(out))
	}

	defer func() {
		_, _ = runCmd("tmux", "delete-buffer", "-b", bufName)
	}()

	if _, err := runCmd("tmux", "paste-buffer", "-b", bufName, "-t", session); err != nil {
		return fmt.Errorf("failed to paste tmux buffer: %w", err)
	}
	if enter {
		if _, err := runCmd("tmux", "send-keys", "-t", session, "Enter"); err != nil {
			return fmt.Errorf("failed to send enter: %w", err)
		}
	}
	return nil
}

func tmuxSendKeys(session string, keys []string, enter bool) error {
	args := []string{"send-keys", "-t", session}
	args = append(args, keys...)
	if enter {
		args = append(args, "Enter")
	}
	_, err := runCmd("tmux", args...)
	return err
}

func tmuxHasSession(session string) bool {
	_, err := runCmd("tmux", "has-session", "-t", session)
	return err == nil
}

func tmuxKillSession(session string) error {
	_, err := runCmd("tmux", "kill-session", "-t", session)
	return err
}

func tmuxListSessions(projectOnly bool, projectRoot string) ([]string, error) {
	out, err := runCmd("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if isTmuxNoSessionsOutput(out) {
			return []string{}, nil
		}
		return nil, err
	}
	lines := trimLines(out)
	filtered := make([]string, 0, len(lines))
	expectedProjectHash := projectHash(projectRoot)
	for _, s := range lines {
		if !strings.HasPrefix(s, "lisa-") {
			continue
		}
		if projectOnly {
			if !sessionMatchesProjectRoot(s, projectRoot, expectedProjectHash) {
				continue
			}
		}
		filtered = append(filtered, s)
	}
	sort.Strings(filtered)
	return filtered, nil
}

func isTmuxNoSessionsOutput(output string) bool {
	msg := strings.ToLower(strings.TrimSpace(output))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "no server running") ||
		strings.Contains(msg, "failed to connect to server") ||
		msg == "no sessions" ||
		strings.HasPrefix(msg, "no sessions ")
}

func sessionMatchesProjectRoot(session, projectRoot, expectedProjectHash string) bool {
	if expectedProjectHash == "" {
		expectedProjectHash = projectHash(projectRoot)
	}

	if hash, err := tmuxShowEnvironmentFn(session, "LISA_PROJECT_HASH"); err == nil {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			// Key absent in older sessions; fall back to legacy matching.
		} else if hash == expectedProjectHash {
			return true
		} else {
			return false
		}
	}

	// Legacy fallback for sessions created before LISA_PROJECT_HASH.
	return fileExists(sessionMetaFile(projectRoot, session))
}

func tmuxDisplay(session, format string) (string, error) {
	out, err := runCmd("tmux", "display-message", "-t", session, "-p", format)
	return strings.TrimSpace(out), err
}

func tmuxShowEnvironment(session, key string) (string, error) {
	out, err := runCmd("tmux", "show-environment", "-t", session, key)
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "=", 2)
	if len(parts) == 2 {
		return parts[1], nil
	}
	return "", nil
}

func tmuxPaneStatus(session string) (string, error) {
	dead, err := tmuxDisplayFn(session, "#{pane_dead}")
	if err != nil {
		return "", err
	}
	exit, err := tmuxDisplayFn(session, "#{pane_dead_status}")
	if err != nil {
		return "", err
	}
	if dead == "1" {
		if exit != "" {
			return "exited:" + exit, nil
		}
		return "exited:0", nil
	}
	if exit != "" && exit != "0" {
		return "crashed:" + exit, nil
	}
	return "alive", nil
}

func tmuxCapturePane(session string, lines int) (string, error) {
	return runCmd("tmux", "capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lines))
}

func detectAgentProcess(panePID int, agent string) (int, float64, error) {
	if panePID <= 0 {
		return 0, 0, nil
	}
	procs, err := listProcessesCachedFn()
	if err != nil {
		return 0, 0, fmt.Errorf("process scan failed: %w", err)
	}
	children := map[int][]processInfo{}
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
	}

	queue := []int{panePID}
	seen := map[int]bool{}
	primaryExec := agentPrimaryExecutable(agent)
	customNeedles := agentProcessNeedles(agent)

	bestPID := 0
	bestCPU := -1.0
	bestScore := -1
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, child := range children[cur] {
			queue = append(queue, child.PID)
			cmdLower := strings.ToLower(child.Command)
			execName := commandExecutableName(cmdLower)
			strictMatch := executableMatchesAgent(execName, primaryExec)
			wrapperMatch := commandReferencesPrimaryBinary(cmdLower, primaryExec)
			customMatch := matchesAnyNeedleWord(cmdLower, customNeedles)
			if !strictMatch && !wrapperMatch && !customMatch {
				continue
			}
			if strings.Contains(cmdLower, "grep") {
				continue
			}
			score := 1
			if strictMatch {
				score = 3
			} else if wrapperMatch {
				score = 2
			}
			if score > bestScore || (score == bestScore && (bestPID == 0 || child.CPU > bestCPU)) {
				bestScore = score
				bestCPU = child.CPU
				bestPID = child.PID
			}
		}
	}
	if bestPID == 0 {
		return 0, 0, nil
	}
	return bestPID, bestCPU, nil
}

func listProcessesCached() ([]processInfo, error) {
	cacheMS := getIntEnv("LISA_PROCESS_LIST_CACHE_MS", defaultProcessListCacheMS)
	if cacheMS <= 0 {
		cacheMS = defaultProcessListCacheMS
	}
	nowNanos := time.Now().UnixNano()
	ttlNanos := int64(cacheMS) * int64(time.Millisecond)
	currentFnPtr := reflect.ValueOf(listProcessesFn).Pointer()

	processCache.mu.Lock()
	if processCache.fnPtr == currentFnPtr && processCache.atNanos > 0 && (nowNanos-processCache.atNanos) < ttlNanos {
		procs := make([]processInfo, len(processCache.procs))
		copy(procs, processCache.procs)
		processCache.mu.Unlock()
		return procs, nil
	}
	processCache.mu.Unlock()

	procs, err := listProcessesFn()
	if err != nil {
		processCache.mu.Lock()
		processCache.fnPtr = currentFnPtr
		processCache.atNanos = 0
		processCache.procs = nil
		processCache.mu.Unlock()
		return nil, err
	}

	processCache.mu.Lock()
	processCache.fnPtr = currentFnPtr
	processCache.atNanos = nowNanos
	copied := make([]processInfo, len(procs))
	copy(copied, procs)
	processCache.procs = copied
	processCache.mu.Unlock()
	return procs, nil
}

func agentProcessNeedles(agent string) []string {
	needles := []string{}
	needles = append(needles, parseNeedleEnv("LISA_AGENT_PROCESS_MATCH")...)
	agent = strings.ToLower(strings.TrimSpace(agent))
	switch agent {
	case "codex":
		needles = append(needles, parseNeedleEnv("LISA_AGENT_PROCESS_MATCH_CODEX")...)
	default:
		needles = append(needles, parseNeedleEnv("LISA_AGENT_PROCESS_MATCH_CLAUDE")...)
	}

	out := make([]string, 0, len(needles))
	seen := map[string]bool{}
	for _, n := range needles {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func agentPrimaryExecutable(agent string) string {
	agent = strings.ToLower(strings.TrimSpace(agent))
	if agent == "codex" {
		return "codex"
	}
	return "claude"
}

func commandExecutableName(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(filepath.Base(fields[0])))
}

func executableMatchesAgent(executable, primary string) bool {
	if executable == "" || primary == "" {
		return false
	}
	return executable == primary ||
		strings.HasPrefix(executable, primary+"-") ||
		strings.HasSuffix(executable, "-"+primary)
}

func commandReferencesPrimaryBinary(command, primary string) bool {
	if primary == "" {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) < 2 {
		return false
	}

	runner := strings.ToLower(strings.TrimSpace(filepath.Base(fields[0])))
	idx := 1
	if runner == "env" {
		for idx < len(fields) && strings.Contains(fields[idx], "=") {
			idx++
		}
		if idx >= len(fields) {
			return false
		}
		runner = strings.ToLower(strings.TrimSpace(filepath.Base(fields[idx])))
		idx++
	}
	if !isWrapperRunner(runner) {
		return false
	}
	for ; idx < len(fields); idx++ {
		token := strings.TrimSpace(fields[idx])
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}
		target := strings.ToLower(strings.TrimSpace(filepath.Base(token)))
		return executableMatchesAgent(target, primary)
	}
	return false
}

func isWrapperRunner(executable string) bool {
	switch executable {
	case "bash", "sh", "zsh", "dash", "fish", "python", "python3", "node", "ruby", "perl":
		return true
	default:
		return false
	}
}

func parseNeedleEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matchesAnyNeedleWord(command string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && containsWordToken(command, needle) {
			return true
		}
	}
	return false
}

func containsWordToken(haystack, needle string) bool {
	haystack = strings.ToLower(haystack)
	needle = strings.ToLower(strings.TrimSpace(needle))
	if haystack == "" || needle == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return false
		}
		pos := start + idx
		prevOK := pos == 0 || !isTokenChar(rune(haystack[pos-1]))
		next := pos + len(needle)
		nextOK := next == len(haystack) || !isTokenChar(rune(haystack[next]))
		if prevOK && nextOK {
			return true
		}
		start = pos + 1
		if start >= len(haystack) {
			return false
		}
	}
}

func isTokenChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-'
}

func listProcesses() ([]processInfo, error) {
	out, err := runCmd("ps", "-axo", "pid=,ppid=,%cpu=,command=")
	if err != nil {
		return nil, err
	}
	lines := trimLines(out)
	procs := make([]processInfo, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := splitNWhitespace(line, 4)
		if len(parts) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(parts[0])
		ppid, err2 := strconv.Atoi(parts[1])
		cpu, err3 := strconv.ParseFloat(parts[2], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		procs = append(procs, processInfo{
			PID:     pid,
			PPID:    ppid,
			CPU:     cpu,
			Command: parts[3],
		})
	}
	return procs, nil
}

func splitNWhitespace(input string, n int) []string {
	fields := []string{}
	cur := strings.TrimSpace(input)
	for i := 0; i < n-1; i++ {
		if cur == "" {
			break
		}
		idx := strings.IndexAny(cur, " \t")
		if idx < 0 {
			fields = append(fields, cur)
			cur = ""
			break
		}
		fields = append(fields, strings.TrimSpace(cur[:idx]))
		cur = strings.TrimLeft(cur[idx:], " \t")
	}
	if cur != "" {
		fields = append(fields, cur)
	}
	return fields
}
