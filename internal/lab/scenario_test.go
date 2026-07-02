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
		validScenarioTestDeps(ex),
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
	if got.StatsPath != filepath.Join(runsDir, "run-test", "stats.jsonl") {
		t.Fatalf("unexpected stats path: %s", got.StatsPath)
	}
	latestTarget, err := os.Readlink(got.LatestDir)
	if err != nil {
		t.Fatalf("failed to read latest run link: %v", err)
	}
	if latestTarget != "run-test" {
		t.Fatalf("latest run link = %s, want run-test", latestTarget)
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
	stats := readStatsRecords(t, got.StatsPath)
	if len(stats) != 2 {
		t.Fatalf("expected merged stats from two peers, got %+v", stats)
	}
	if stats[0].Node != "node2" || stats[1].Node != "node1" {
		t.Fatalf("unexpected stats order: %+v", stats)
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
		validScenarioTestDeps(ex),
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

func TestRunScenarioWithDeps_ApplyFailureStopsPeersBeforeWait(t *testing.T) {
	ex := scenarioTestExecutor(func(name string, args ...string) error {
		if callKey(name, args...) == "ip netns exec node1 tc qdisc replace dev eth0 root netem rate 1mbit" {
			return errors.New("apply failed")
		}
		return nil
	})
	runsDir := filepath.Join(t.TempDir(), "runs")
	peerCanceled := make(chan string, 2)

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validScenarioTestDeps(ex),
		scenarioRunDeps{
			now: func() time.Time {
				return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
			},
			newRunID: func(time.Time) (string, error) {
				return "run-apply-failure-cancel", nil
			},
			executable: func() (string, error) {
				return "/tmp/rtc-emulator", nil
			},
			runCommand: func(ctx context.Context, name string, args []string, _ io.Writer, _ io.Writer) error {
				if name != "ip" {
					return nil
				}
				runID := argValue(args, "--run-id")
				runDir := argValue(args, "--run-dir")
				node := argValue(args, "--node")
				peer := argValue(args, "--peer")
				state := &webRTCPeerRuntimeState{
					peerConnection: "connected",
					iceConnection:  "connected",
				}
				if err := writePeerConnectedMarker(runDir, WebRTCPeerOptions{
					RunID: runID,
					Node:  node,
					Peer:  peer,
				}, state); err != nil {
					return err
				}
				if err := writeOneStatsRecord(filepath.Join(runDir, peerStatsFilename(node)), WebRTCStatsRecord{
					RunID:               runID,
					Time:                time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
					Node:                node,
					Peer:                peer,
					PeerConnectionState: "connected",
					ICEConnectionState:  "connected",
				}); err != nil {
					return err
				}
				<-ctx.Done()
				peerCanceled <- node
				return ctx.Err()
			},
			sleep: func(context.Context, time.Duration) error {
				return nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "impaired phase failed") {
		t.Fatalf("expected impaired phase error, got: %v", err)
	}
	if strings.Contains(err.Error(), "webrtc peer") {
		t.Fatalf("expected intentional peer cancellation to be suppressed, got: %v", err)
	}
	if got == nil {
		t.Fatalf("expected partial result")
	}
	for i := 0; i < 2; i++ {
		select {
		case <-peerCanceled:
		default:
			t.Fatalf("expected both peer processes to be canceled before scenario returns")
		}
	}
}

func TestRunScenarioWithDeps_ExecutableFailureDoesNotAddMissingStatsError(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	runsDir := filepath.Join(t.TempDir(), "runs")

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validScenarioTestDeps(ex),
		scenarioRunDeps{
			now: func() time.Time {
				return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
			},
			newRunID: func(time.Time) (string, error) {
				return "run-executable-failure", nil
			},
			executable: func() (string, error) {
				return "", errors.New("no executable")
			},
			runCommand: fakeScenarioWebRTCPeerCommand,
			sleep:      func(context.Context, time.Duration) error { return nil },
		},
	)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve current executable") {
		t.Fatalf("expected executable error, got: %v", err)
	}
	if strings.Contains(err.Error(), "failed to merge peer stats logs") {
		t.Fatalf("expected no missing stats merge error, got: %v", err)
	}
	if got == nil {
		t.Fatalf("expected partial result")
	}
}

func TestRunScenarioWithDeps_RejectsUnsupportedInterface(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	runsDir := filepath.Join(t.TempDir(), "runs")

	got, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{
			Scenario:  ScenarioWebRTCUplinkCongestion,
			RunsDir:   runsDir,
			Interface: "eth1",
		},
		validScenarioTestDeps(ex),
		fixedScenarioRunDeps("run-unsupported-interface"),
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported interface "eth1"`) {
		t.Fatalf("expected unsupported interface error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result, got: %+v", got)
	}
	if len(ex.calls) != 0 {
		t.Fatalf("expected no apply or clear calls, got: %v", ex.calls)
	}
	if _, statErr := os.Stat(filepath.Join(runsDir, "run-unsupported-interface")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no run directory, stat err: %v", statErr)
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
		validScenarioTestDeps(ex),
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

func TestRunScenarioWithDeps_CancelDuringBaselineSkipsApplyAndCleansUp(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	runsDir := filepath.Join(t.TempDir(), "runs")
	ctx, cancel := context.WithCancel(context.Background())

	got, err := runScenarioWithDeps(
		ctx,
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validScenarioTestDeps(ex),
		scenarioRunDeps{
			now: func() time.Time {
				return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
			},
			newRunID: func(time.Time) (string, error) {
				return "run-cancel-baseline", nil
			},
			executable: func() (string, error) {
				return "/tmp/rtc-emulator", nil
			},
			runCommand: fakeScenarioWebRTCPeerCommand,
			sleep: func(context.Context, time.Duration) error {
				cancel()
				return context.Canceled
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "phase wait interrupted") {
		t.Fatalf("expected interrupted wait error, got: %v", err)
	}
	if got == nil {
		t.Fatalf("expected partial result")
	}
	if hasCall(ex.calls, "ip netns exec node1 tc qdisc replace dev eth0 root netem rate 1mbit") {
		t.Fatalf("expected no impairment apply after cancellation, calls=%v", ex.calls)
	}
	if countCall(ex.calls, "ip netns exec node1 tc qdisc del dev eth0 root") != 1 {
		t.Fatalf("expected cleanup clear after cancellation, calls=%v", ex.calls)
	}
}

func TestRunScenarioWithDeps_LogWriteFailureReturnsErrorAndCleansUp(t *testing.T) {
	ex := scenarioTestExecutor(nil)
	writer := &failingWriteCloser{failOn: 2}
	runsDir := filepath.Join(t.TempDir(), "runs")

	_, err := runScenarioWithDeps(
		context.Background(),
		ScenarioRunOptions{Scenario: ScenarioWebRTCUplinkCongestion, RunsDir: runsDir},
		validScenarioTestDeps(ex),
		scenarioRunDeps{
			now: func() time.Time {
				return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
			},
			newRunID: func(time.Time) (string, error) {
				return "run-write-failure", nil
			},
			mkdirAll: os.MkdirAll,
			openFile: func(string, int, os.FileMode) (io.WriteCloser, error) {
				return writer, nil
			},
			executable: func() (string, error) {
				return "/tmp/rtc-emulator", nil
			},
			runCommand: fakeScenarioWebRTCPeerCommand,
			sleep:      func(context.Context, time.Duration) error { return nil },
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
				return "node1\nnode2\n", nil
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
		executable: func() (string, error) {
			return "/tmp/rtc-emulator", nil
		},
		runCommand: fakeScenarioWebRTCPeerCommand,
		sleep:      func(context.Context, time.Duration) error { return nil },
	}
}

func validScenarioTestDeps(ex *fakeExecutor) createDeps {
	return impairmentTestDeps(ex, func(context.Context) (*LabState, error) {
		return &LabState{Nodes: []string{"node1", "node2"}}, nil
	})
}

func fakeScenarioWebRTCPeerCommand(_ context.Context, name string, args []string, _ io.Writer, _ io.Writer) error {
	if name != "ip" {
		return nil
	}
	runID := argValue(args, "--run-id")
	runDir := argValue(args, "--run-dir")
	node := argValue(args, "--node")
	peer := argValue(args, "--peer")
	state := &webRTCPeerRuntimeState{
		peerConnection: "connected",
		iceConnection:  "connected",
	}
	if err := writePeerConnectedMarker(runDir, WebRTCPeerOptions{
		RunID: runID,
		Node:  node,
		Peer:  peer,
	}, state); err != nil {
		return err
	}
	statsTime := "2026-06-21T12:00:02Z"
	if node == "node2" {
		statsTime = "2026-06-21T12:00:01Z"
	}
	return writeOneStatsRecord(filepath.Join(runDir, peerStatsFilename(node)), WebRTCStatsRecord{
		RunID:               runID,
		Time:                statsTime,
		Node:                node,
		Peer:                peer,
		PeerConnectionState: "connected",
		ICEConnectionState:  "connected",
	})
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
