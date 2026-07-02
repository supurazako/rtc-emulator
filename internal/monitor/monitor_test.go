package monitor

import (
	"os"
	"path/filepath"
	"strings"
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
	if got.Bitrate.MaxText != "max 1.60 Mbps" {
		t.Fatalf("bitrate max = %s, want max 1.60 Mbps", got.Bitrate.MaxText)
	}
	if got.RTT.CurrentText != "86 ms" {
		t.Fatalf("RTT = %s, want 86 ms", got.RTT.CurrentText)
	}
	if got.RTT.MaxText != "max 86 ms" {
		t.Fatalf("RTT max = %s, want max 86 ms", got.RTT.MaxText)
	}
	if got.Jitter.CurrentText != "12 ms" {
		t.Fatalf("jitter = %s, want 12 ms", got.Jitter.CurrentText)
	}
	if got.Jitter.MaxText != "max 12 ms" {
		t.Fatalf("jitter max = %s, want max 12 ms", got.Jitter.MaxText)
	}
	if got.PacketLoss.CurrentText != "+3 / 4" {
		t.Fatalf("packet loss = %s, want +3 / 4", got.PacketLoss.CurrentText)
	}
	if got.PacketLoss.MaxText != "max 3" {
		t.Fatalf("packet loss max = %s, want max 3", got.PacketLoss.MaxText)
	}
	if len(got.PacketLoss.Points) != 1 || got.PacketLoss.Points[0].Value != 3 {
		t.Fatalf("packet loss chart points = %v, want value [3]", got.PacketLoss.Points)
	}
	if got.PacketLoss.Points[0].Time != startedAt.Add(time.Second) {
		t.Fatalf("packet loss chart time = %v, want %v", got.PacketLoss.Points[0].Time, startedAt.Add(time.Second))
	}
}

func TestBuildSnapshotShowsNoRTPForRTPOnlyMetricsOnDataChannelStats(t *testing.T) {
	startedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	now := startedAt.Add(3 * time.Second)
	info := lab.ScenarioRunInfo{
		RunID:     "run-1",
		Scenario:  lab.ScenarioWebRTCUplinkCongestion,
		Node:      "node1",
		Peer:      "node2",
		StartedAt: startedAt,
	}
	stats := []lab.WebRTCStatsRecord{
		{
			Time:                 startedAt.Format(time.RFC3339Nano),
			Node:                 "node1",
			Peer:                 "node2",
			PeerConnectionState:  "connected",
			ICEConnectionState:   "connected",
			BytesSent:            uint64Ptr(100_000),
			DataMessagesSent:     uint64Ptr(10),
			DataMessagesReceived: uint64Ptr(8),
		},
	}

	got := BuildSnapshot(info, nil, stats, now)
	if got.Jitter.CurrentText != "no RTP" {
		t.Fatalf("jitter = %s, want no RTP", got.Jitter.CurrentText)
	}
	if got.PacketLoss.CurrentText != "no RTP" {
		t.Fatalf("packet loss = %s, want no RTP", got.PacketLoss.CurrentText)
	}
}

func TestRenderChartHandlesEmptyValues(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	got := renderChart(Metric{Name: "RTT", CurrentText: "collecting"}, now.Add(-historyWindow), now, now.Add(-10*time.Second), 40, 4)
	if got == "" {
		t.Fatalf("expected chart output")
	}
	if !contains(got, "collecting...") {
		t.Fatalf("chart = %q, want collecting placeholder", got)
	}
}

func TestRenderChartHandlesLineValues(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	tests := map[string][]float64{
		"increasing": {1, 2, 3, 4, 5},
		"decreasing": {5, 4, 3, 2, 1},
		"flat":       {3, 3, 3, 3, 3},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			got := renderChart(Metric{Name: "RTT", CurrentText: "5 ms", MaxText: "max 5 ms", Points: metricPoints(now.Add(-4*time.Second), values...)}, now.Add(-historyWindow), now, now.Add(-10*time.Second), 40, 4)
			if got == "" {
				t.Fatalf("expected chart output")
			}
			if !contains(got, "RTT") {
				t.Fatalf("chart = %q, want title", got)
			}
		})
	}
}

func TestPaddedRangeAddsVisualHeadroom(t *testing.T) {
	tests := []struct {
		name string
		min  float64
		max  float64
	}{
		{name: "increasing", min: 1, max: 5},
		{name: "flat", min: 3, max: 3},
		{name: "near zero", min: 0, max: 0.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax := paddedRange(tt.min, tt.max)
			if !(gotMin < tt.min) {
				t.Fatalf("min = %v, want below %v", gotMin, tt.min)
			}
			if !(gotMax > tt.max) {
				t.Fatalf("max = %v, want above %v", gotMax, tt.max)
			}
		})
	}
}

func TestChartFrameLinesMatchRequestedWidth(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	for _, line := range []string{
		chartHeader(Metric{Name: "RTT", CurrentText: "5 ms", MaxText: "max 5 ms"}, 40),
		chartFooter(40, now.Add(-historyWindow), now, now.Add(-10*time.Second)),
	} {
		if got := runeLen(line); got != 40 {
			t.Fatalf("line width = %d, want 40: %q", got, line)
		}
	}
}

func TestChartFooterMarksRunStartWithinFirstMinute(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	got := chartFooter(40, now.Add(-historyWindow), now, now.Add(-10*time.Second))
	if !contains(got, "start") {
		t.Fatalf("footer = %q, want start marker", got)
	}
	if !strings.HasSuffix(got, "now ┘") {
		t.Fatalf("footer = %q, want now suffix", got)
	}
}

func TestChartPointUsesRollingTimeWindow(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	windowStart := now.Add(-historyWindow)
	point := chartPoint(MetricPoint{
		Time:  now.Add(-10 * time.Second),
		Value: 42,
	}, windowStart, historyWindow.Seconds())
	if point.X != 50 {
		t.Fatalf("x = %v, want 50 seconds into rolling window", point.X)
	}
	if point.Y != 42 {
		t.Fatalf("y = %v, want 42", point.Y)
	}
}

func TestRenderChartHandlesSmallDimensions(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 10, 0, time.UTC)
	got := renderChart(Metric{Name: "RTT", CurrentText: "5 ms", Points: metricPoints(now.Add(-time.Second), 1, 2)}, now.Add(-historyWindow), now, now.Add(-10*time.Second), 8, 1)
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

func metricPoints(start time.Time, values ...float64) []MetricPoint {
	points := make([]MetricPoint, 0, len(values))
	for i, value := range values {
		points = append(points, MetricPoint{
			Time:  start.Add(time.Duration(i) * time.Second),
			Value: value,
		})
	}
	return points
}

func contains(value string, substr string) bool {
	return strings.Contains(value, substr)
}
