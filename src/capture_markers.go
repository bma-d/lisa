package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type captureMarkerSummary struct {
	Markers []string       `json:"markers"`
	Matches map[string]bool `json:"matches"`
	Counts  map[string]int  `json:"counts,omitempty"`
	Found   []string        `json:"found,omitempty"`
	Missing []string        `json:"missing,omitempty"`
}

type captureMarkerHit struct {
	Marker string `json:"marker"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Line   int    `json:"line"`
	At     string `json:"at,omitempty"`
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

func buildCaptureMarkerHits(capture string, markers []string) []captureMarkerHit {
	out := make([]captureMarkerHit, 0, len(markers))
	stamp := time.Now().UTC().Format(time.RFC3339)
	for _, marker := range markers {
		start := 0
		for {
			idx := strings.Index(capture[start:], marker)
			if idx < 0 {
				break
			}
			abs := start + idx
			line := 1 + strings.Count(capture[:abs], "\n")
			out = append(out, captureMarkerHit{
				Marker: marker,
				Start:  abs,
				End:    abs + len(marker),
				Line:   line,
				At:     stamp,
			})
			start = abs + len(marker)
			if start >= len(capture) {
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].Marker < out[j].Marker
		}
		return out[i].Start < out[j].Start
	})
	return out
}
