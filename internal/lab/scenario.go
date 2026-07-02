package lab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ScenarioWebRTCUplinkCongestion = "webrtc-uplink-congestion"

	defaultRunsDir       = "runs"
	defaultScenarioNode  = "node1"
	defaultScenarioPeer  = "node2"
	defaultScenarioIface = "eth0"
	defaultUplinkBW      = "1mbit"
	defaultBaseline      = 5 * time.Second
	defaultImpaired      = 10 * time.Second
	defaultRecovery      = 5 * time.Second
	scenarioPeerSlack    = 5 * time.Second
	scenarioCleanupLimit = 5 * time.Second
)

type ScenarioRunOptions struct {
	Scenario         string
	RunsDir          string
	Node             string
	Peer             string
	Interface        string
	Delay            string
	Loss             string
	Jitter           string
	BW               string
	BaselineDuration time.Duration
	ImpairedDuration time.Duration
	RecoveryDuration time.Duration
	StatsInterval    time.Duration
	Observer         ScenarioRunObserver
}

type ScenarioRunResult struct {
	RunID      string
	RunDir     string
	LatestDir  string
	EventsPath string
	StatsPath  string
}

type ScenarioRunInfo struct {
	RunID            string
	RunDir           string
	EventsPath       string
	StatsPaths       []string
	Scenario         string
	Node             string
	Peer             string
	BaselineDuration time.Duration
	ImpairedDuration time.Duration
	RecoveryDuration time.Duration
	StatsInterval    time.Duration
	StartedAt        time.Time
	Condition        ImpairmentCondition
}

type ScenarioRunObserver interface {
	ScenarioRunStarted(ScenarioRunInfo)
}

