package monitor

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/supurazako/rtc-emulator/internal/lab"
)

const (
	historyWindow = 60 * time.Second
	tickInterval  = 500 * time.Millisecond
	minWidth      = 120
	minHeight     = 36
)

type Monitor struct {
	started  chan lab.ScenarioRunInfo
	finished chan error
}

func New() *Monitor {
	return &Monitor{
		started:  make(chan lab.ScenarioRunInfo, 1),
		finished: make(chan error, 1),
	}
}

func (m *Monitor) ScenarioRunStarted(info lab.ScenarioRunInfo) {
	select {
	case m.started <- info:
	default:
	}
}

func (m *Monitor) Finish(err error) {
	select {
	case m.finished <- err:
	default:
	}
}

func (m *Monitor) Run(ctx context.Context, cancel context.CancelFunc) error {
	model := liveModel{
		started:  m.started,
		finished: m.finished,
		cancel:   cancel,
	}
	_, err := tea.NewProgram(model).Run()
	return err
}

type startedMsg lab.ScenarioRunInfo

type finishedMsg struct {
	err error
}

type tickMsg time.Time

type liveModel struct {
	started  <-chan lab.ScenarioRunInfo
	finished <-chan error
	cancel   context.CancelFunc

	info     *lab.ScenarioRunInfo
	events   []lab.EventRecord
	stats    []lab.WebRTCStatsRecord
	snapshot Snapshot
	readErr  error
	offsets  map[string]int64
	width    int
	height   int
}

func (m liveModel) Init() tea.Cmd {
	return tea.Batch(waitStartOrFinish(m.started, m.finished), tick())
}

func (m liveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case startedMsg:
		info := lab.ScenarioRunInfo(msg)
		m.info = &info
		m.refresh(time.Now())
		return m, waitFinished(m.finished)
	case finishedMsg:
		return m, tea.Quit
	case tickMsg:
		m.refresh(time.Time(msg))
		return m, tick()
	}
	return m, nil
}

func (m liveModel) View() tea.View {
	var content string
	if m.info == nil {
		content = "rtc-emulator live monitor\n\nwaiting for scenario run...\n\nq / ctrl+c: stop\n"
	} else if m.width > 0 && m.height > 0 && (m.width < minWidth || m.height < minHeight) {
		content = m.compactView()
	} else {
		content = m.dashboardView()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m *liveModel) refresh(now time.Time) {
	if m.info == nil {
		return
	}
	if m.offsets == nil {
		m.offsets = make(map[string]int64)
	}
	events, eventErr := readEventsFrom(m.info.EventsPath, m.offsets)
	stats, statsErr := readStatsFrom(m.info.StatsPaths, m.offsets)
	m.events = append(m.events, events...)
	m.stats = append(m.stats, stats...)
	m.trim(now)
	m.snapshot = BuildSnapshot(*m.info, m.events, m.stats, now)
	m.readErr = errors.Join(eventErr, statsErr)
}

func (m *liveModel) trim(now time.Time) {
	if len(m.events) > 50 {
		m.events = m.events[len(m.events)-50:]
	}
	cutoff := now.Add(-historyWindow - tickInterval)
	kept := m.stats[:0]
	for _, record := range m.stats {
		t, ok := parseTime(record.Time)
		if !ok || !t.Before(cutoff) {
			kept = append(kept, record)
		}
	}
	m.stats = kept
}

func readErrorText(err error) string {
	if err == nil {
		return ""
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func waitStartOrFinish(started <-chan lab.ScenarioRunInfo, finished <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case info := <-started:
			return startedMsg(info)
		case err := <-finished:
			return finishedMsg{err: err}
		}
	}
}

func waitFinished(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		return finishedMsg{err: <-ch}
	}
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
