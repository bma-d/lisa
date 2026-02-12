package app

import (
	"bufio"
	"encoding/json"
	"fmt"
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

var findClaudeSessionIDFn = findClaudeSessionID
var readClaudeTranscriptFn = readClaudeTranscript

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

func cmdSessionCaptureTranscript(session, projectRoot string, jsonOut bool) int {
	if projectRoot == "" {
		// Try to discover from meta file
		meta, err := loadSessionMetaByGlobFn(session)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot determine project root: provide --project-root or ensure session metadata exists: %v\n", err)
			return 1
		}
		projectRoot = meta.ProjectRoot
	}
	projectRoot = canonicalProjectRoot(projectRoot)

	meta, err := loadSessionMeta(projectRoot, session)
	if err != nil {
		// Fallback to glob
		meta, err = loadSessionMetaByGlobFn(session)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot load session metadata: %v\n", err)
			return 1
		}
	}

	sessionID, err := findClaudeSessionIDFn(meta.ProjectRoot, meta.Prompt, meta.CreatedAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find Claude session: %v\n", err)
		return 1
	}

	projDir := claudeProjectDir(meta.ProjectRoot)
	jsonlPath := filepath.Join(projDir, sessionID+".jsonl")
	messages, err := readClaudeTranscriptFn(jsonlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read Claude transcript: %v\n", err)
		return 1
	}

	if jsonOut {
		writeJSON(map[string]any{
			"session":      session,
			"claudeSession": sessionID,
			"messages":     messages,
		})
		return 0
	}

	fmt.Print(formatTranscriptPlain(messages))
	return 0
}
