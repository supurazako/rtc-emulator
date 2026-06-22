package lab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	ScenarioWebRTCUplinkCongestion = "webrtc-uplink-congestion"

	defaultRunsDir       = "runs"
	defaultScenarioNode  = "node1"
	defaultScenarioIface = "eth0"
	defaultUplinkBW      = "1mbit"
)

type ScenarioRunOptions struct {
	Scenario  string
	RunsDir   string
	Node      string
	Interface string
	Delay     string
	Loss      string
	Jitter    string
	BW        string
}

type ScenarioRunResult struct {
	RunID      string
	RunDir     string
	EventsPath string
}

type scenarioRunDeps struct {
	now      func() time.Time
	newRunID func(time.Time) (string, error)
	mkdirAll func(string, os.FileMode) error
	openFile func(string, int, os.FileMode) (io.WriteCloser, error)
}

func RunScenario(ctx context.Context, opts ScenarioRunOptions) (*ScenarioRunResult, error) {
	return runScenarioWithDeps(ctx, opts, defaultCreateDeps(), scenarioRunDeps{})
}

func runScenarioWithDeps(
	ctx context.Context,
	opts ScenarioRunOptions,
	deps createDeps,
	runDeps scenarioRunDeps,
) (*ScenarioRunResult, error) {
	opts = normalizeScenarioRunOptions(opts)
	deps = fillCreateDeps(deps)
	runDeps = fillScenarioRunDeps(runDeps)

	if err := validateScenarioRunOptions(opts); err != nil {
		return nil, err
	}

	startedAt := runDeps.now().UTC()
	runID, err := runDeps.newRunID(startedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate run id: %w", err)
	}

	logger, err := newEventLogger(opts.RunsDir, runID, runDeps)
	if err != nil {
		return nil, err
	}
	result := &ScenarioRunResult{
		RunID:      logger.runID,
		RunDir:     logger.runDir,
		EventsPath: logger.eventsPath,
	}

	condition := ImpairmentCondition{
		Delay:  opts.Delay,
		Loss:   opts.Loss,
		Jitter: opts.Jitter,
		BW:     opts.BW,
	}

	record := func(phase string, action string, status string, opErr error) error {
		return logger.write(EventRecord{
			RunID:     runID,
			Event:     "scenario_phase",
			Scenario:  opts.Scenario,
			Phase:     phase,
			Time:      runDeps.now().UTC().Format(time.RFC3339Nano),
			Node:      opts.Node,
			Interface: opts.Interface,
			Action:    action,
			Condition: condition,
			Status:    status,
			Error:     errorString(opErr),
		})
	}

	var runErr error
	if err := record("baseline", "start", "ok", nil); err != nil {
		return result, errors.Join(err, logger.close())
	}

	_, applyErr := applyWithDeps(ctx, ApplyOptions{
		Node:   opts.Node,
		Delay:  opts.Delay,
		Loss:   opts.Loss,
		Jitter: opts.Jitter,
		BW:     opts.BW,
	}, deps)
	if applyErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("impaired phase failed: %w", applyErr))
	}
	if err := record("impaired", "apply", statusForError(applyErr), applyErr); err != nil {
		runErr = errors.Join(runErr, err)
	}

	if applyErr == nil {
		recoveryResult, recoveryErr := clearWithDeps(ctx, ClearOptions{Node: opts.Node}, deps)
		if recoveryErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("recovery phase failed: %w", recoveryErr))
		}
		if err := record("recovery", "clear", statusForClear(recoveryResult, recoveryErr), recoveryErr); err != nil {
			runErr = errors.Join(runErr, err)
		}
	} else if err := record("recovery", "skip", "skipped", nil); err != nil {
		runErr = errors.Join(runErr, err)
	}

	cleanupResult, cleanupErr := clearWithDeps(ctx, ClearOptions{Node: opts.Node}, deps)
	if cleanupErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("cleanup phase failed: %w", cleanupErr))
	}
	if err := record("cleanup", "clear", statusForClear(cleanupResult, cleanupErr), cleanupErr); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := logger.close(); err != nil {
		runErr = errors.Join(runErr, err)
	}

	return result, runErr
}

func normalizeScenarioRunOptions(opts ScenarioRunOptions) ScenarioRunOptions {
	opts.Scenario = strings.TrimSpace(opts.Scenario)
	opts.RunsDir = strings.TrimSpace(opts.RunsDir)
	opts.Node = strings.TrimSpace(opts.Node)
	opts.Interface = strings.TrimSpace(opts.Interface)
	opts.Delay = strings.TrimSpace(opts.Delay)
	opts.Loss = strings.TrimSpace(opts.Loss)
	opts.Jitter = strings.TrimSpace(opts.Jitter)
	opts.BW = strings.TrimSpace(opts.BW)

	if opts.RunsDir == "" {
		opts.RunsDir = defaultRunsDir
	}
	if opts.Node == "" {
		opts.Node = defaultScenarioNode
	}
	if opts.Interface == "" {
		opts.Interface = defaultScenarioIface
	}
	if opts.Scenario == ScenarioWebRTCUplinkCongestion && opts.Delay == "" && opts.Loss == "" && opts.BW == "" {
		opts.BW = defaultUplinkBW
	}
	return opts
}

func validateScenarioRunOptions(opts ScenarioRunOptions) error {
	if opts.Scenario != ScenarioWebRTCUplinkCongestion {
		return fmt.Errorf("unsupported scenario %q", opts.Scenario)
	}
	if opts.Interface != defaultScenarioIface {
		return fmt.Errorf("unsupported interface %q: only %s is supported", opts.Interface, defaultScenarioIface)
	}
	if opts.Jitter != "" && opts.Delay == "" {
		return errors.New("jitter requires delay")
	}
	if opts.Delay == "" && opts.Loss == "" && opts.BW == "" {
		return errors.New("at least one impairment condition is required")
	}
	return nil
}

func fillScenarioRunDeps(deps scenarioRunDeps) scenarioRunDeps {
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.newRunID == nil {
		deps.newRunID = generateRunID
	}
	if deps.mkdirAll == nil {
		deps.mkdirAll = os.MkdirAll
	}
	if deps.openFile == nil {
		deps.openFile = func(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return os.OpenFile(path, flag, perm)
		}
	}
	return deps
}

func statusForError(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func statusForClear(result *ClearResult, err error) string {
	if err != nil {
		return "error"
	}
	if result != nil && !result.Cleared {
		return "absent"
	}
	return "cleared"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
