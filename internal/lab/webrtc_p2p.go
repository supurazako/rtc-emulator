package lab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultWebRTCDuration      = 10 * time.Second
	defaultWebRTCStatsInterval = time.Second
	defaultWebRTCNodeA         = "node1"
	defaultWebRTCNodeB         = "node2"
	latestRunSymlinkName       = "latest"
	webRTCP2PEventName         = "webrtc_p2p"
	webRTCSignalTimeout        = 20 * time.Second
	webRTCSignalPollInterval   = 50 * time.Millisecond
)

type WebRTCP2POptions struct {
	RunsDir       string
	NodeA         string
	NodeB         string
	Duration      time.Duration
	StatsInterval time.Duration
}

type WebRTCP2PResult struct {
	RunID      string
	RunDir     string
	LatestDir  string
	EventsPath string
	StatsPath  string
}

type webRTCP2PDeps struct {
	createDeps
	now        func() time.Time
	newRunID   func(time.Time) (string, error)
	mkdirAll   func(string, os.FileMode) error
	openFile   func(string, int, os.FileMode) (io.WriteCloser, error)
	executable func() (string, error)
	runCommand func(context.Context, string, []string, io.Writer, io.Writer) error
}

func RunWebRTCP2P(ctx context.Context, opts WebRTCP2POptions) (*WebRTCP2PResult, error) {
	return runWebRTCP2PWithDeps(ctx, opts, defaultWebRTCP2PDeps())
}

func defaultWebRTCP2PDeps() webRTCP2PDeps {
	return webRTCP2PDeps{
		createDeps: defaultCreateDeps(),
		now:        time.Now,
		newRunID:   generateRunID,
		mkdirAll:   os.MkdirAll,
		openFile: func(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
			return os.OpenFile(path, flag, perm)
		},
		executable: os.Executable,
		runCommand: func(ctx context.Context, name string, args []string, stdout io.Writer, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
		},
	}
}

func runWebRTCP2PWithDeps(ctx context.Context, opts WebRTCP2POptions, deps webRTCP2PDeps) (*WebRTCP2PResult, error) {
	opts = normalizeWebRTCP2POptions(opts)
	deps = fillWebRTCP2PDeps(deps)
	if err := validateWebRTCP2POptions(ctx, opts, deps.createDeps); err != nil {
		return nil, err
	}

	startedAt := deps.now().UTC()
	runID, err := deps.newRunID(startedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate run id: %w", err)
	}
	runDir := filepath.Join(opts.RunsDir, runID)
	if err := deps.mkdirAll(filepath.Join(runDir, "signal"), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create WebRTC run directory %s: %w", runDir, err)
	}
	latestDir, err := updateLatestRunSymlink(opts.RunsDir, runID)
	if err != nil {
		return nil, err
	}

	logger, err := newEventLogger(opts.RunsDir, runID, scenarioRunDeps{
		now:      deps.now,
		newRunID: deps.newRunID,
		mkdirAll: deps.mkdirAll,
		openFile: deps.openFile,
	})
	if err != nil {
		return nil, err
	}
	result := &WebRTCP2PResult{
		RunID:      runID,
		RunDir:     runDir,
		LatestDir:  latestDir,
		EventsPath: logger.eventsPath,
		StatsPath:  filepath.Join(runDir, "stats.jsonl"),
	}

	record := func(phase string, status string, opErr error) error {
		return logger.write(EventRecord{
			RunID:    runID,
			Event:    webRTCP2PEventName,
			Scenario: "webrtc-p2p",
			Phase:    phase,
			Time:     deps.now().UTC().Format(time.RFC3339Nano),
			Action:   "run",
			Status:   status,
			Error:    errorString(opErr),
		})
	}

	var runErr error
	if err := record("webrtc_start", "ok", nil); err != nil {
		return result, errors.Join(err, logger.close())
	}

	executable, err := deps.executable()
	if err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("failed to resolve current executable: %w", err))
	} else {
		if err := runWebRTCPeerProcesses(ctx, opts, runID, runDir, executable, deps, func() error {
			return record("connected", "ok", nil)
		}); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}

	mergeErr := mergeStatsLogs(result.StatsPath, []string{
		filepath.Join(runDir, peerStatsFilename(opts.NodeA)),
		filepath.Join(runDir, peerStatsFilename(opts.NodeB)),
	})
	if mergeErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("failed to merge peer stats logs: %w", mergeErr))
	}
	if err := record("stats_complete", statusForError(mergeErr), mergeErr); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := record("cleanup", "ok", nil); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := logger.close(); err != nil {
		runErr = errors.Join(runErr, err)
	}

	return result, runErr
}

