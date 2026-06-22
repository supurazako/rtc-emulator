package lab

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type EventRecord struct {
	RunID     string              `json:"run_id"`
	Event     string              `json:"event"`
	Scenario  string              `json:"scenario"`
	Phase     string              `json:"phase"`
	Time      string              `json:"time"`
	Node      string              `json:"node"`
	Interface string              `json:"interface"`
	Action    string              `json:"action"`
	Condition ImpairmentCondition `json:"condition"`
	Status    string              `json:"status"`
	Error     string              `json:"error"`
}

type ImpairmentCondition struct {
	Delay  string `json:"delay"`
	Loss   string `json:"loss"`
	Jitter string `json:"jitter"`
	BW     string `json:"bw"`
}

type eventLogger struct {
	runID      string
	runDir     string
	eventsPath string
	writer     io.WriteCloser
}

func generateRunID(t time.Time) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return t.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix[:]), nil
}

func newEventLogger(runsDir string, runID string, deps scenarioRunDeps) (*eventLogger, error) {
	runDir := filepath.Join(runsDir, runID)
	if err := deps.mkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create run directory %s: %w", runDir, err)
	}

	eventsPath := filepath.Join(runDir, "events.jsonl")
	writer, err := deps.openFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open event log %s: %w", eventsPath, err)
	}

	return &eventLogger{
		runID:      runID,
		runDir:     runDir,
		eventsPath: eventsPath,
		writer:     writer,
	}, nil
}

func (l *eventLogger) write(record EventRecord) error {
	b, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode event record: %w", err)
	}
	if _, err := l.writer.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("failed to write event log %s: %w", l.eventsPath, err)
	}
	return nil
}

func (l *eventLogger) close() error {
	if l.writer == nil {
		return nil
	}
	if err := l.writer.Close(); err != nil {
		return fmt.Errorf("failed to close event log %s: %w", l.eventsPath, err)
	}
	l.writer = nil
	return nil
}
