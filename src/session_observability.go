package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ensureHeartbeatWritableFn = ensureHeartbeatWritable
var withStateFileLockFn = withStateFileLock
var appendSessionEventFn = appendSessionEvent
var readSessionEventTailFn = readSessionEventTail
var pruneStaleSessionEventArtifactsFn = pruneStaleSessionEventArtifacts

type stateLockMeta struct {
	WaitMS int
}

type stateLockTimeoutError struct {
	WaitMS int
}

func (e *stateLockTimeoutError) Error() string {
	return fmt.Sprintf("state lock timeout after %dms", e.WaitMS)
}

func stateLockTimeoutWaitMS(err error) (int, bool) {
	var timeoutErr *stateLockTimeoutError
	if errors.As(err, &timeoutErr) {
		return timeoutErr.WaitMS, true
	}
	return 0, false
}

type sessionEventTail struct {
	Events       []sessionEvent `json:"events"`
	DroppedLines int            `json:"droppedLines"`
	NextCursor   int            `json:"nextCursor,omitempty"`
}

func ensureHeartbeatWritable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func withStateFileLock(statePath string, fn func() error) (stateLockMeta, error) {
	lockPath := statePath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return stateLockMeta{}, err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return stateLockMeta{}, err
	}
	defer lockFile.Close()

	timeoutMS := getIntEnv("LISA_STATE_LOCK_TIMEOUT_MS", defaultStateLockTimeoutMS)
	if timeoutMS <= 0 {
		timeoutMS = defaultStateLockTimeoutMS
	}

	start := time.Now()
	waited := 0
	for {
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				waited = int(time.Since(start).Milliseconds())
				if waited >= timeoutMS {
					return stateLockMeta{WaitMS: waited}, &stateLockTimeoutError{WaitMS: waited}
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return stateLockMeta{WaitMS: waited}, err
		}
		waited = int(time.Since(start).Milliseconds())
		break
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck

	return stateLockMeta{WaitMS: waited}, fn()
}

func withExclusiveFileLock(lockPath string, timeoutMS int, fn func() error) error {
	return withFileLock(lockPath, timeoutMS, syscall.LOCK_EX, "event lock", fn)
}

func withSharedFileLock(lockPath string, timeoutMS int, fn func() error) error {
	return withFileLock(lockPath, timeoutMS, syscall.LOCK_SH, "event read lock", fn)
}

func withFileLock(lockPath string, timeoutMS int, lockMode int, timeoutLabel string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if timeoutMS <= 0 {
		timeoutMS = defaultEventLockTimeoutMS
	}

	start := time.Now()
	for {
		if err := syscall.Flock(int(lockFile.Fd()), lockMode|syscall.LOCK_NB); err != nil {
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				waited := int(time.Since(start).Milliseconds())
				if waited >= timeoutMS {
					return fmt.Errorf("%s timeout after %dms", timeoutLabel, waited)
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return err
		}
		break
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) //nolint:errcheck

	return fn()
}

func appendSessionEvent(projectRoot, session string, event sessionEvent) error {
	path := sessionEventsFile(projectRoot, session)
	countPath := sessionEventCountFile(path)
	lockTimeout := getIntEnv("LISA_EVENT_LOCK_TIMEOUT_MS", defaultEventLockTimeoutMS)
	lockPath := path + ".lock"
	maxBytes := getIntEnv("LISA_EVENTS_MAX_BYTES", defaultEventsMaxBytes)
	if maxBytes <= 0 {
		maxBytes = defaultEventsMaxBytes
	}
	maxLines := getIntEnv("LISA_EVENTS_MAX_LINES", defaultEventsMaxLines)
	if maxLines <= 0 {
		maxLines = defaultEventsMaxLines
	}

	return withExclusiveFileLock(lockPath, lockTimeout, func() error {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}

		data, err := json.Marshal(event)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		lineCount, countKnown := readSessionEventCount(countPath)
		if countKnown {
			lineCount++
		}
		if !countKnown {
			bootCount, err := countSessionEventLines(path)
			if err == nil {
				lineCount = bootCount
				countKnown = true
			}
		}

		needsTrim := info.Size() > int64(maxBytes)
		if !needsTrim && countKnown && lineCount > maxLines {
			needsTrim = true
		}

		if needsTrim {
			keptLines, err := trimSessionEventFileAndCount(path)
			if err != nil {
				return err
			}
			return writeSessionEventCount(countPath, keptLines)
		}

		if countKnown {
			return writeSessionEventCount(countPath, lineCount)
		}
		return nil
	})
}

func trimSessionEventFile(path string) error {
	_, err := trimSessionEventFileAndCount(path)
	return err
}

