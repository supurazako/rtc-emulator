package monitor

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/supurazako/rtc-emulator/internal/lab"
)

type Snapshot struct {
	Scenario            string
	Phase               string
	LastEvent           string
	Elapsed             time.Duration
	PeerConnectionState string
	ICEConnectionState  string
	Bitrate             Metric
	RTT                 Metric
	Jitter              Metric
	PacketLoss          Metric
}

type Metric struct {
	Name        string
	CurrentText string
	Values      []float64
}

func BuildSnapshot(info lab.ScenarioRunInfo, events []lab.EventRecord, stats []lab.WebRTCStatsRecord, now time.Time) Snapshot {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time < events[j].Time
	})
	sort.SliceStable(stats, func(i, j int) bool {
		return stats[i].Time < stats[j].Time
	})

	snapshot := Snapshot{
		Scenario:   info.Scenario,
		Phase:      "starting",
		Elapsed:    maxDuration(0, now.Sub(info.StartedAt)),
		Bitrate:    Metric{Name: "send bitrate"},
		RTT:        Metric{Name: "RTT"},
		Jitter:     Metric{Name: "jitter"},
		PacketLoss: Metric{Name: "packet loss"},
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		snapshot.Phase = last.Phase
		snapshot.LastEvent = strings.TrimSpace(last.Action + " " + conditionText(last.Condition))
	}

	nodeStats := statsForNode(stats, info.Node)
	if len(nodeStats) > 0 {
		last := nodeStats[len(nodeStats)-1]
		snapshot.PeerConnectionState = last.PeerConnectionState
		snapshot.ICEConnectionState = last.ICEConnectionState
	}

	cutoff := now.Add(-historyWindow)
	bitrate := deriveBitrate(nodeStats, cutoff)
	rtt := valuesFromStats(nodeStats, cutoff, func(r lab.WebRTCStatsRecord) (float64, bool) {
		if r.RoundTripTime == nil {
			return 0, false
		}
		return *r.RoundTripTime * 1000, true
	})
	jitter := valuesFromStats(nodeStats, cutoff, func(r lab.WebRTCStatsRecord) (float64, bool) {
		if r.Jitter == nil {
			return 0, false
		}
		return *r.Jitter * 1000, true
	})
	loss := derivePacketLossDelta(nodeStats, cutoff)

	snapshot.Bitrate.Values = bitrate
	snapshot.Bitrate.CurrentText = formatCurrent(lastFloat(bitrate), "Mbps", 2)
	snapshot.RTT.Values = rtt
	snapshot.RTT.CurrentText = formatCurrent(lastFloat(rtt), "ms", 0)
	snapshot.Jitter.Values = jitter
	snapshot.Jitter.CurrentText = formatCurrent(lastFloat(jitter), "ms", 0)
	snapshot.PacketLoss.Values = loss
	snapshot.PacketLoss.CurrentText = formatPacketLoss(nodeStats)
	return snapshot
}

func statsForNode(stats []lab.WebRTCStatsRecord, node string) []lab.WebRTCStatsRecord {
	nodeStats := make([]lab.WebRTCStatsRecord, 0, len(stats))
	for _, record := range stats {
		if record.Node == node {
			nodeStats = append(nodeStats, record)
		}
	}
	if len(nodeStats) == 0 {
		return stats
	}
	return nodeStats
}

func deriveBitrate(records []lab.WebRTCStatsRecord, cutoff time.Time) []float64 {
	values := make([]float64, 0, len(records))
	var prev *lab.WebRTCStatsRecord
	for i := range records {
		record := records[i]
		t, ok := parseTime(record.Time)
		if !ok || t.Before(cutoff) || record.BytesSent == nil {
			continue
		}
		if prev != nil && prev.BytesSent != nil {
			prevT, ok := parseTime(prev.Time)
			if ok {
				seconds := t.Sub(prevT).Seconds()
				bytesDelta := int64(*record.BytesSent) - int64(*prev.BytesSent)
				if seconds > 0 && bytesDelta >= 0 {
					values = append(values, float64(bytesDelta)*8/seconds/1_000_000)
				}
			}
		}
		prev = &record
	}
	return values
}

func valuesFromStats(records []lab.WebRTCStatsRecord, cutoff time.Time, value func(lab.WebRTCStatsRecord) (float64, bool)) []float64 {
	values := make([]float64, 0, len(records))
	for _, record := range records {
		t, ok := parseTime(record.Time)
		if !ok || t.Before(cutoff) {
			continue
		}
		v, ok := value(record)
		if ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
			values = append(values, v)
		}
	}
	return values
}

func derivePacketLossDelta(records []lab.WebRTCStatsRecord, cutoff time.Time) []float64 {
	values := make([]float64, 0, len(records))
	var prev *lab.WebRTCStatsRecord
	for i := range records {
		record := records[i]
		t, ok := parseTime(record.Time)
		if !ok || t.Before(cutoff) || record.PacketsLost == nil {
			continue
		}
		if prev != nil && prev.PacketsLost != nil {
			delta := *record.PacketsLost - *prev.PacketsLost
			if delta >= 0 {
				values = append(values, float64(delta))
			}
		}
		prev = &record
	}
	return values
}

func formatCurrent(value float64, unit string, precision int) string {
	if math.IsNaN(value) {
		return "collecting"
	}
	return fmt.Sprintf("%.*f %s", precision, value, unit)
}

func formatPacketLoss(records []lab.WebRTCStatsRecord) string {
	if len(records) == 0 {
		return "collecting"
	}
	var last *int64
	var prev *int64
	for i := range records {
		if records[i].PacketsLost == nil {
			continue
		}
		prev = last
		last = records[i].PacketsLost
	}
	if last == nil {
		return "collecting"
	}
	delta := int64(0)
	if prev != nil {
		delta = *last - *prev
	}
	return fmt.Sprintf("+%d / %d", delta, *last)
}

func conditionText(condition lab.ImpairmentCondition) string {
	parts := make([]string, 0, 4)
	if condition.Delay != "" {
		parts = append(parts, "delay="+condition.Delay)
	}
	if condition.Loss != "" {
		parts = append(parts, "loss="+condition.Loss)
	}
	if condition.Jitter != "" {
		parts = append(parts, "jitter="+condition.Jitter)
	}
	if condition.BW != "" {
		parts = append(parts, "bw="+condition.BW)
	}
	return strings.Join(parts, " ")
}

func lastFloat(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	return values[len(values)-1]
}

func parseTime(value string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func maxDuration(min time.Duration, value time.Duration) time.Duration {
	if value < min {
		return min
	}
	return value
}
