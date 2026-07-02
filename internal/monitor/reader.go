package monitor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/supurazako/rtc-emulator/internal/lab"
)

func readEvents(path string) ([]lab.EventRecord, error) {
	return readEventsFrom(path, map[string]int64{})
}

func readEventsFrom(path string, offsets map[string]int64) ([]lab.EventRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	offset, err := seekToOffset(file, offsets[path])
	if err != nil {
		return nil, fmt.Errorf("seek events %s: %w", path, err)
	}

	var events []lab.EventRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		offset += int64(len(scanner.Bytes()) + 1)
		var event lab.EventRecord
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return events, err
	}
	offsets[path] = offset
	return events, nil
}

func readStats(paths []string) ([]lab.WebRTCStatsRecord, error) {
	return readStatsFrom(paths, map[string]int64{})
}

func readStatsFrom(paths []string, offsets map[string]int64) ([]lab.WebRTCStatsRecord, error) {
	var records []lab.WebRTCStatsRecord
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return records, fmt.Errorf("read stats %s: %w", path, err)
		}
		offset, err := seekToOffset(file, offsets[path])
		if err != nil {
			_ = file.Close()
			return records, fmt.Errorf("seek stats %s: %w", path, err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			offset += int64(len(scanner.Bytes()) + 1)
			var record lab.WebRTCStatsRecord
			if err := json.Unmarshal(scanner.Bytes(), &record); err == nil {
				records = append(records, record)
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return records, fmt.Errorf("read stats %s: %w", path, scanErr)
		}
		if closeErr != nil {
			return records, fmt.Errorf("close stats %s: %w", path, closeErr)
		}
		offsets[path] = offset
	}
	return records, nil
}

func seekToOffset(file *os.File, offset int64) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if offset > info.Size() {
		offset = 0
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return 0, err
	}
	return offset, nil
}
