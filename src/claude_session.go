package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type claudeJSONLEntry struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type transcriptMessage struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type claudeHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

type claudeJSONLTailEntry struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
}

var findClaudeSessionIDFn = findClaudeSessionID
var readClaudeTranscriptFn = readClaudeTranscript
var checkTranscriptTurnCompleteFn = checkTranscriptTurnComplete

func encodeClaudePath(absPath string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(absPath)
}

func claudeProjectDir(projectRoot string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".claude", "projects", encodeClaudePath(projectRoot))
}

func findClaudeSessionID(projectRoot, prompt, createdAt string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	historyPath := filepath.Join(home, ".claude", "history.jsonl")

	createdTime, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		createdTime, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return "", fmt.Errorf("cannot parse createdAt %q: %w", createdAt, err)
		}
	}

	promptPrefix := prompt
	if len(promptPrefix) > 80 {
		promptPrefix = promptPrefix[:80]
	}

	// Strategy 1: scan history.jsonl from tail
	if sessionID, err := scanHistoryForSession(historyPath, projectRoot, promptPrefix, createdTime); err == nil && sessionID != "" {
		projDir := claudeProjectDir(projectRoot)
		jsonlPath := filepath.Join(projDir, sessionID+".jsonl")
		if fileExists(jsonlPath) {
			return sessionID, nil
		}
	}

	// Strategy 2: glob project dir, check first user message
	projDir := claudeProjectDir(projectRoot)
	return scanProjectDirForSession(projDir, promptPrefix, createdTime)
}

func scanHistoryForSession(historyPath, projectRoot, promptPrefix string, createdTime time.Time) (string, error) {
	f, err := os.Open(historyPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var entries []claudeHistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		var entry claudeHistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Scan from tail
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Project != projectRoot {
			continue
		}
		if promptPrefix != "" && !strings.HasPrefix(entry.Display, promptPrefix) {
			continue
		}
		entryTime := time.UnixMilli(entry.Timestamp)
		diff := entryTime.Sub(createdTime)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 60*time.Second {
			return entry.SessionID, nil
		}
	}
	return "", nil
}

func scanProjectDirForSession(projDir, promptPrefix string, createdTime time.Time) (string, error) {
	matches, err := filepath.Glob(filepath.Join(projDir, "*.jsonl"))
	if err != nil {
		return "", err
	}

	type candidate struct {
		sessionID string
		diff      time.Duration
	}
	var best *candidate

	for _, path := range matches {
		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
		found := false
		for scanner.Scan() {
			var entry claudeJSONLEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			if entry.Type != "user" {
				continue
			}

			var msg claudeMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				break
			}
			userText := extractMessageText(entry.Message, "user")
			if promptPrefix != "" && !strings.HasPrefix(userText, promptPrefix) {
				break
			}

			entryTime, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil {
				entryTime, err = time.Parse(time.RFC3339, entry.Timestamp)
				if err != nil {
					break
				}
			}
			diff := entryTime.Sub(createdTime)
			if diff < 0 {
				diff = -diff
			}
			if diff <= 120*time.Second {
				if best == nil || diff < best.diff {
					best = &candidate{sessionID: sessionID, diff: diff}
				}
			}
			found = true
			break
		}
		f.Close()
		_ = found
	}

	if best != nil {
		return best.sessionID, nil
	}
	return "", fmt.Errorf("no matching Claude session found in %s", projDir)
}

func readClaudeTranscript(jsonlPath string) ([]transcriptMessage, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []transcriptMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	for scanner.Scan() {
		var entry claudeJSONLEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "user" && entry.Type != "assistant" {
			continue
		}
		text := extractMessageText(entry.Message, entry.Type)
		if text == "" {
			continue
		}
		messages = append(messages, transcriptMessage{
			Role:      entry.Type,
			Text:      text,
			Timestamp: entry.Timestamp,
		})
	}
	return messages, scanner.Err()
}