func trimSessionEventFileAndCount(path string) (int, error) {
	maxBytes := getIntEnv("LISA_EVENTS_MAX_BYTES", defaultEventsMaxBytes)
	if maxBytes <= 0 {
		maxBytes = defaultEventsMaxBytes
	}

	raw, err := readTrimCandidateBytes(path, maxBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	maxLines := getIntEnv("LISA_EVENTS_MAX_LINES", defaultEventsMaxLines)
	if maxLines <= 0 {
		maxLines = defaultEventsMaxLines
	}

	lines := make([]string, 0, maxLines)
	for _, line := range trimLines(string(raw)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	lines = tailLines(lines, maxLines)

	for len(lines) > 0 {
		data := strings.Join(lines, "\n") + "\n"
		if len(data) <= maxBytes {
			if err := writeFileAtomic(path, []byte(data)); err != nil {
				return 0, err
			}
			return len(lines), nil
		}
		lines = lines[1:]
	}

	if err := writeFileAtomic(path, []byte{}); err != nil {
		return 0, err
	}
	return 0, nil
}

func readTrimCandidateBytes(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size <= 0 {
		return []byte{}, nil
	}

	window := int64(maxBytes * 2)
	if window < int64(maxBytes) {
		window = int64(maxBytes)
	}
	if window < 64*1024 {
		window = 64 * 1024
	}
	if window > size {
		window = size
	}
	start := size - window
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}

	raw, err := io.ReadAll(io.LimitReader(f, window))
	if err != nil {
		return nil, err
	}
	if start > 0 {
		if idx := bytes.IndexByte(raw, '\n'); idx >= 0 {
			raw = raw[idx+1:]
		} else {
			raw = []byte{}
		}
	}
	return raw, nil
}

func readSessionEventTail(projectRoot, session string, max int) (sessionEventTail, error) {
	if max <= 0 {
		max = 1
	}
	path := sessionEventsFile(projectRoot, session)
	lockTimeout := getIntEnv("LISA_EVENT_LOCK_TIMEOUT_MS", defaultEventLockTimeoutMS)
	lockPath := path + ".lock"

	var tail sessionEventTail
	err := withSharedFileLock(lockPath, lockTimeout, func() error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		r := bufio.NewReaderSize(f, 64*1024)
		ring := make([]string, max)
		readLines := 0
		for {
			line, readErr := r.ReadString('\n')
			if line == "" && errors.Is(readErr, io.EOF) {
				break
			}
			line = strings.TrimRight(line, "\r\n")
			ring[readLines%max] = line
			readLines++
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return readErr
			}
		}

		kept := readLines
		if kept > max {
			kept = max
		}
		lines := make([]string, 0, kept)
		start := 0
		if readLines > max {
			start = readLines % max
		}
		for i := 0; i < kept; i++ {
			lines = append(lines, ring[(start+i)%max])
		}

		events := make([]sessionEvent, 0, len(lines))
		dropped := 0
		for _, line := range lines {
			if line == "" {
				continue
			}
			var event sessionEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				dropped++
				continue
			}
			events = append(events, event)
		}
		tail = sessionEventTail{Events: events, DroppedLines: dropped, NextCursor: readLines}
		return nil
	})
	if err != nil {
		return sessionEventTail{}, err
	}
	return tail, nil
}

func sessionEventCountFile(path string) string {
	return path + ".lines"
}

func readSessionEventCount(path string) (int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func writeSessionEventCount(path string, count int) error {
	if count < 0 {
		count = 0
	}
	return writeFileAtomic(path, []byte(strconv.Itoa(count)))
}

func countSessionEventLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func appendLifecycleEvent(projectRoot, session, eventType, state, status, reason string) error {
	if strings.TrimSpace(projectRoot) == "" || strings.TrimSpace(session) == "" {
		return nil
	}
	return appendSessionEventFn(projectRoot, session, sessionEvent{
		At:      nowFn().UTC().Format(time.RFC3339Nano),
		Type:    eventType,
		Session: session,
		State:   state,
		Status:  status,
		Reason:  reason,
		Poll:    0,
		Signals: statusSignals{},
	})
}

func pruneStaleSessionEventArtifacts() error {
	retentionDays := getIntEnv("LISA_EVENT_RETENTION_DAYS", defaultEventRetentionDays)
	if retentionDays <= 0 {
		return nil
	}
	cutoff := nowFn().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	paths, err := filepath.Glob("/tmp/.lisa-*-session-*-events.jsonl")
	if err != nil {
		return err
	}

	var errs []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, err.Error())
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		for _, target := range []string{path, sessionEventCountFile(path), path + ".lock"} {
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
