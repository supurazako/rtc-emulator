package monitor

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/guptarohit/asciigraph"
	"github.com/supurazako/rtc-emulator/internal/lab"
)

const (
	chartWidth  = 56
	chartHeight = 8
)

func (m liveModel) dashboardView() string {
	s := m.snapshot
	width := chartWidth*2 + 2
	top := fitLine(fmt.Sprintf(
		"rtc-emulator live   %s   phase=%s   elapsed=%s",
		s.Scenario,
		displayOrDash(s.Phase),
		formatDuration(s.Elapsed),
	), width) + "\n"
	top += fitLine(fmt.Sprintf(
		"event: %s   pc=%s   ice=%s",
		displayOrDash(s.LastEvent),
		displayOrDash(s.PeerConnectionState),
		displayOrDash(s.ICEConnectionState),
	), width) + "\n\n"
	if m.readErr != nil {
		top += fitLine("read: "+readErrorText(m.readErr), width) + "\n\n"
	}

	row1 := joinColumns(
		renderChart("send bitrate", s.Bitrate.CurrentText, s.Bitrate.Values, chartWidth, chartHeight),
		renderChart("RTT", s.RTT.CurrentText, s.RTT.Values, chartWidth, chartHeight),
	)
	row2 := joinColumns(
		renderChart("jitter", s.Jitter.CurrentText, s.Jitter.Values, chartWidth, chartHeight),
		renderChart("packet loss", s.PacketLoss.CurrentText, s.PacketLoss.Values, chartWidth, chartHeight),
	)

	footer := "\nrecent events:\n"
	for _, event := range recentEvents(m.events, 3) {
		footer += fitLine(fmt.Sprintf("%s %s %s %s", shortTime(event.Time), event.Phase, event.Action, event.Status), width) + "\n"
	}
	footer += "\nq / ctrl+c: stop scenario and cleanup\n"
	return top + row1 + "\n" + row2 + footer
}

func (m liveModel) compactView() string {
	s := m.snapshot
	width := max(20, m.width)
	var b strings.Builder
	b.WriteString(fitLine(fmt.Sprintf("rtc-emulator live   %s   phase=%s   elapsed=%s", s.Scenario, displayOrDash(s.Phase), formatDuration(s.Elapsed)), width) + "\n")
	b.WriteString(fitLine(fmt.Sprintf("terminal %dx%d; %dx%d recommended for full charts", m.width, m.height, minWidth, minHeight), width) + "\n\n")
	for _, metric := range []Metric{s.Bitrate, s.RTT, s.Jitter, s.PacketLoss} {
		b.WriteString(fitLine(fmt.Sprintf("%-14s %12s  %s", metric.Name, metric.CurrentText, sparkline(metric.Values, 30)), width) + "\n")
	}
	b.WriteString("\n" + fitLine(fmt.Sprintf("event=%s pc=%s ice=%s", displayOrDash(s.LastEvent), displayOrDash(s.PeerConnectionState), displayOrDash(s.ICEConnectionState)), width) + "\n")
	if m.readErr != nil {
		b.WriteString(fitLine("read="+readErrorText(m.readErr), width) + "\n")
	}
	b.WriteString("\nq / ctrl+c: stop scenario and cleanup\n")
	return b.String()
}

func renderChart(title string, current string, values []float64, width int, height int) string {
	innerWidth := max(10, width-4)
	graph := asciigraph.Plot(valuesOrZero(values),
		asciigraph.Width(innerWidth),
		asciigraph.Height(height),
		asciigraph.Offset(0),
		asciigraph.Precision(1),
	)

	lines := []string{chartHeader(title, current, width)}
	for _, line := range strings.Split(graph, "\n") {
		lines = append(lines, "│ "+padRight(line, width-4)+" │")
	}
	lines = append(lines, chartFooter(width))
	return strings.Join(lines, "\n")
}

func chartHeader(title string, current string, width int) string {
	header := fmt.Sprintf("┌ %-18s %12s ", title, current)
	header = truncateRunes(header, width-1)
	return header + strings.Repeat("─", max(0, width-runeLen(header)-1)) + "┐"
}

func fitLine(value string, width int) string {
	return padRight(truncateRunes(value, width), width)
}

func chartFooter(width int) string {
	footer := "└ 60s ago"
	footer += strings.Repeat(" ", max(1, width-runeLen(footer)-8))
	return footer + "now ┘"
}

func joinColumns(left string, right string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	n := max(len(leftLines), len(rightLines))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, lineAt(leftLines, i)+"  "+lineAt(rightLines, i))
	}
	return strings.Join(out, "\n")
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return ""
	}
	return lines[i]
}

func sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat("·", width)
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	levels := []rune("▁▂▃▄▅▆▇█")
	minV, maxV := minMax(values)
	var b strings.Builder
	for _, v := range values {
		idx := 0
		if maxV > minV {
			idx = int(math.Round((v - minV) / (maxV - minV) * float64(len(levels)-1)))
		}
		idx = clamp(idx, 0, len(levels)-1)
		b.WriteRune(levels[idx])
	}
	return padRight(b.String(), width)
}

func recentEvents(events []lab.EventRecord, n int) []lab.EventRecord {
	if len(events) <= n {
		return events
	}
	return events[len(events)-n:]
}

func valuesOrZero(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{0}
	}
	return values
}

func minMax(values []float64) (float64, float64) {
	minV := values[0]
	maxV := values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func shortTime(value string) string {
	t, ok := parseTime(value)
	if !ok {
		return "--:--:--"
	}
	return t.Format("15:04:05")
}

func formatDuration(d time.Duration) string {
	seconds := int(d.Round(time.Second).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func displayOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func padRight(value string, width int) string {
	length := runeLen(value)
	if length >= width {
		return truncateRunes(value, width)
	}
	return value + strings.Repeat(" ", width-length)
}

func truncateRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(value) <= width {
		return value
	}
	out := make([]rune, 0, width)
	for _, r := range value {
		if len(out) == width {
			break
		}
		out = append(out, r)
	}
	return string(out)
}

func runeLen(value string) int {
	return utf8.RuneCountInString(value)
}

func clamp(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