func extractMessageText(raw json.RawMessage, role string) string {
	var msg claudeMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	// User content is a plain string
	if msg.Role == "user" || role == "user" {
		var s string
		if err := json.Unmarshal(msg.Content, &s); err == nil {
			return strings.TrimSpace(s)
		}
	}

	// Assistant content is []block — extract text blocks only
	var blocks []claudeContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

func checkTranscriptTurnComplete(projectRoot, prompt, createdAt, cachedSessionID string) (turnComplete bool, fileAge int, sessionID string, err error) {
	sid := cachedSessionID
	if sid == "" {
		sid, err = findClaudeSessionIDFn(projectRoot, prompt, createdAt)
		if err != nil {
			return false, 0, "", err
		}
	}
	sessionID = sid

	projDir := claudeProjectDir(projectRoot)
	jsonlPath := filepath.Join(projDir, sid+".jsonl")

	info, err := os.Stat(jsonlPath)
	if err != nil {
		return false, 0, sessionID, fmt.Errorf("cannot stat transcript: %w", err)
	}
	fileAge = int(time.Since(info.ModTime()).Seconds())
	if fileAge < 3 {
		return false, fileAge, sessionID, nil
	}

	f, err := os.Open(jsonlPath)
	if err != nil {
		return false, fileAge, sessionID, fmt.Errorf("cannot open transcript: %w", err)
	}
	defer f.Close()

	// Read last 8KB
	const tailBytes = 8 * 1024
	fSize := info.Size()
	offset := int64(0)
	if fSize > tailBytes {
		offset = fSize - tailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return false, fileAge, sessionID, fmt.Errorf("cannot seek transcript: %w", err)
	}

	var entries []claudeJSONLTailEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailBytes), tailBytes+1024)
	first := offset > 0
	for scanner.Scan() {
		if first {
			first = false
			continue // skip potentially partial first line
		}
		var entry claudeJSONLTailEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Walk backwards, skip non-meaningful entries
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		switch e.Type {
		case "progress", "system", "file-history-snapshot":
			continue
		case "assistant":
			return assistantHasTextBlock(e.Message), fileAge, sessionID, nil
		default:
			// user, tool_result, etc. → not turn-complete
			return false, fileAge, sessionID, nil
		}
	}

	return false, fileAge, sessionID, nil
}

func assistantHasTextBlock(raw json.RawMessage) bool {
	var msg claudeMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return false
	}
	var blocks []claudeContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return true
		}
	}
	return false
}

func formatTranscriptPlain(messages []transcriptMessage) string {
	var sb strings.Builder
	for i, msg := range messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		if msg.Role == "user" {
			sb.WriteString("> ")
			sb.WriteString(msg.Text)
			sb.WriteString("\n")
		} else {
			sb.WriteString(msg.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func writeTranscriptCapture(session, sessionID string, messages []transcriptMessage, jsonOut bool) int {
	if jsonOut {
		writeJSON(map[string]any{
			"session":       session,
			"claudeSession": sessionID,
			"messages":      messages,
		})
		return 0
	}

	fmt.Print(formatTranscriptPlain(messages))
	return 0
}

func captureSessionTranscript(session, projectRoot string) (string, []transcriptMessage, error) {
	var (
		meta sessionMeta
		err  error
	)
	if strings.TrimSpace(projectRoot) == "" {
		meta, err = loadSessionMetaByGlobFn(session)
		if err != nil {
			return "", nil, fmt.Errorf("cannot determine project root: provide --project-root or ensure session metadata exists: %w", err)
		}
		projectRoot = canonicalProjectRoot(meta.ProjectRoot)
	} else {
		projectRoot = canonicalProjectRoot(projectRoot)
		meta, err = loadSessionMeta(projectRoot, session)
		if err != nil {
			return "", nil, fmt.Errorf("cannot load session metadata: %w", err)
		}
	}
	if strings.TrimSpace(meta.Prompt) == "" || strings.TrimSpace(meta.CreatedAt) == "" {
		return "", nil, fmt.Errorf("cannot find Claude transcript: session metadata missing prompt/createdAt")
	}

	sessionID, err := findClaudeSessionIDFn(meta.ProjectRoot, meta.Prompt, meta.CreatedAt)
	if err != nil {
		return "", nil, fmt.Errorf("cannot find Claude session: %w", err)
	}

	projDir := claudeProjectDir(meta.ProjectRoot)
	jsonlPath := filepath.Join(projDir, sessionID+".jsonl")
	messages, err := readClaudeTranscriptFn(jsonlPath)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read Claude transcript: %w", err)
	}
	return sessionID, messages, nil
}

func cmdSessionCaptureTranscript(session, projectRoot string, jsonOut bool) int {
	sessionID, messages, err := captureSessionTranscript(session, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return writeTranscriptCapture(session, sessionID, messages, jsonOut)
}
