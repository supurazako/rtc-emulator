package lab

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebRTCPeerNetNSArgs(t *testing.T) {
	args := webRTCPeerNetNSArgs("node1", "/tmp/rtc-emulator", WebRTCPeerOptions{
		Role:          "offerer",
		RunID:         "run-1",
		RunDir:        "runs/run-1",
		Node:          "node1",
		Peer:          "node2",
		Duration:      3 * time.Second,
		StatsInterval: 500 * time.Millisecond,
	})

	want := []string{
		"netns", "exec", "node1",
		"/tmp/rtc-emulator",
		"lab", "webrtc", "peer",
		"--role", "offerer",
		"--run-id", "run-1",
		"--run-dir", "runs/run-1",
		"--node", "node1",
		"--peer", "node2",
		"--duration", "3s",
		"--stats-interval", "500ms",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestRunWebRTCP2PWithDepsWritesConnectedEventAndMergedStats(t *testing.T) {
	runsDir := t.TempDir()
	runID := "run-1"
	runDir := filepath.Join(runsDir, runID)
	var mu sync.Mutex
	runCommands := 0
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	deps := webRTCP2PDeps{
		createDeps: createDeps{
			exec: &fakeExecutor{
				outputFn: func(name string, args ...string) (string, error) {
					if callKey(name, args...) == "ip netns list" {
						return "node1\nnode2\n", nil
					}
					return "", nil
				},
			},
			goos:     "linux",
			isRoot:   func() bool { return true },
			findPath: func(string) (string, error) { return "/sbin/ip", nil },
			loadState: func(context.Context) (*LabState, error) {
				return &LabState{Nodes: []string{"node1", "node2"}}, nil
			},
		},
		now:      func() time.Time { now = now.Add(time.Second); return now },
		newRunID: func(time.Time) (string, error) { return runID, nil },
		mkdirAll: os.MkdirAll,
		openFile: func(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return os.OpenFile(path, flag, perm)
		},
		executable: func() (string, error) { return "/tmp/rtc-emulator", nil },
		runCommand: func(ctx context.Context, name string, args []string, stdout io.Writer, stderr io.Writer) error {
			if name != "ip" {
				return nil
			}
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
			statsTime := "2026-01-01T00:00:02Z"
			if node == "node2" {
				statsTime = "2026-01-01T00:00:01Z"
			}
			if err := writeOneStatsRecord(filepath.Join(runDir, peerStatsFilename(node)), WebRTCStatsRecord{
				RunID:               runID,
				Time:                statsTime,
				Node:                node,
				Peer:                peer,
				PeerConnectionState: "connected",
				ICEConnectionState:  "connected",
			}); err != nil {
				return err
			}
			mu.Lock()
			runCommands++
			mu.Unlock()
			return nil
		},
	}

	result, err := runWebRTCP2PWithDeps(t.Context(), WebRTCP2POptions{
		RunsDir:       runsDir,
		NodeA:         "node1",
		NodeB:         "node2",
		Duration:      time.Second,
		StatsInterval: time.Second,
	}, deps)
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.LatestDir != filepath.Join(runsDir, latestRunSymlinkName) {
		t.Fatalf("LatestDir = %s, want %s", result.LatestDir, filepath.Join(runsDir, latestRunSymlinkName))
	}
	latestTarget, err := os.Readlink(result.LatestDir)
	if err != nil {
		t.Fatalf("failed to read latest link: %v", err)
	}
	if latestTarget != runID {
		t.Fatalf("latest link target = %s, want %s", latestTarget, runID)
	}

	mu.Lock()
	gotRunCommands := runCommands
	mu.Unlock()
	if gotRunCommands != 2 {
		t.Fatalf("runCommand calls = %d, want 2", gotRunCommands)
	}

	events := readEventRecords(t, result.EventsPath)
	var phases []string
	for _, event := range events {
		phases = append(phases, event.Phase+" "+event.Status)
	}
	wantPhases := []string{
		"webrtc_start ok",
		"connected ok",
		"stats_complete ok",
		"cleanup ok",
	}
	if !reflect.DeepEqual(phases, wantPhases) {
		t.Fatalf("event phases = %#v, want %#v", phases, wantPhases)
	}

	stats := readStatsRecords(t, result.StatsPath)
	if len(stats) != 2 {
		t.Fatalf("merged stats count = %d, want 2", len(stats))
	}
	if stats[0].Node != "node2" || stats[1].Node != "node1" {
		t.Fatalf("merged stats order = %s, %s; want node2, node1", stats[0].Node, stats[1].Node)
	}
}

func TestValidateWebRTCP2POptionsRejectsSameNodeBeforeHostChecks(t *testing.T) {
	err := validateWebRTCP2POptions(t.Context(), WebRTCP2POptions{
		NodeA:         "node1",
		NodeB:         "node1",
		Duration:      time.Second,
		StatsInterval: time.Second,
	}, createDeps{})
	if err == nil || !strings.Contains(err.Error(), "node-a and node-b must be different") {
		t.Fatalf("expected same-node validation error, got %v", err)
	}
}

func argValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func writeOneStatsRecord(path string, record WebRTCStatsRecord) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func readEventRecords(t *testing.T, path string) []EventRecord {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read events %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	records := make([]EventRecord, 0, len(lines))
	for _, line := range lines {
		var record EventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to parse event line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func readStatsRecords(t *testing.T, path string) []WebRTCStatsRecord {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read stats %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	records := make([]WebRTCStatsRecord, 0, len(lines))
	for _, line := range lines {
		var record WebRTCStatsRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to parse stats line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func TestValidateWebRTCPeerOptions(t *testing.T) {
	valid := WebRTCPeerOptions{
		Role:          "offerer",
		RunID:         "run-1",
		RunDir:        "runs/run-1",
		Node:          "node1",
		Peer:          "node2",
		Duration:      time.Second,
		StatsInterval: time.Second,
	}
	if err := validateWebRTCPeerOptions(valid); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	invalid := valid
	invalid.Role = "controller"
	err := validateWebRTCPeerOptions(invalid)
	if err == nil || !strings.Contains(err.Error(), "unsupported webrtc peer role") {
		t.Fatalf("expected unsupported role error, got %v", err)
	}

	invalid = valid
	invalid.RunDir = ""
	err = validateWebRTCPeerOptions(invalid)
	if err == nil || !strings.Contains(err.Error(), "run dir is required") {
		t.Fatalf("expected missing run dir error, got %v", err)
	}
}
