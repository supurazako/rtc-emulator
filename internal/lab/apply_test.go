package lab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestApplyWithDeps_RequireLinux(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, createDeps{
		exec:     ex,
		goos:     "darwin",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "only on linux") {
		t.Fatalf("expected linux-only error, got: %v", err)
	}
}

func TestApplyWithDeps_RequireRoot(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return false },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "requires root privileges") {
		t.Fatalf("expected root requirement error, got: %v", err)
	}
}

func TestApplyWithDeps_RequireCommands(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, createDeps{
		exec:   ex,
		goos:   "linux",
		isRoot: func() bool { return true },
		findPath: func(name string) (string, error) {
			if name == "tc" {
				return "", errors.New("not found")
			}
			return "/bin/x", nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "required command \"tc\" not found") {
		t.Fatalf("expected tc not found error, got: %v", err)
	}
}

func TestApplyWithDeps_RequireAnyFlag(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node: "node1",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "at least one impairment flag is required") {
		t.Fatalf("expected impairment flag requirement error, got: %v", err)
	}
}

func TestApplyWithDeps_RequireDelayForJitter(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:   "node1",
		Jitter: "10ms",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "jitter requires delay") {
		t.Fatalf("expected jitter validation error, got: %v", err)
	}
}

func TestApplyWithDeps_StateNotFound(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return nil, ErrStateNotFound
		},
	})
	if err == nil || !strings.Contains(err.Error(), "lab state not found") {
		t.Fatalf("expected state-not-found error, got: %v", err)
	}
}

func TestApplyWithDeps_NodeNotManaged(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node9",
		Delay: "50ms",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{Nodes: []string{"node1", "node2"}}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "is not managed by current lab") {
		t.Fatalf("expected unmanaged-node error, got: %v", err)
	}
}

func TestApplyWithDeps_NodeNamespaceNotFound(t *testing.T) {
	ex := &fakeExecutor{
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node2\n", nil
			}
			return "", nil
		},
	}
	_, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{Nodes: []string{"node1"}}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "namespace not found") {
		t.Fatalf("expected namespace-not-found error, got: %v", err)
	}
}

func TestApplyWithDeps_Success(t *testing.T) {
	ex := &fakeExecutor{
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node1\n", nil
			}
			return "", nil
		},
	}
	got, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:   "node1",
		Delay:  "50ms",
		Jitter: "10ms",
		Loss:   "1%",
		BW:     "2mbit",
	}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{Nodes: []string{"node1"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Node != "node1" || got.Delay != "50ms" || got.Jitter != "10ms" || got.Loss != "1%" || got.BW != "2mbit" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !hasCall(ex.calls, "ip netns exec node1 tc qdisc replace dev eth0 root netem delay 50ms 10ms loss 1% rate 2mbit") {
		t.Fatalf("missing replace command, calls=%v", ex.calls)
	}
}

func TestApplyWithDeps_ReapplyUsesReplace(t *testing.T) {
	ex := &fakeExecutor{
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node1\n", nil
			}
			return "", nil
		},
	}
	deps := createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{Nodes: []string{"node1"}}, nil
		},
	}

	if _, err := applyWithDeps(context.Background(), ApplyOptions{
		Node:  "node1",
		Delay: "50ms",
	}, deps); err != nil {
		t.Fatalf("unexpected first apply error: %v", err)
	}
	if _, err := applyWithDeps(context.Background(), ApplyOptions{
		Node: "node1",
		Loss: "2%",
	}, deps); err != nil {
		t.Fatalf("unexpected second apply error: %v", err)
	}

	if !hasCall(ex.calls, "ip netns exec node1 tc qdisc replace dev eth0 root netem delay 50ms") {
		t.Fatalf("missing first replace command, calls=%v", ex.calls)
	}
	if !hasCall(ex.calls, "ip netns exec node1 tc qdisc replace dev eth0 root netem loss 2%") {
		t.Fatalf("missing second replace command, calls=%v", ex.calls)
	}
}
