package lab

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

type WebRTCStatsRecord struct {
	RunID                string   `json:"run_id"`
	Time                 string   `json:"time"`
	Node                 string   `json:"node"`
	Peer                 string   `json:"peer"`
	PeerConnectionState  string   `json:"peer_connection_state"`
	ICEConnectionState   string   `json:"ice_connection_state"`
	BytesSent            *uint64  `json:"bytes_sent,omitempty"`
	BytesReceived        *uint64  `json:"bytes_received,omitempty"`
	PacketsSent          *uint64  `json:"packets_sent,omitempty"`
	PacketsReceived      *uint64  `json:"packets_received,omitempty"`
	PacketsLost          *int64   `json:"packets_lost,omitempty"`
	RoundTripTime        *float64 `json:"round_trip_time,omitempty"`
	Jitter               *float64 `json:"jitter,omitempty"`
	FramesSent           *uint64  `json:"frames_sent,omitempty"`
	FramesReceived       *uint64  `json:"frames_received,omitempty"`
	DataMessagesSent     *uint64  `json:"data_messages_sent,omitempty"`
	DataMessagesReceived *uint64  `json:"data_messages_received,omitempty"`
	DataChannelsOpened   *uint64  `json:"data_channels_opened,omitempty"`
	DataChannelsClosed   *uint64  `json:"data_channels_closed,omitempty"`
}

type statsLogger struct {
	path   string
	writer io.WriteCloser
}

func newStatsLogger(path string, openFile func(string, int, os.FileMode) (io.WriteCloser, error)) (*statsLogger, error) {
	writer, err := openFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open stats log %s: %w", path, err)
	}
	return &statsLogger{path: path, writer: writer}, nil
}

func (l *statsLogger) write(record WebRTCStatsRecord) error {
	b, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode stats record: %w", err)
	}
	if _, err := l.writer.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("failed to write stats log %s: %w", l.path, err)
	}
	return nil
}

func (l *statsLogger) close() error {
	if l.writer == nil {
		return nil
	}
	if err := l.writer.Close(); err != nil {
		return fmt.Errorf("failed to close stats log %s: %w", l.path, err)
	}
	l.writer = nil
	return nil
}

func mergeStatsLogs(outputPath string, inputPaths []string) error {
	records := make([]WebRTCStatsRecord, 0)
	for _, inputPath := range inputPaths {
		file, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("failed to open peer stats log %s: %w", inputPath, err)
		}
		scanner := bufio.NewScanner(file)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			var record WebRTCStatsRecord
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				_ = file.Close()
				return fmt.Errorf("failed to parse stats log %s:%d: %w", inputPath, lineNo, err)
			}
			records = append(records, record)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return fmt.Errorf("failed to read stats log %s: %w", inputPath, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close stats log %s: %w", inputPath, err)
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339Nano, records[i].Time)
		tj, errJ := time.Parse(time.RFC3339Nano, records[j].Time)
		if errI != nil || errJ != nil {
			return records[i].Time < records[j].Time
		}
		return ti.Before(tj)
	})

	logger, err := newStatsLogger(outputPath, func(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(path, flag, perm)
	})
	if err != nil {
		return err
	}
	var writeErr error
	for _, record := range records {
		if err := logger.write(record); err != nil {
			writeErr = err
			break
		}
	}
	if err := logger.close(); err != nil && writeErr == nil {
		writeErr = err
	}
	return writeErr
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
