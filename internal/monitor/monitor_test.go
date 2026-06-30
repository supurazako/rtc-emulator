package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/supurazako/rtc-emulator/internal/lab"
)

func TestBuildSnapshotDerivesQualityMetrics(t *testing.T) {
	startedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	now := startedAt.Add(3 * time.Second)
	info := lab.ScenarioRunInfo{
		RunID:     "run-1",
		Scenario:  lab.ScenarioWebRTCUplinkCongestion,
		Node:      "node1",
		Peer:      "node2",
		StartedAt: startedAt,
	}
	events := []lab.EventRecord{
		{
			Time:   startedAt.Add(time.Second).Format(time.RFC3339Nano),
			Phase:  "impaired",
			Action: "apply",
			Condition: lab.ImpairmentCondition{
				BW: "1mbit",
			},
			Status: "ok",
		},
	}
	stats := []lab.WebRTCStatsRecord{
		{
			Time:                startedAt.Format(time.RFC3339Nano),
			Node:                "node1",
			Peer:                "node2",
			PeerConnectionState: "connected",
			ICEConnectionState:  "connected",
			BytesSent:           uint64Ptr(100_000),
			RoundTripTime:       float64Ptr(0.040),
			Jitter:              float64Ptr(0.005),
			PacketsLost:         int64Ptr(1),
		},
		{
			Time:                startedAt.Add(time.Second).Format(time.RFC3339Nano),
			Node:                "node1",
			Peer:                "node2",
			PeerConnectionState: "connected",
			ICEConnectionState:  "connected",
			BytesSent:           uint64Ptr(300_000),
			RoundTripTime:       float64Ptr(0.086),
			Jitter:              float64Ptr(0.012),
			PacketsLost:         int64Ptr(4),
		},
	}

	got := BuildSnapshot(info, events, stats, now)
	if got.Phase != "impaired" {
		t.Fatalf("phase = %s, want impaired", got.Phase)
	}
	if got.LastEvent != "apply bw=1mbit" {
		t.Fatalf("last event = %q, want apply bw=1mbit", got.LastEvent)
	}
	if got.PeerConnectionState != "connected" || got.ICEConnectionState != "connected" {
		t.Fatalf("unexpected connection state: pc=%s ice=%s", got.PeerConnectionState, got.ICEConnectionState)
	}
	if got.Bitrate.CurrentText != "1.60 Mbps" {
		t.Fatalf("bitrate = %s, want 1.60 Mbps", got.Bitrate.CurrentText)
	}
	if got.RTT.CurrentText != "86 ms" {
		t.Fatalf("RTT = %s, want 86 ms", got.RTT.CurrentText)
	}
	if got.Jitter.CurrentText != "12 ms" {
		t.Fatalf("jitter = %s, want 12 ms", got.Jitter.CurrentText)
	}
	if got.PacketLoss.CurrentText != "+3 / 4" {
		t.Fatalf("packet loss = %s, want +3 / 4", got.PacketLoss.CurrentText)
	}
	if len(got.PacketLoss.Values) != 1 || got.PacketLoss.Values[0] != 3 {
		t.Fatalf("packet loss chart values = %v, want [3]", got.PacketLoss.Values)
	}
}

func TestRenderChartHandlesEmptyValues(t *testing.T) {
	got := renderChart("RTT", "collecting", nil, 40, 4)
	if got == "" {
		t.Fatalf("expected chart output")
	}
}

func TestReadStatsFromOnlyReturnsAppendedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.node1.jsonl")
	first := `{"run_id":"run-1","time":"2026-06-30T12:00:00Z","node":"node1"}`
	second := `{"run_id":"run-1","time":"2026-06-30T12:00:01Z","node":"node1"}`
	if err := os.WriteFile(path, []byte(first+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write stats: %v", err)
	}
	offsets := make(map[string]int64)

	got, err := readStatsFrom([]string{path}, offsets)
	if err != nil {
		t.Fatalf("unexpected first read error: %v", err)
	}
	if len(got) != 1 || got[0].Time != "2026-06-30T12:00:00Z" {
		t.Fatalf("first read = %+v, want first record", got)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("failed to reopen stats: %v", err)
	}
	if _, err := file.WriteString(second + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("failed to append stats: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close stats: %v", err)
	}

	got, err = readStatsFrom([]string{path}, offsets)
	if err != nil {
		t.Fatalf("unexpected second read error: %v", err)
	}
	if len(got) != 1 || got[0].Time != "2026-06-30T12:00:01Z" {
		t.Fatalf("second read = %+v, want appended record only", got)
	}
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func float64Ptr(v float64) *float64 {
	return &v
}