func normalizeWebRTCP2POptions(opts WebRTCP2POptions) WebRTCP2POptions {
	opts.RunsDir = strings.TrimSpace(opts.RunsDir)
	opts.NodeA = strings.TrimSpace(opts.NodeA)
	opts.NodeB = strings.TrimSpace(opts.NodeB)
	if opts.RunsDir == "" {
		opts.RunsDir = defaultRunsDir
	}
	if opts.NodeA == "" {
		opts.NodeA = defaultWebRTCNodeA
	}
	if opts.NodeB == "" {
		opts.NodeB = defaultWebRTCNodeB
	}
	if opts.Duration <= 0 {
		opts.Duration = defaultWebRTCDuration
	}
	if opts.StatsInterval <= 0 {
		opts.StatsInterval = defaultWebRTCStatsInterval
	}
	return opts
}

func fillWebRTCP2PDeps(deps webRTCP2PDeps) webRTCP2PDeps {
	deps.createDeps = fillCreateDeps(deps.createDeps)
	d := defaultWebRTCP2PDeps()
	if deps.now == nil {
		deps.now = d.now
	}
	if deps.newRunID == nil {
		deps.newRunID = d.newRunID
	}
	if deps.mkdirAll == nil {
		deps.mkdirAll = d.mkdirAll
	}
	if deps.openFile == nil {
		deps.openFile = d.openFile
	}
	if deps.executable == nil {
		deps.executable = d.executable
	}
	if deps.runCommand == nil {
		deps.runCommand = d.runCommand
	}
	return deps
}

func validateWebRTCP2POptions(ctx context.Context, opts WebRTCP2POptions, deps createDeps) error {
	if opts.NodeA == opts.NodeB {
		return errors.New("node-a and node-b must be different")
	}
	if opts.Duration <= 0 {
		return errors.New("duration must be positive")
	}
	if opts.StatsInterval <= 0 {
		return errors.New("stats interval must be positive")
	}
	if deps.goos != "linux" {
		return fmt.Errorf("lab webrtc p2p is supported only on linux: got %s", deps.goos)
	}
	if !deps.isRoot() {
		return errors.New("lab webrtc p2p requires root privileges")
	}
	if _, err := deps.findPath("ip"); err != nil {
		return fmt.Errorf("required command %q not found: %w", "ip", err)
	}
	if deps.loadState == nil {
		return errors.New("lab state loader is not configured")
	}
	state, err := deps.loadState(ctx)
	if errors.Is(err, ErrStateNotFound) {
		return errors.New("lab state not found: run `rtc-emulator lab create --nodes 2` first")
	}
	if err != nil {
		return fmt.Errorf("failed to load lab state: %w", err)
	}
	for _, node := range []string{opts.NodeA, opts.NodeB} {
		if !containsString(state.Nodes, node) {
			return fmt.Errorf("node %q is not managed by current lab", node)
		}
	}
	namespaces, err := listNamespaces(ctx, deps.exec)
	if err != nil {
		return err
	}
	for _, node := range []string{opts.NodeA, opts.NodeB} {
		if !containsString(namespaces, node) {
			return fmt.Errorf("node %q namespace not found", node)
		}
	}
	return nil
}

func runWebRTCPeerProcesses(
	ctx context.Context,
	opts WebRTCP2POptions,
	runID string,
	runDir string,
	executable string,
	deps webRTCP2PDeps,
	onConnected func() error,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	waitPeers := launchWebRTCPeerProcesses(ctx, opts, runID, runDir, executable, deps, cancel)

	readyErr := waitForWebRTCPeerReadiness(ctx, runDir, []string{opts.NodeA, opts.NodeB}, webRTCSignalTimeout)
	if readyErr != nil {
		cancel()
	} else if onConnected != nil {
		readyErr = onConnected()
		if readyErr != nil {
			cancel()
		}
	}

	return errors.Join(readyErr, waitPeers())
}

