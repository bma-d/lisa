package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type sessionObjectiveRecord struct {
	ID         string `json:"id"`
	Goal       string `json:"goal"`
	Acceptance string `json:"acceptance,omitempty"`
	Budget     int    `json:"budget,omitempty"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
}

type sessionObjectiveStore struct {
	CurrentID  string                            `json:"currentId,omitempty"`
	Objectives map[string]sessionObjectiveRecord `json:"objectives"`
	UpdatedAt  string                            `json:"updatedAt"`
}

type sessionLaneRecord struct {
	Name          string `json:"name"`
	Goal          string `json:"goal,omitempty"`
	Agent         string `json:"agent,omitempty"`
	Mode          string `json:"mode,omitempty"`
	NestedPolicy  string `json:"nestedPolicy,omitempty"`
	NestingIntent string `json:"nestingIntent,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Model         string `json:"model,omitempty"`
	Budget        int    `json:"budget,omitempty"`
	Topology      string `json:"topology,omitempty"`
	Contract      string `json:"contract,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
}

type sessionLaneStore struct {
	Lanes     map[string]sessionLaneRecord `json:"lanes"`
	UpdatedAt string                       `json:"updatedAt"`
}

type sessionMemoryRecord struct {
	Session   string   `json:"session"`
	UpdatedAt string   `json:"updatedAt"`
	ExpiresAt string   `json:"expiresAt"`
	MaxLines  int      `json:"maxLines"`
	Lines     []string `json:"lines"`
}

func objectivesRegistryFile(projectRoot string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-objectives.json", projectHash(projectRoot))
}

func lanesRegistryFile(projectRoot string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-lanes.json", projectHash(projectRoot))
}

func sessionMemoryFile(projectRoot, session string) string {
	return fmt.Sprintf("/tmp/.lisa-%s-session-%s-memory.json", projectHash(projectRoot), sessionArtifactID(session))
}

func loadObjectiveStore(projectRoot string) (sessionObjectiveStore, error) {
	path := objectivesRegistryFile(projectRoot)
	if !fileExists(path) {
		return sessionObjectiveStore{Objectives: map[string]sessionObjectiveRecord{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionObjectiveStore{}, err
	}
	store := sessionObjectiveStore{}
	if err := json.Unmarshal(raw, &store); err != nil {
		return sessionObjectiveStore{}, err
	}
	if store.Objectives == nil {
		store.Objectives = map[string]sessionObjectiveRecord{}
	}
	return store, nil
}

func saveObjectiveStore(projectRoot string, store sessionObjectiveStore) error {
	if store.Objectives == nil {
		store.Objectives = map[string]sessionObjectiveRecord{}
	}
	store.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(objectivesRegistryFile(projectRoot), raw)
}

func pruneExpiredObjectives(store sessionObjectiveStore) sessionObjectiveStore {
	if store.Objectives == nil {
		store.Objectives = map[string]sessionObjectiveRecord{}
		return store
	}
	now := nowFn().UTC()
	for id, record := range store.Objectives {
		expiresAt := strings.TrimSpace(record.ExpiresAt)
		if expiresAt == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			continue
		}
		if now.After(ts) {
			delete(store.Objectives, id)
			if store.CurrentID == id {
				store.CurrentID = ""
			}
		}
	}
	return store
}

func getCurrentObjective(projectRoot string) (sessionObjectiveRecord, bool) {
	store, err := loadObjectiveStore(projectRoot)
	if err != nil {
		return sessionObjectiveRecord{}, false
	}
	store = pruneExpiredObjectives(store)
	if strings.TrimSpace(store.CurrentID) == "" {
		return sessionObjectiveRecord{}, false
	}
	record, ok := store.Objectives[store.CurrentID]
	if !ok {
		return sessionObjectiveRecord{}, false
	}
	if strings.TrimSpace(record.Status) == "" {
		record.Status = "open"
	}
	return record, true
}

func objectivePayloadFromMeta(meta sessionMeta) map[string]any {
	id := strings.TrimSpace(meta.ObjectiveID)
	goal := strings.TrimSpace(meta.ObjectiveGoal)
	if id == "" && goal == "" {
		return nil
	}
	payload := map[string]any{}
	if id != "" {
		payload["id"] = id
	}
	if goal != "" {
		payload["goal"] = goal
	}
	if strings.TrimSpace(meta.ObjectiveAcceptance) != "" {
		payload["acceptance"] = meta.ObjectiveAcceptance
	}
	if meta.ObjectiveBudget > 0 {
		payload["budget"] = meta.ObjectiveBudget
	}
	if strings.TrimSpace(meta.Lane) != "" {
		payload["lane"] = meta.Lane
	}
	return payload
}

func loadLaneStore(projectRoot string) (sessionLaneStore, error) {
	path := lanesRegistryFile(projectRoot)
	if !fileExists(path) {
		return sessionLaneStore{Lanes: map[string]sessionLaneRecord{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionLaneStore{}, err
	}
	store := sessionLaneStore{}
	if err := json.Unmarshal(raw, &store); err != nil {
		return sessionLaneStore{}, err
	}
	if store.Lanes == nil {
		store.Lanes = map[string]sessionLaneRecord{}
	}
	return store, nil
}

func saveLaneStore(projectRoot string, store sessionLaneStore) error {
	if store.Lanes == nil {
		store.Lanes = map[string]sessionLaneRecord{}
	}
	store.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(lanesRegistryFile(projectRoot), raw)
}

func loadLaneRecord(projectRoot, name string) (sessionLaneRecord, bool, error) {
	store, err := loadLaneStore(projectRoot)
	if err != nil {
		return sessionLaneRecord{}, false, err
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	record, ok := store.Lanes[normalized]
	return record, ok, nil
}

func laneNames(store sessionLaneStore) []string {
	names := make([]string, 0, len(store.Lanes))
	for name := range store.Lanes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func loadSessionMemory(projectRoot, session string) (sessionMemoryRecord, bool, error) {
	path := sessionMemoryFile(projectRoot, session)
	if !fileExists(path) {
		return sessionMemoryRecord{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return sessionMemoryRecord{}, false, err
	}
	record := sessionMemoryRecord{}
	if err := json.Unmarshal(raw, &record); err != nil {
		return sessionMemoryRecord{}, false, err
	}
	record.Lines = dedupeNonEmpty(record.Lines)
	if record.MaxLines <= 0 {
		record.MaxLines = 80
	}
	if expires := strings.TrimSpace(record.ExpiresAt); expires != "" {
		ts, err := time.Parse(time.RFC3339, expires)
		if err == nil && nowFn().UTC().After(ts) {
			return sessionMemoryRecord{}, false, nil
		}
	}
	return record, true, nil
}

func saveSessionMemory(projectRoot, session string, record sessionMemoryRecord) error {
	record.Session = session
	record.Lines = dedupeNonEmpty(record.Lines)
	if record.MaxLines <= 0 {
		record.MaxLines = 80
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(sessionMemoryFile(projectRoot, session), raw)
}

func captureSessionSemanticLines(projectRoot, session string, lines int) []string {
	capture := ""
	restore := withProjectRuntimeEnv(projectRoot)
	active := tmuxHasSessionFn(session)
	restore()
	if active {
		restore = withProjectRuntimeEnv(projectRoot)
		pane, err := tmuxCapturePaneFn(session, lines)
		restore()
		if err == nil {
			capture = pane
		}
	}
	if strings.TrimSpace(capture) == "" {
		raw, err := os.ReadFile(sessionOutputFile(projectRoot, session))
		if err == nil {
			capture = string(raw)
		}
	}
	return extractSemanticLines(capture)
}

func buildSessionMemory(projectRoot, session string, maxLines, ttlHours int) (sessionMemoryRecord, []string, error) {
	if maxLines <= 0 {
		maxLines = 80
	}
	if ttlHours <= 0 {
		ttlHours = 24
	}
	previous, _, err := loadSessionMemory(projectRoot, session)
	if err != nil {
		return sessionMemoryRecord{}, nil, err
	}
	current := captureSessionSemanticLines(projectRoot, session, 320)
	if len(current) > maxLines {
		current = current[len(current)-maxLines:]
	}
	delta := computeSemanticDelta(current, previous.Lines)
	now := nowFn().UTC()
	record := sessionMemoryRecord{
		Session:   session,
		UpdatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(time.Duration(ttlHours) * time.Hour).Format(time.RFC3339),
		MaxLines:  maxLines,
		Lines:     current,
	}
	if err := saveSessionMemory(projectRoot, session, record); err != nil {
		return sessionMemoryRecord{}, nil, err
	}
	return record, delta, nil
}

func buildObjectivePromptPrefix(record sessionObjectiveRecord, lane string) string {
	parts := []string{}
	if strings.TrimSpace(record.ID) != "" {
		parts = append(parts, "id="+record.ID)
	}
	if strings.TrimSpace(record.Goal) != "" {
		parts = append(parts, "goal="+record.Goal)
	}
	if strings.TrimSpace(record.Acceptance) != "" {
		parts = append(parts, "acceptance="+record.Acceptance)
	}
	if record.Budget > 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", record.Budget))
	}
	if strings.TrimSpace(lane) != "" {
		parts = append(parts, "lane="+lane)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Objective context: " + strings.Join(parts, " | ")
}

func injectObjectiveIntoPrompt(prompt string, record sessionObjectiveRecord, lane string) string {
	prefix := buildObjectivePromptPrefix(record, lane)
	if prefix == "" {
		return prompt
	}
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return prefix
	}
	lowerPrompt := strings.ToLower(trimmedPrompt)
	if strings.TrimSpace(record.Goal) != "" && strings.Contains(lowerPrompt, strings.ToLower(record.Goal)) {
		return prompt
	}
	if strings.TrimSpace(record.ID) != "" && strings.Contains(lowerPrompt, strings.ToLower(record.ID)) {
		return prompt
	}
	return prefix + "\n\n" + prompt
}

func buildObjectiveSendPrefix(meta sessionMeta) string {
	payload := objectivePayloadFromMeta(meta)
	if payload == nil {
		return ""
	}
	parts := []string{}
	if v := strings.TrimSpace(meta.ObjectiveID); v != "" {
		parts = append(parts, "id="+v)
	}
	if v := strings.TrimSpace(meta.ObjectiveGoal); v != "" {
		parts = append(parts, "goal="+v)
	}
	if v := strings.TrimSpace(meta.ObjectiveAcceptance); v != "" {
		parts = append(parts, "acceptance="+v)
	}
	if meta.ObjectiveBudget > 0 {
		parts = append(parts, fmt.Sprintf("budget=%d", meta.ObjectiveBudget))
	}
	if v := strings.TrimSpace(meta.Lane); v != "" {
		parts = append(parts, "lane="+v)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Objective reminder: " + strings.Join(parts, " | ")
}

func loadSessionMemoryCompact(projectRoot, session string, maxLines int) (map[string]any, bool) {
	record, ok, err := loadSessionMemory(projectRoot, session)
	if err != nil || !ok {
		return nil, false
	}
	lines := append([]string{}, record.Lines...)
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	payload := map[string]any{
		"updatedAt": record.UpdatedAt,
		"expiresAt": record.ExpiresAt,
		"lineCount": len(record.Lines),
		"lines":     lines,
	}
	return payload, true
}