type scenarioRunDeps struct {
	now        func() time.Time
	newRunID   func(time.Time) (string, error)
	mkdirAll   func(string, os.FileMode) error
	openFile   func(string, int, os.FileMode) (io.WriteCloser, error)
	executable func() (string, error)
	runCommand func(context.Context, string, []string, io.Writer, io.Writer) error
	sleep      func(context.Context, time.Duration) error
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
	webRTCOpts := WebRTCP2POptions{
		RunsDir:       opts.RunsDir,
		NodeA:         opts.Node,
		NodeB:         opts.Peer,
		Duration:      opts.BaselineDuration + opts.ImpairedDuration + opts.RecoveryDuration + scenarioPeerSlack,
		StatsInterval: opts.StatsInterval,
	}
	if err := validateWebRTCP2POptions(ctx, webRTCOpts, deps); err != nil {
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
	if err := runDeps.mkdirAll(signalDir(logger.runDir), 0o755); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to create WebRTC signal directory: %w", err), logger.close())
	}
	latestDir, err := updateLatestRunSymlink(opts.RunsDir, runID)
	if err != nil {
		return nil, errors.Join(err, logger.close())
	}
	result := &ScenarioRunResult{
		RunID:      logger.runID,
		RunDir:     logger.runDir,
		LatestDir:  latestDir,
		EventsPath: logger.eventsPath,
		StatsPath:  filepath.Join(logger.runDir, "stats.jsonl"),
	}

	condition := ImpairmentCondition{
		Delay:  opts.Delay,
		Loss:   opts.Loss,
		Jitter: opts.Jitter,
		BW:     opts.BW,
	}
	if opts.Observer != nil {
		opts.Observer.ScenarioRunStarted(ScenarioRunInfo{
			RunID:      runID,
			RunDir:     logger.runDir,
			EventsPath: logger.eventsPath,
			StatsPaths: []string{
				filepath.Join(logger.runDir, peerStatsFilename(opts.Node)),
				filepath.Join(logger.runDir, peerStatsFilename(opts.Peer)),
			},
			Scenario:         opts.Scenario,
			Node:             opts.Node,
			Peer:             opts.Peer,
			BaselineDuration: opts.BaselineDuration,
			ImpairedDuration: opts.ImpairedDuration,
			RecoveryDuration: opts.RecoveryDuration,
			StatsInterval:    opts.StatsInterval,
			StartedAt:        startedAt,
			Condition:        condition,
		})
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
	executable, err := runDeps.executable()
	if err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("failed to resolve current executable: %w", err))
	}
	peerCtx, cancelPeers := context.WithCancel(ctx)
	defer cancelPeers()
	var waitPeers func() error
	peersReady := false
	if err == nil {
		waitPeers = startWebRTCPeerProcesses(peerCtx, webRTCOpts, runID, logger.runDir, executable, webRTCP2PDeps{
			runCommand: runDeps.runCommand,
		}, cancelPeers)
		if readyErr := waitForWebRTCPeerReadiness(ctx, logger.runDir, []string{opts.Node, opts.Peer}, webRTCSignalTimeout); readyErr != nil {
			runErr = errors.Join(runErr, readyErr)
			cancelPeers()
		} else {
			peersReady = true
		}
	}

	if err := record("baseline", "start", "ok", nil); err != nil {
		return result, errors.Join(err, logger.close())
	}
	if runErr == nil {
		runErr = errors.Join(runErr, sleepPhase(ctx, runDeps, opts.BaselineDuration))
	}

	var applyErr error
	if runErr == nil {
		_, applyErr = applyWithDeps(ctx, ApplyOptions{
			Node:   opts.Node,
			Delay:  opts.Delay,
			Loss:   opts.Loss,
			Jitter: opts.Jitter,
			BW:     opts.BW,
		}, deps)
	} else {
		applyErr = runErr
	}
	if applyErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("impaired phase failed: %w", applyErr))
	}
	if err := record("impaired", "apply", statusForError(applyErr), applyErr); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if applyErr == nil {
		runErr = errors.Join(runErr, sleepPhase(ctx, runDeps, opts.ImpairedDuration))
	}

	if applyErr == nil && ctx.Err() == nil {
		recoveryResult, recoveryErr := clearWithDeps(ctx, ClearOptions{Node: opts.Node}, deps)
		if recoveryErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("recovery phase failed: %w", recoveryErr))
		}
		if err := record("recovery", "clear", statusForClear(recoveryResult, recoveryErr), recoveryErr); err != nil {
			runErr = errors.Join(runErr, err)
		}
		runErr = errors.Join(runErr, sleepPhase(ctx, runDeps, opts.RecoveryDuration))
	} else if err := record("recovery", "skip", "skipped", nil); err != nil {
		runErr = errors.Join(runErr, err)
	} else {
		runErr = errors.Join(runErr, sleepPhase(ctx, runDeps, opts.RecoveryDuration))
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), scenarioCleanupLimit)
	cleanupResult, cleanupErr := clearWithDeps(cleanupCtx, ClearOptions{Node: opts.Node}, deps)
	cleanupCancel()
	if cleanupErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("cleanup phase failed: %w", cleanupErr))
	}
	if err := record("cleanup", "clear", statusForClear(cleanupResult, cleanupErr), cleanupErr); err != nil {
		runErr = errors.Join(runErr, err)
	}
	cancelPeers()
	if waitPeers != nil {
		if err := waitPeers(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	if peersReady {
		mergeErr := mergeStatsLogs(result.StatsPath, []string{
			filepath.Join(logger.runDir, peerStatsFilename(opts.Node)),
			filepath.Join(logger.runDir, peerStatsFilename(opts.Peer)),
		})
		if mergeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to merge peer stats logs: %w", mergeErr))
		}
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
	opts.Peer = strings.TrimSpace(opts.Peer)
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
	if opts.Peer == "" {
		opts.Peer = defaultScenarioPeer
	}
	if opts.Interface == "" {
		opts.Interface = defaultScenarioIface
	}
	if opts.BaselineDuration <= 0 {
		opts.BaselineDuration = defaultBaseline
	}
	if opts.ImpairedDuration <= 0 {
		opts.ImpairedDuration = defaultImpaired
	}
	if opts.RecoveryDuration <= 0 {
		opts.RecoveryDuration = defaultRecovery
	}
	if opts.StatsInterval <= 0 {
		opts.StatsInterval = defaultWebRTCStatsInterval
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
	if opts.Node == opts.Peer {
		return errors.New("node and peer must be different")
	}
	if opts.BaselineDuration <= 0 {
		return errors.New("baseline duration must be positive")
	}
	if opts.ImpairedDuration <= 0 {
		return errors.New("impaired duration must be positive")
	}
	if opts.RecoveryDuration <= 0 {
		return errors.New("recovery duration must be positive")
	}
	if opts.StatsInterval <= 0 {
		return errors.New("stats interval must be positive")
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
	if deps.executable == nil {
		deps.executable = os.Executable
	}
	if deps.runCommand == nil {
		deps.runCommand = func(ctx context.Context, name string, args []string, stdout io.Writer, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
		}
	}
	if deps.sleep == nil {
		deps.sleep = func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	return deps
}

func sleepPhase(ctx context.Context, deps scenarioRunDeps, d time.Duration) error {
	if err := deps.sleep(ctx, d); err != nil {
		return fmt.Errorf("phase wait interrupted: %w", err)
	}
	return nil
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
