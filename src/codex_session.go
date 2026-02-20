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

type codexHistoryEntry struct {
	SessionID string `json:"session_id"`
	Ts        int64  `json:"ts"`
	Text      string `json:"text"`
}

type codexJSONLEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexEventMsgPayload struct {
	Type string `json:"type"`
}

type codexResponseItemPayload struct {
	Type    string              `json:"type"`
	Role    string              `json:"role"`
	Content []codexContentBlock `json:"content"`
}

type codexContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var checkCodexTranscriptTurnCompleteFn = checkCodexTranscriptTurnComplete

func findCodexSessionID(prompt, createdAt string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	historyPath := filepath.Join(home, ".codex", "history.jsonl")

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

	f, err := os.Open(historyPath)
	if err != nil {
		return "", fmt.Errorf("cannot open codex history: %w", err)
	}
	defer f.Close()

	var entries []codexHistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		var entry codexHistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Scan from tail
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if promptPrefix != "" && !strings.HasPrefix(entry.Text, promptPrefix) {
			continue
		}
		entryTime := time.Unix(entry.Ts, 0)
		diff := entryTime.Sub(createdTime)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 60*time.Second {
			return entry.SessionID, nil
		}
	}
	return "", fmt.Errorf("no matching Codex session found in history")
}

func findCodexSessionFile(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Strategy 1: glob sessions dir
	pattern := filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*-"+sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		return matches[0], nil
	}

	// Strategy 2: glob archived_sessions dir
	pattern = filepath.Join(home, ".codex", "archived_sessions", "*-"+sessionID+".jsonl")
	matches, err = filepath.Glob(pattern)
	if err == nil && len(matches) > 0 {
		return matches[0], nil
	}

	return "", fmt.Errorf("no Codex session file found for %s", sessionID)
}

func checkCodexTranscriptTurnComplete(prompt, createdAt, cachedSessionID string) (turnComplete bool, fileAge int, sessionID string, err error) {
	sid := cachedSessionID
	if sid == "" {
		sid, err = findCodexSessionID(prompt, createdAt)
		if err != nil {
			return false, 0, "", err
		}
	}
	sessionID = sid

	sessionPath, err := findCodexSessionFile(sid)
	if err != nil {
		return false, 0, sessionID, err
	}

	info, err := os.Stat(sessionPath)
	if err != nil {
		return false, 0, sessionID, fmt.Errorf("cannot stat codex transcript: %w", err)
	}
	fileAge = int(time.Since(info.ModTime()).Seconds())
	if fileAge < 3 {
		return false, fileAge, sessionID, nil
	}

	f, err := os.Open(sessionPath)
	if err != nil {
		return false, fileAge, sessionID, fmt.Errorf("cannot open codex transcript: %w", err)
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
		return false, fileAge, sessionID, fmt.Errorf("cannot seek codex transcript: %w", err)
	}

	var entries []codexJSONLEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, tailBytes), tailBytes+1024)
	first := offset > 0
	for scanner.Scan() {
		if first {
			first = false
			continue // skip potentially partial first line
		}
		var entry codexJSONLEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Walk backwards, skip non-meaningful entries
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		switch e.Type {
		case "session_meta", "turn_context":
			continue
		case "event_msg":
			var payload codexEventMsgPayload
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return false, fileAge, sessionID, nil
			}
			switch payload.Type {
			case "token_count", "task_started", "task_complete", "agent_reasoning":
				continue
			case "agent_message":
				return true, fileAge, sessionID, nil
			default:
				return false, fileAge, sessionID, nil
			}
		case "response_item":
			var payload codexResponseItemPayload
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return false, fileAge, sessionID, nil
			}
			if payload.Type == "message" && payload.Role == "assistant" {
				for _, block := range payload.Content {
					if block.Type == "output_text" {
						return true, fileAge, sessionID, nil
					}
				}
			}
			return false, fileAge, sessionID, nil
		case "user_message":
			return false, fileAge, sessionID, nil
		default:
			continue
		}
	}

	return false, fileAge, sessionID, nil
}
