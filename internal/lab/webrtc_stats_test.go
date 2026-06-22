package lab

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nopWriteCloser struct {
	bytes.Buffer
}

func (w *nopWriteCloser) Close() error {
	return nil
}

func TestStatsLoggerWritesJSONL(t *testing.T) {
	writer := &nopWriteCloser{}
	logger, err := newStatsLogger("stats.jsonl", func(string, int, os.FileMode) (io.WriteCloser, error) {
		return writer, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := logger.write(WebRTCStatsRecord{
		RunID:               "run-1",
		Time:                "2026-01-01T00:00:00Z",
		Node:                "node1",
		Peer:                "node2",
		PeerConnectionState: "connected",
		ICEConnectionState:  "connected",
		BytesSent:           uint64Ptr(42),
	}); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}

	got := writer.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected newline-delimited JSON, got %q", got)
	}
	if strings.Contains(got, "bytes_received") {
		t.Fatalf("expected empty optional stats to be omitted, got %s", got)
	}
	var record WebRTCStatsRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &record); err != nil {
		t.Fatalf("failed to parse stats line: %v", err)
	}
	if record.Node != "node1" || record.BytesSent == nil || *record.BytesSent != 42 {
		t.Fatalf("unexpected stats record: %+v", record)
	}
}

func TestMergeStatsLogsSortsByTimestamp(t *testing.T) {
	dir := t.TempDir()
	node1 := filepath.Join(dir, "stats.node1.jsonl")
	node2 := filepath.Join(dir, "stats.node2.jsonl")
	output := filepath.Join(dir, "stats.jsonl")

	writeStatsTestFile(t, node1, []WebRTCStatsRecord{
		{RunID: "run-1", Time: "2026-01-01T00:00:03Z", Node: "node1", Peer: "node2"},
		{RunID: "run-1", Time: "2026-01-01T00:00:01Z", Node: "node1", Peer: "node2"},
	})
	writeStatsTestFile(t, node2, []WebRTCStatsRecord{
		{RunID: "run-1", Time: "2026-01-01T00:00:02Z", Node: "node2", Peer: "node1"},
	})

	if err := mergeStatsLogs(output, []string{node1, node2}); err != nil {
		t.Fatalf("unexpected merge error: %v", err)
	}

	b, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("failed to read merged stats: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 merged records, got %d: %s", len(lines), string(b))
	}
	var got []WebRTCStatsRecord
	for _, line := range lines {
		var record WebRTCStatsRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to parse merged line %q: %v", line, err)
		}
		got = append(got, record)
	}
	wantTimes := []string{
		"2026-01-01T00:00:01Z",
		"2026-01-01T00:00:02Z",
		"2026-01-01T00:00:03Z",
	}
	for i, want := range wantTimes {
		if got[i].Time != want {
			t.Fatalf("merged record %d time = %s, want %s", i, got[i].Time, want)
		}
	}
}

func writeStatsTestFile(t *testing.T, path string, records []WebRTCStatsRecord) {
	t.Helper()
	var buf bytes.Buffer
	for _, record := range records {
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("failed to encode test record: %v", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
