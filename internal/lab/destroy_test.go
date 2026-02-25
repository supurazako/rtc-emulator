package lab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDestroyWithDeps_RequireLinux(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := destroyWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "darwin",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "only on linux") {
		t.Fatalf("expected linux-only error, got: %v", err)
	}
}

func TestDestroyWithDeps_StateNotFoundFallback(t *testing.T) {
	checkCount := map[string]int{}
	ex := &fakeExecutor{}
	ex.runFn = func(name string, args ...string) error {
		cmd := callKey(name, args...)
		switch cmd {
		case "ip link show rtcemu0":
			return nil
		}
		if name == "iptables" && containsArg(args, "-C") {
			checkCount[cmd]++
			if checkCount[cmd] > 1 {
				return errors.New("Bad rule (does a matching rule exist in that chain?)")
			}
			return nil
		}
		return nil
	}
	ex.outputFn = func(name string, args ...string) (string, error) {
		if callKey(name, args...) == "ip -o link show master rtcemu0" {
			return "7: br-node1@if8: <BROADCAST> mtu 1500 master rtcemu0 state UP mode DEFAULT group default\n8: cni123@if2: <BROADCAST> mtu 1500 master rtcemu0 state UP mode DEFAULT group default\n", nil
		}
		return "", nil
	}

	got, err := destroyWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return nil, ErrStateNotFound
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.StateMissingFallback {
		t.Fatalf("expected state missing fallback")
	}
	if !got.BridgeDeleted {
		t.Fatalf("expected bridge cleanup in fallback")
	}
	if hasCall(ex.calls, "ip netns del node1") {
		t.Fatalf("fallback must not delete namespaces")
	}
	if !hasCall(ex.calls, "ip link del br-node1") {
		t.Fatalf("expected managed bridge peer deletion")
	}
	if hasCall(ex.calls, "ip link del cni123") {
		t.Fatalf("must not delete non-managed bridge member")
	}
}

func TestDestroyWithDeps_Success(t *testing.T) {
	checkCount := map[string]int{}
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			cmd := callKey(name, args...)
			switch cmd {
			case "ip link show rtcemu0":
				return nil
			case "ip netns del node9":
				return errors.New("No such file or directory")
			}

			if name == "iptables" && containsArg(args, "-C") {
				checkCount[cmd]++
				if checkCount[cmd] > 1 {
					return errors.New("Bad rule (does a matching rule exist in that chain?)")
				}
				return nil
			}
			return nil
		},
	}

	deletedState := false
	got, err := destroyWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{
				Bridge:          bridgeName,
				Nodes:           []string{"node1", "node9"},
				Rules:           managedIPTablesRules(),
				IPForwardBefore: "0",
			}, nil
		},
		deleteState: func(context.Context) error {
			deletedState = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.BridgeDeleted {
		t.Fatalf("expected bridge deleted")
	}
	if len(got.NodesDeleted) != 1 || got.NodesDeleted[0] != "node1" {
		t.Fatalf("unexpected nodes deleted: %+v", got.NodesDeleted)
	}
	if !deletedState {
		t.Fatalf("expected state file deletion")
	}
	if !got.IPForwardRestored || got.IPForwardRestoreValue != "0" {
		t.Fatalf("expected ip forward restore result, got %+v", got)
	}
	if !hasCall(ex.calls, "sysctl -w net.ipv4.ip_forward=0") {
		t.Fatalf("expected ip_forward restore command")
	}
	if !hasCall(ex.calls, "ip netns del node1") {
		t.Fatalf("expected node1 deletion")
	}
}

func TestDestroyWithDeps_BridgeCheckError(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			if callKey(name, args...) == "ip link show rtcemu0" {
				return errors.New("operation not permitted")
			}
			return nil
		},
	}

	_, err := destroyWithDeps(context.Background(), createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
		loadState: func(context.Context) (*LabState, error) {
			return &LabState{
				Bridge:          bridgeName,
				Nodes:           []string{},
				Rules:           []IPTablesRule{},
				IPForwardBefore: "",
			}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to check bridge existence") {
		t.Fatalf("expected strict bridge check error, got: %v", err)
	}
}
