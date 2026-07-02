package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/supurazako/rtc-emulator/internal/lab"
)

func TestLabImpairHelpListsApplyAndClear(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Manage node network impairments", "apply", "clear"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabScenarioRunHelpListsBuiltInScenarioOptions(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "scenario", "run", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"headless Pion WebRTC peers",
		"does not require a browser",
		"--runs-dir",
		"--node",
		"--peer",
		"--bw",
		"--baseline",
		"--impaired",
		"--recovery",
		"--stats-interval",
		"--watch",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabWebRTCP2PHelpListsStatsOptions(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "webrtc", "p2p", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Run a lab WebRTC P2P flow",
		"--node-a",
		"--node-b",
		"--duration",
		"--stats-interval",
		"--runs-dir",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabWebRTCHelpHidesInternalPeerCommand(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "webrtc", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "peer") {
		t.Fatalf("expected help not to expose internal peer command, got:\n%s", got)
	}
	if !strings.Contains(got, "p2p") {
		t.Fatalf("expected help to expose p2p command, got:\n%s", got)
	}
}

func TestPrintScenarioRunResultIncludesStatsAndLatestDir(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	printScenarioRunResult(cmd, &lab.ScenarioRunResult{
		RunID:      "run-1",
		RunDir:     "runs/run-1",
		LatestDir:  "runs/latest",
		EventsPath: "runs/run-1/events.jsonl",
		StatsPath:  "runs/run-1/stats.jsonl",
	})

	got := out.String()
	for _, want := range []string{
		"run-id=run-1\n",
		"run-dir=runs/run-1\n",
		"latest-dir=runs/latest\n",
		"events=runs/run-1/events.jsonl\n",
		"stats=runs/run-1/stats.jsonl\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestPrintWebRTCP2PResultIncludesLatestDir(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	printWebRTCP2PResult(cmd, &lab.WebRTCP2PResult{
		RunID:      "run-1",
		RunDir:     "runs/run-1",
		LatestDir:  "runs/latest",
		EventsPath: "runs/run-1/events.jsonl",
		StatsPath:  "runs/run-1/stats.jsonl",
	})

	got := out.String()
	for _, want := range []string{
		"run-id=run-1\n",
		"run-dir=runs/run-1\n",
		"latest-dir=runs/latest\n",
		"events=runs/run-1/events.jsonl\n",
		"stats=runs/run-1/stats.jsonl\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestRunScenarioWithMonitorRejectsNonTTY(t *testing.T) {
	err := runScenarioWithMonitor(&cobra.Command{}, lab.ScenarioRunOptions{
		Scenario: lab.ScenarioWebRTCUplinkCongestion,
	})
	if err == nil || !strings.Contains(err.Error(), "--watch requires an interactive terminal") {
		t.Fatalf("expected non-TTY watch error, got: %v", err)
	}
}

func TestRunScenarioWithMonitorDepsRunsScenarioWithObserver(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	mon := &fakeScenarioMonitor{finished: make(chan error, 1)}
	var gotObserver bool

	err := runScenarioWithMonitorDeps(cmd, lab.ScenarioRunOptions{
		Scenario: lab.ScenarioWebRTCUplinkCongestion,
	}, scenarioMonitorDeps{
		isTerminal: func() bool { return true },
		newMonitor: func() scenarioMonitor { return mon },
		runScenario: func(_ context.Context, opts lab.ScenarioRunOptions) (*lab.ScenarioRunResult, error) {
			gotObserver = opts.Observer == mon
			opts.Observer.ScenarioRunStarted(lab.ScenarioRunInfo{RunID: "run-watch"})
			return &lab.ScenarioRunResult{
				RunID:      "run-watch",
				RunDir:     "runs/run-watch",
				LatestDir:  "runs/latest",
				EventsPath: "runs/run-watch/events.jsonl",
				StatsPath:  "runs/run-watch/stats.jsonl",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotObserver || !mon.started {
		t.Fatalf("expected monitor observer to be wired, gotObserver=%v started=%v", gotObserver, mon.started)
	}
	if !strings.Contains(out.String(), "stats=runs/run-watch/stats.jsonl\n") {
		t.Fatalf("expected scenario result output, got:\n%s", out.String())
	}
}

func TestRunScenarioWithMonitorDepsJoinsMonitorAndScenarioErrors(t *testing.T) {
	cmd := &cobra.Command{}
	monErr := errors.New("monitor failed")
	scenarioErr := errors.New("scenario failed")
	mon := &fakeScenarioMonitor{
		finished: make(chan error, 1),
		runErr:   monErr,
	}

	err := runScenarioWithMonitorDeps(cmd, lab.ScenarioRunOptions{
		Scenario: lab.ScenarioWebRTCUplinkCongestion,
	}, scenarioMonitorDeps{
		isTerminal: func() bool { return true },
		newMonitor: func() scenarioMonitor { return mon },
		runScenario: func(_ context.Context, opts lab.ScenarioRunOptions) (*lab.ScenarioRunResult, error) {
			opts.Observer.ScenarioRunStarted(lab.ScenarioRunInfo{RunID: "run-watch"})
			return nil, scenarioErr
		},
	})
	if !errors.Is(err, scenarioErr) || !errors.Is(err, monErr) {
		t.Fatalf("expected joined scenario and monitor errors, got: %v", err)
	}
}

func TestLabImpairApplyRejectsPositionalArgs(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "apply", "--node", "node1", "--bw", "1mbit", "extra"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "extra"`) {
		t.Fatalf("expected positional arg error, got: %v", err)
	}
}

type fakeScenarioMonitor struct {
	started  bool
	finished chan error
	runErr   error
}

func (m *fakeScenarioMonitor) ScenarioRunStarted(lab.ScenarioRunInfo) {
	m.started = true
}

func (m *fakeScenarioMonitor) Finish(err error) {
	m.finished <- err
}

func (m *fakeScenarioMonitor) Run(context.Context, context.CancelFunc) error {
	<-m.finished
	return m.runErr
}

func TestLabImpairClearRejectsPositionalArgs(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "clear", "--node", "node1", "extra"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "extra"`) {
		t.Fatalf("expected positional arg error, got: %v", err)
	}
}
