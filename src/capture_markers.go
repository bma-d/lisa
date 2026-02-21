package app

import (
	"fmt"
	"sort"
	"strings"
)

type captureMarkerSummary struct {
	Markers []string       `json:"markers"`
	Matches map[string]bool `json:"matches"`
	Counts  map[string]int  `json:"counts,omitempty"`
	Found   []string        `json:"found,omitempty"`
	Missing []string        `json:"missing,omitempty"`
}

func parseCaptureMarkersFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	markers := make([]string, 0, len(parts))
	for _, part := range parts {
		marker := strings.TrimSpace(part)
		if marker == "" {
			return nil, fmt.Errorf("invalid --markers: markers must be non-empty comma-separated values")
		}
		if _, exists := seen[marker]; exists {
			continue
		}
		seen[marker] = struct{}{}
		markers = append(markers, marker)
	}
	return markers, nil
}

func buildCaptureMarkerSummary(capture string, markers []string) captureMarkerSummary {
	summary := captureMarkerSummary{
		Markers: append([]string(nil), markers...),
		Matches: make(map[string]bool, len(markers)),
		Counts:  make(map[string]int, len(markers)),
		Found:   make([]string, 0, len(markers)),
		Missing: make([]string, 0, len(markers)),
	}
	for _, marker := range markers {
		count := strings.Count(capture, marker)
		summary.Counts[marker] = count
		matched := count > 0
		summary.Matches[marker] = matched
		if matched {
			summary.Found = append(summary.Found, marker)
		} else {
			summary.Missing = append(summary.Missing, marker)
		}
	}
	sort.Strings(summary.Found)
	sort.Strings(summary.Missing)
	return summary
}
