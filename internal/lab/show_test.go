package lab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestShowWithDeps_RequireLinux(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := showWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "darwin",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "only on linux") {
		t.Fatalf("expected linux-only error, got: %v", err)
	}
}

func TestShowWithDeps_RequireRoot(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := showWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return false },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "requires root privileges") {
		t.Fatalf("expected root requirement error, got: %v", err)
	}
}

func TestShowWithDeps_RequireCommands(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := showWithDeps(context.Background(), createDeps{
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
		t.Fatalf("expected command not found error, got: %v", err)
	}
}

func TestShowWithDeps_StateNotFound(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := showWithDeps(context.Background(), createDeps{
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

func TestShowWithDeps_NodeNamespaceNotFound(t *testing.T) {
	ex := &fakeExecutor{
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node2\n", nil
			}
			return "", nil
		},
	}

	_, err := showWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{Bridge: bridgeName, Subnet: subnetCIDR, Nodes: []string{"node1"}}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "namespace not found") {
		t.Fatalf("expected namespace-not-found error, got: %v", err)
	}
}

func TestShowWithDeps_SuccessNetem(t *testing.T) {
	ex := &fakeExecutor{
		outputFn: func(name string, args ...string) (string, error) {
			switch callKey(name, args...) {
			case "ip netns list":
				return "node1\nnode2\n", nil
			case "ip netns exec node1 tc qdisc show dev eth0":
				return "qdisc netem 8001: root refcnt 2 limit 1000 delay 50ms 10ms loss 1% rate 2mbit\n", nil
			case "ip netns exec node2 tc qdisc show dev eth0":
				return "qdisc fq_codel 0: root refcnt 2\n", nil
			default:
				return "", nil
			}
		},
	}

	got, err := showWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{
				Bridge: bridgeName,
				Subnet: subnetCIDR,
				Nodes:  []string{"node1", "node2"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bridge != bridgeName || got.Subnet != subnetCIDR {
		t.Fatalf("unexpected bridge/subnet: %+v", got)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("unexpected node count: %+v", got.Nodes)
	}

	n1 := got.Nodes[0]
	if n1.Name != "node1" || n1.Interface != "eth0" {
		t.Fatalf("unexpected node1 identity: %+v", n1)
	}
	if n1.Delay != "50ms" || n1.Jitter != "10ms" || n1.Loss != "1%" || n1.BW != "2mbit" {
		t.Fatalf("unexpected node1 parsed values: %+v", n1)
	}
	if n1.RawQDisc == "none" {
		t.Fatalf("expected raw netem line for node1")
	}

	n2 := got.Nodes[1]
	if n2.Name != "node2" || n2.Interface != "eth0" {
		t.Fatalf("unexpected node2 identity: %+v", n2)
	}
	if n2.Delay != "" || n2.Jitter != "" || n2.Loss != "" || n2.BW != "" || n2.RawQDisc != "none" {
		t.Fatalf("expected none values for node2, got %+v", n2)
	}
}

func TestParseNetemValues_DelayWithoutJitter(t *testing.T) {
	delay, jitter, loss, bw := parseNetemValues("qdisc netem 8001: root refcnt 2 delay 120ms loss 2% rate 800kbit")
	if delay != "120ms" || jitter != "" || loss != "2%" || bw != "800kbit" {
		t.Fatalf("unexpected parsed values: delay=%s jitter=%s loss=%s bw=%s", delay, jitter, loss, bw)
	}
}
