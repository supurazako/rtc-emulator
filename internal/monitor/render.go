package monitor

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"
	"github.com/supurazako/rtc-emulator/internal/lab"
)

const (
	defaultDashboardWidth = 114
	minChartWidth         = 56
	minChartHeight        = 8
	maxChartHeight        = 18
	chartColumnGap        = 2
	dashboardFixedLines   = 13
)

func (m liveModel) dashboardView() string {
	s := m.snapshot
	layout := dashboardLayoutFor(m.width, m.height)
	width := layout.width
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
		renderChart(s.Bitrate, s.WindowStart, s.WindowEnd, s.StartedAt, layout.chartWidth, layout.chartHeight),
		renderChart(s.RTT, s.WindowStart, s.WindowEnd, s.StartedAt, layout.chartWidth, layout.chartHeight),
	)
	row2 := joinColumns(
		renderChart(s.Jitter, s.WindowStart, s.WindowEnd, s.StartedAt, layout.chartWidth, layout.chartHeight),
		renderChart(s.PacketLoss, s.WindowStart, s.WindowEnd, s.StartedAt, layout.chartWidth, layout.chartHeight),
	)

	footer := "\nrecent events:\n"
	for _, event := range recentEvents(m.events, 3) {
		footer += fitLine(fmt.Sprintf("%s %s %s %s", shortTime(event.Time), event.Phase, event.Action, event.Status), width) + "\n"
	}
	footer += "\nq / ctrl+c: stop scenario and cleanup\n"
	return top + row1 + "\n" + row2 + footer
}

type dashboardLayout struct {
	width       int
	chartWidth  int
	chartHeight int
}

func dashboardLayoutFor(termWidth int, termHeight int) dashboardLayout {
	width := termWidth
	if width <= 0 {
		width = defaultDashboardWidth
	}
	width = max(minWidth, width)
	chartWidth := max(minChartWidth, (width-chartColumnGap)/2)
	width = chartWidth*2 + chartColumnGap

	chartHeight := minChartHeight
	if termHeight > 0 {
		available := termHeight - dashboardFixedLines
		if available > 0 {
			chartHeight = clamp(available/2, minChartHeight, maxChartHeight)
		}
	}
	return dashboardLayout{
		width:       width,
		chartWidth:  chartWidth,
		chartHeight: chartHeight,
	}
}

func (m liveModel) compactView() string {
	s := m.snapshot
	width := max(20, m.width)
	var b strings.Builder
	b.WriteString(fitLine(fmt.Sprintf("rtc-emulator live   %s   phase=%s   elapsed=%s", s.Scenario, displayOrDash(s.Phase), formatDuration(s.Elapsed)), width) + "\n")
	b.WriteString(fitLine(fmt.Sprintf("terminal %dx%d; %dx%d recommended for full charts", m.width, m.height, minWidth, minHeight), width) + "\n\n")
	for _, metric := range []Metric{s.Bitrate, s.RTT, s.Jitter, s.PacketLoss} {
		b.WriteString(fitLine(fmt.Sprintf("%-14s %12s  %s", metric.Name, metric.CurrentText, sparkline(metricValues(metric.Points), 30)), width) + "\n")
	}
	b.WriteString("\n" + fitLine(fmt.Sprintf("event=%s pc=%s ice=%s", displayOrDash(s.LastEvent), displayOrDash(s.PeerConnectionState), displayOrDash(s.ICEConnectionState)), width) + "\n")
	if m.readErr != nil {
		b.WriteString(fitLine("read="+readErrorText(m.readErr), width) + "\n")
	}
	b.WriteString("\nq / ctrl+c: stop scenario and cleanup\n")
	return b.String()
}

func renderChart(metric Metric, windowStart time.Time, windowEnd time.Time, startedAt time.Time, width int, height int) string {
	innerWidth := max(10, width-4)
	lines := []string{chartHeader(metric, width)}
	visiblePoints := pointsInWindow(metric.Points, windowStart, windowEnd)

	if len(visiblePoints) == 0 {
		for _, line := range emptyChartLines(innerWidth, height) {
			lines = append(lines, "│ "+padRight(line, width-4)+" │")
		}
		lines = append(lines, chartFooter(width, windowStart, windowEnd, startedAt))
		return strings.Join(lines, "\n")
	}

	graph := renderLineChart(visiblePoints, windowStart, windowEnd, innerWidth, height)
	for _, line := range strings.Split(graph, "\n") {
		lines = append(lines, "│ "+padRight(line, width-4)+" │")
	}
	lines = append(lines, chartFooter(width, windowStart, windowEnd, startedAt))
	return strings.Join(lines, "\n")
}

