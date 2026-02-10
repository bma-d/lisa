package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ensureHeartbeatWritableFn = ensureHeartbeatWritable
var withStateFileLockFn = withStateFileLock
var appendSessionEventFn = appendSessionEvent
var readSessionEventTailFn = readSessionEventTail

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

func appendSessionEvent(projectRoot, session string, event sessionEvent) error {
	path := sessionEventsFile(projectRoot, session)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := trimSessionEventFile(path); err != nil {
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
	return trimSessionEventFile(path)
}

func trimSessionEventFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	maxBytes := getIntEnv("LISA_EVENTS_MAX_BYTES", defaultEventsMaxBytes)
	if maxBytes <= 0 || info.Size() <= int64(maxBytes) {
		return nil
	}

	maxLines := getIntEnv("LISA_EVENTS_MAX_LINES", defaultEventsMaxLines)
	if maxLines <= 0 {
		maxLines = defaultEventsMaxLines
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := trimLines(string(raw))
	lines = tailLines(lines, maxLines)
	for len(lines) > 1 {
		data := strings.Join(lines, "\n") + "\n"
		if len(data) <= maxBytes {
			return writeFileAtomic(path, []byte(data))
		}
		lines = lines[1:]
	}
	if len(lines) == 1 {
		data := strings.TrimSpace(lines[0])
		if len(data)+1 > maxBytes {
			// Drop pathological oversized line instead of writing a partial JSON fragment.
			return writeFileAtomic(path, []byte{})
		}
		return writeFileAtomic(path, []byte(data+"\n"))
	}
	return writeFileAtomic(path, []byte{})
}

func readSessionEventTail(projectRoot, session string, max int) (sessionEventTail, error) {
	path := sessionEventsFile(projectRoot, session)
	f, err := os.Open(path)
	if err != nil {
		return sessionEventTail{}, err
	}
	defer f.Close()

	lines := make([]string, 0, max)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return sessionEventTail{}, err
	}
	lines = tailLines(lines, max)

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
	return sessionEventTail{Events: events, DroppedLines: dropped}, nil
}