func startWebRTCPeerProcesses(
	ctx context.Context,
	opts WebRTCP2POptions,
	runID string,
	runDir string,
	executable string,
	deps webRTCP2PDeps,
	cancel context.CancelFunc,
) func() error {
	if deps.runCommand == nil {
		deps.runCommand = defaultWebRTCP2PDeps().runCommand
	}

	return launchWebRTCPeerProcesses(ctx, opts, runID, runDir, executable, deps, cancel)
}

type webRTCPeerProc struct {
	node   string
	role   string
	peer   string
	err    error
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func launchWebRTCPeerProcesses(
	ctx context.Context,
	opts WebRTCP2POptions,
	runID string,
	runDir string,
	executable string,
	deps webRTCP2PDeps,
	cancel context.CancelFunc,
) func() error {
	procs := []*webRTCPeerProc{
		{node: opts.NodeB, role: webRTCPeerRoleAnswerer, peer: opts.NodeA},
		{node: opts.NodeA, role: webRTCPeerRoleOfferer, peer: opts.NodeB},
	}
	var wg sync.WaitGroup
	for _, proc := range procs {
		proc := proc
		wg.Add(1)
		go func() {
			defer wg.Done()
			args := webRTCPeerNetNSArgs(proc.node, executable, WebRTCPeerOptions{
				Role:          proc.role,
				RunID:         runID,
				RunDir:        runDir,
				Node:          proc.node,
				Peer:          proc.peer,
				Duration:      opts.Duration,
				StatsInterval: opts.StatsInterval,
			})
			if err := deps.runCommand(ctx, "ip", args, &proc.stdout, &proc.stderr); err != nil {
				if ctx.Err() == nil {
					proc.err = err
					if cancel != nil {
						cancel()
					}
				}
			}
		}()
	}

	return func() error {
		wg.Wait()
		var runErr error
		for _, proc := range procs {
			if proc.err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("webrtc peer %s/%s failed: %w stdout=%q stderr=%q", proc.node, proc.role, proc.err, proc.stdout.String(), proc.stderr.String()))
			}
		}
		return runErr
	}
}

func webRTCPeerNetNSArgs(node string, executable string, opts WebRTCPeerOptions) []string {
	return []string{
		"netns", "exec", node,
		executable,
		"lab", "webrtc", "peer",
		"--role", opts.Role,
		"--run-id", opts.RunID,
		"--run-dir", opts.RunDir,
		"--node", opts.Node,
		"--peer", opts.Peer,
		"--duration", opts.Duration.String(),
		"--stats-interval", opts.StatsInterval.String(),
	}
}

func peerStatsFilename(node string) string {
	return "stats." + node + ".jsonl"
}

func updateLatestRunSymlink(runsDir string, runID string) (string, error) {
	latestPath := filepath.Join(runsDir, latestRunSymlinkName)
	info, err := os.Lstat(latestPath)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("failed to update latest run link %s: path is a directory", latestPath)
		}
		if err := os.Remove(latestPath); err != nil {
			return "", fmt.Errorf("failed to remove existing latest run link %s: %w", latestPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("failed to inspect latest run link %s: %w", latestPath, err)
	}

	if err := os.Symlink(runID, latestPath); err != nil {
		return "", fmt.Errorf("failed to create latest run link %s: %w", latestPath, err)
	}
	return latestPath, nil
}

func waitForWebRTCPeerReadiness(ctx context.Context, runDir string, nodes []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ready := make(map[string]bool, len(nodes))
	for {
		for _, node := range nodes {
			if ready[node] {
				continue
			}
			if _, err := os.Stat(peerConnectedMarkerPath(runDir, node)); err == nil {
				ready[node] = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to read WebRTC connected marker for %s: %w", node, err)
			}
		}
		if len(ready) == len(nodes) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for WebRTC peer readiness: %w", ctx.Err())
		case <-time.After(webRTCSignalPollInterval):
		}
	}
}

func signalDir(runDir string) string {
	return filepath.Join(runDir, "signal")
}

func offerPath(runDir string) string {
	return filepath.Join(signalDir(runDir), "offer.json")
}

func answerPath(runDir string) string {
	return filepath.Join(signalDir(runDir), "answer.json")
}

func peerConnectedMarkerPath(runDir string, node string) string {
	return filepath.Join(signalDir(runDir), "connected."+node+".json")
}