func renderLineChart(points []MetricPoint, windowStart time.Time, windowEnd time.Time, width int, height int) string {
	width = max(2, width)
	height = max(2, height)
	values := metricValues(points)
	minY, maxY := minMax(values)
	minY, maxY = paddedRange(minY, maxY)
	maxX := windowEnd.Sub(windowStart).Seconds()
	if maxX <= 0 {
		maxX = historyWindow.Seconds()
	}

	chart := linechart.New(width, height, 0, maxX, minY, maxY,
		linechart.WithXYSteps(0, 0),
	)
	prev := chartPoint(points[0], windowStart, maxX)
	chart.DrawRune(prev, '•')
	for _, point := range points[1:] {
		next := chartPoint(point, windowStart, maxX)
		chart.DrawLine(prev, next, runes.ThinLineStyle)
		prev = next
	}
	return chart.View()
}

func chartPoint(point MetricPoint, windowStart time.Time, maxX float64) canvas.Float64Point {
	x := point.Time.Sub(windowStart).Seconds()
	if x < 0 {
		x = 0
	}
	if x > maxX {
		x = maxX
	}
	return canvas.Float64Point{X: x, Y: point.Value}
}

func paddedRange(minY float64, maxY float64) (float64, float64) {
	if minY == maxY {
		minY--
		maxY++
	}
	padding := (maxY - minY) * 0.1
	return minY - padding, maxY + padding
}

func pointsInWindow(points []MetricPoint, windowStart time.Time, windowEnd time.Time) []MetricPoint {
	out := make([]MetricPoint, 0, len(points))
	for _, point := range points {
		if point.Time.Before(windowStart) || point.Time.After(windowEnd) {
			continue
		}
		out = append(out, point)
	}
	return out
}

func metricValues(points []MetricPoint) []float64 {
	values := make([]float64, 0, len(points))
	for _, point := range points {
		values = append(values, point.Value)
	}
	return values
}

func emptyChartLines(width int, height int) []string {
	lines := make([]string, max(2, height))
	mid := len(lines) / 2
	label := "collecting..."
	for i := range lines {
		if i == mid {
			lines[i] = padRight(label, width)
			continue
		}
		lines[i] = strings.Repeat(" ", width)
	}
	return lines
}

func chartHeader(metric Metric, width int) string {
	headerValue := metric.CurrentText
	if metric.MaxText != "" {
		headerValue = strings.TrimSpace(headerValue + "  " + metric.MaxText)
	}
	header := fmt.Sprintf("┌ %-18s %s ", metric.Name, headerValue)
	header = truncateRunes(header, width-1)
	return header + strings.Repeat("─", max(0, width-runeLen(header)-1)) + "┐"
}

func fitLine(value string, width int) string {
	return padRight(truncateRunes(value, width), width)
}

func chartFooter(width int, windowStart time.Time, windowEnd time.Time, startedAt time.Time) string {
	footer := []rune(padRight("└ 60s ago", width))
	writeRight(footer, "now ┘")
	windowSeconds := windowEnd.Sub(windowStart).Seconds()
	if windowSeconds > 0 && startedAt.After(windowStart) && startedAt.Before(windowEnd) {
		x := int(math.Round(startedAt.Sub(windowStart).Seconds() / windowSeconds * float64(width-1)))
		maxStartX := width - runeLen("now ┘") - runeLen(" start")
		writeAt(footer, clamp(x, 0, maxStartX), "start")
	}
	return string(footer)
}

func writeRight(line []rune, value string) {
	writeAt(line, max(0, len(line)-runeLen(value)), value)
}

func writeAt(line []rune, idx int, value string) {
	if idx < 0 || idx >= len(line) {
		return
	}
	for _, r := range value {
		if idx >= len(line) {
			return
		}
		line[idx] = r
		idx++
	}
}

func joinColumns(left string, right string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	n := max(len(leftLines), len(rightLines))
	out := make([]string, 0, n)
	gap := strings.Repeat(" ", chartColumnGap)
	for i := 0; i < n; i++ {
		out = append(out, lineAt(leftLines, i)+gap+lineAt(rightLines, i))
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
