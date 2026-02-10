package app

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var tmuxShowEnvironmentFn = tmuxShowEnvironment
var tmuxHasSessionFn = tmuxHasSession
var tmuxKillSessionFn = tmuxKillSession
var tmuxListSessionsFn = tmuxListSessions
var tmuxCapturePaneFn = tmuxCapturePane
var tmuxDisplayFn = tmuxDisplay
var tmuxPaneStatusFn = tmuxPaneStatus
var detectAgentProcessFn = detectAgentProcess

func tmuxNewSession(session, projectRoot, agent, mode string, width, height int) error {
	_, err := runCmd("tmux", "new-session", "-d", "-s", session,
		"-x", strconv.Itoa(width),
		"-y", strconv.Itoa(height),
		"-c", projectRoot,
		"-e", "LISA_SESSION=true",
		"-e", "LISA_AGENT="+agent,
		"-e", "LISA_MODE="+mode,
		"-e", "LISA_PROJECT_HASH="+projectHash(projectRoot),
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

func detectAgentProcess(panePID int, agent string) (int, float64) {
	if panePID <= 0 {
		return 0, 0
	}
	procs, err := listProcesses()
	if err != nil {
		return 0, 0
	}
	children := map[int][]processInfo{}
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
	}

	queue := []int{panePID}
	seen := map[int]bool{}
	needle := "claude"
	if agent == "codex" {
		needle = "codex"
	}

	bestPID := 0
	bestCPU := 0.0
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
			if strings.Contains(cmdLower, needle) && !strings.Contains(cmdLower, "grep") {
				if child.CPU > bestCPU {
					bestCPU = child.CPU
					bestPID = child.PID
				}
			}
		}
	}
	return bestPID, bestCPU
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
