package lab

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunScenarioWithDeps_WritesJSONLAndPhaseOrder(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	runsDir := filepath.Join(t.TempDir(), "runs")

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validImpairmentTestDeps(ex),
		fixedScenarioRunDeps("run-test"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RunID != "run-test" {
		t.Fatalf("unexpected run id: %s", got.RunID)
	}
	if got.EventsPath != filepath.Join(runsDir, "run-test", "events.jsonl") {
		t.Fatalf("unexpected events path: %s", got.EventsPath)
	}

	events := readScenarioEvents(t, got.EventsPath)
	assertPhases(t, events, []string{"baseline", "impaired", "recovery", "cleanup"})
	for _, event := range events {
		if event.RunID != "run-test" {
			t.Fatalf("unexpected run id in event: %+v", event)
		}
		if event.Scenario != ScenarioWebRTCUplinkCongestion {
			t.Fatalf("unexpected scenario in event: %+v", event)
		}
		if event.Condition.BW != defaultUplinkBW {
			t.Fatalf("expected default bw %s, got %+v", defaultUplinkBW, event.Condition)
		}
	}
	if !hasCall(ex.calls, "ip netns exec node1 tc qdisc replace dev eth0 root netem rate 1mbit") {
		t.Fatalf("missing apply command, calls=%v", ex.calls)
	}
	if countCall(ex.calls, "ip netns exec node1 tc qdisc del dev eth0 root") != 2 {
		t.Fatalf("expected recovery and cleanup clear calls, calls=%v", ex.calls)
	}
}

func TestRunScenarioWithDeps_ApplyFailureStillLogsCleanup(t *testing.T) {
	ex := scenarioTestExecutor(func(name string, args ...string) error {
		if callKey(name, args...) == "ip netns exec node1 tc qdisc replace dev eth0 root netem rate 1mbit" {
			return errors.New("apply failed")
		}
		return nil
	})
	runsDir := filepath.Join(t.TempDir(), "runs")

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validImpairmentTestDeps(ex),
		fixedScenarioRunDeps("run-apply-failure"),
	)
	if err == nil || !strings.Contains(err.Error(), "impaired phase failed") {
		t.Fatalf("expected impaired phase error, got: %v", err)
	}

	events := readScenarioEvents(t, got.EventsPath)
	assertPhases(t, events, []string{"baseline", "impaired", "recovery", "cleanup"})
	if events[1].Status != "error" || !strings.Contains(events[1].Error, "apply failed") {
		t.Fatalf("expected impaired error event, got: %+v", events[1])
	}
	if events[2].Status != "skipped" {
		t.Fatalf("expected skipped recovery after apply failure, got: %+v", events[2])
	}
	if events[3].Phase != "cleanup" || events[3].Status == "skipped" {
		t.Fatalf("expected cleanup result, got: %+v", events[3])
	}
	if countCall(ex.calls, "ip netns exec node1 tc qdisc del dev eth0 root") != 1 {
		t.Fatalf("expected cleanup clear call, calls=%v", ex.calls)
	}
}

func TestRunScenarioWithDeps_ClearFailureLoggedAsError(t *testing.T) {
	ex := scenarioTestExecutor(func(name string, args ...string) error {
		if callKey(name, args...) == "ip netns exec node1 tc qdisc del dev eth0 root" {
			return errors.New("clear failed")
		}
		return nil
	})
	runsDir := filepath.Join(t.TempDir(), "runs")

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validImpairmentTestDeps(ex),
		fixedScenarioRunDeps("run-clear-failure"),
	)
	if err == nil || !strings.Contains(err.Error(), "recovery phase failed") || !strings.Contains(err.Error(), "cleanup phase failed") {
		t.Fatalf("expected recovery and cleanup errors, got: %v", err)
	}

	events := readScenarioEvents(t, got.EventsPath)
	assertPhases(t, events, []string{"baseline", "impaired", "recovery", "cleanup"})
	if events[2].Status != "error" || !strings.Contains(events[2].Error, "clear failed") {
		t.Fatalf("expected recovery error event, got: %+v", events[2])
	}
	if events[3].Status != "error" || !strings.Contains(events[3].Error, "clear failed") {
		t.Fatalf("expected cleanup error event, got: %+v", events[3])
	}
}

func TestRunScenarioWithDeps_LogWriteFailureReturnsErrorAndCleansUp(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	writer := &failingWriteCloser{failOn: 2}

	_, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: "runs"},
		validImpairmentTestDeps(ex),
		scenarioRunDeps{
			now: func() time.Time {
				return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
			},
			newRunID: func(time.Time) (string, error) {
				return "run-write-failure", nil
			},
			mkdirAll: func(string, os.FileMode) error {
				return nil
			},
			openFile: func(string, int, os.FileMode) (io.WriteCloser, error) {
				return writer, nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "failed to write event log") {
		t.Fatalf("expected log write error, got: %v", err)
	}
	if !writer.closed {
		t.Fatalf("expected writer to be closed")
	}
	if countCall(ex.calls, "ip netns exec node1 tc qdisc del dev eth0 root") != 2 {
		t.Fatalf("expected recovery and cleanup despite log write failure, calls=%v", ex.calls)
	}
}

func scenarioTestExecutor(runFn func(name string, args ...string) error) *fakeExecutor {
	return &fakeExecutor{
		runFn: runFn,
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node1\n", nil
			}
			return "", nil
		},
	}
}

func fixedScenarioRunDeps(runID string) scenarioRunDeps {
	return scenarioRunDeps{
		now: func() time.Time {
			return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
		},
		newRunID: func(time.Time) (string, error) {
			return runID, nil
		},
	}
}

func readScenarioEvents(t *testing.T, path string) []EventRecord {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	rawLines := strings.Split(strings.TrimSpace(string(b)), "\n")
	events := make([]EventRecord, 0, len(rawLines))
	for _, line := range rawLines {
		var event EventRecord
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to parse event line %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func assertPhases(t *testing.T, events []EventRecord, want []string) {
	t.Helper()

	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d: %+v", len(want), len(events), events)
	}
	for i, phase := range want {
		if events[i].Phase != phase {
			t.Fatalf("event %d phase: want %s, got %+v", i, phase, events[i])
		}
	}
}

func countCall(calls []string, want string) int {
	count := 0
	for _, c := range calls {
		if c == want {
			count++
		}
	}
	return count
}

type failingWriteCloser struct {
	failOn int
	writes int
	closed bool
}

func (w *failingWriteCloser) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failOn {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func (w *failingWriteCloser) Close() error {
	w.closed = true
	return nil
}
