package lab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeExecutor struct {
	runFn    func(name string, args ...string) error
	outputFn func(name string, args ...string) (string, error)
	calls    []string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, callKey(name, args...))
	if f.runFn == nil {
		return nil
	}
	return f.runFn(name, args...)
}

func (f *fakeExecutor) Output(_ context.Context, name string, args ...string) (string, error) {
	f.calls = append(f.calls, callKey(name, args...))
	if f.outputFn == nil {
		return "", nil
	}
	return f.outputFn(name, args...)
}

func callKey(name string, args ...string) string {
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasCall(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

func TestCreateWithDeps_ValidateNodes(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 0}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "between 1 and 250") {
		t.Fatalf("expected nodes validation error, got: %v", err)
	}
}

func TestCreateWithDeps_RequireLinux(t *testing.T) {
	ex := &fakeExecutor{}
	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "darwin",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "only on linux") {
		t.Fatalf("expected linux-only error, got: %v", err)
	}
}

func TestCreateWithDeps_ExistingBridgeFails(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			if callKey(name, args...) == "ip link show rtcemu0" {
				return nil
			}
			return nil
		},
	}

	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "existing lab detected") {
		t.Fatalf("expected existing-lab error, got: %v", err)
	}
}

func TestCreateWithDeps_BridgeCheckError(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			if callKey(name, args...) == "ip link show rtcemu0" {
				return errors.New("operation not permitted")
			}
			return nil
		},
	}

	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "failed to check bridge existence") {
		t.Fatalf("expected strict bridge check error, got: %v", err)
	}
}

func TestCreateWithDeps_Success(t *testing.T) {
	notFoundErr := errors.New("Device \"rtcemu0\" does not exist")
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			cmd := callKey(name, args...)
			if cmd == "ip link show rtcemu0" {
				return notFoundErr
			}
			if name == "iptables" && containsArg(args, "-C") {
				return errors.New("Bad rule (does a matching rule exist in that chain?)")
			}
			if cmd == "ip netns exec node1 ping -c 1 -W 1 1.1.1.1" {
				return errors.New("internet unreachable")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "", nil
			}
			if callKey(name, args...) == "sysctl -n net.ipv4.ip_forward" {
				return "0\n", nil
			}
			return "", nil
		},
	}

	got, err := createWithDeps(context.Background(), CreateOptions{Nodes: 2}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bridge != bridgeName {
		t.Fatalf("expected bridge %s, got %s", bridgeName, got.Bridge)
	}
	if len(got.Nodes) != 2 || got.Nodes[0].Name != "node1" || got.Nodes[1].Name != "node2" {
		t.Fatalf("unexpected nodes: %+v", got.Nodes)
	}
	if got.InternetReachable {
		t.Fatalf("internet should be false when internet ping fails")
	}
	if !hasCall(ex.calls, "ip netns add node1") {
		t.Fatalf("missing command: ip netns add node1")
	}
	if !hasCall(ex.calls, "ip netns exec node2 ping -c 1 -W 1 10.200.0.1") {
		t.Fatalf("missing bridge connectivity check for node2")
	}
}

func TestCreateWithDeps_RollbackOnFailure(t *testing.T) {
	notFoundErr := errors.New("Device \"rtcemu0\" does not exist")
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			cmd := callKey(name, args...)
			if cmd == "ip link show rtcemu0" {
				return notFoundErr
			}
			if name == "iptables" && containsArg(args, "-C") {
				return errors.New("Bad rule (does a matching rule exist in that chain?)")
			}
			if cmd == "ip netns exec node1 ping -c 1 -W 1 10.200.0.1" {
				return errors.New("bridge unreachable")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "", nil
			}
			if callKey(name, args...) == "sysctl -n net.ipv4.ip_forward" {
				return "0\n", nil
			}
			return "", nil
		},
	}

	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "connectivity check failed") {
		t.Fatalf("expected connectivity failure, got: %v", err)
	}

	if !hasCall(ex.calls, "ip netns del node1") {
		t.Fatalf("expected namespace cleanup command")
	}
	if !hasCall(ex.calls, "ip link del rtcemu0") {
		t.Fatalf("expected bridge cleanup command")
	}
	if !hasCall(ex.calls, "sysctl -w net.ipv4.ip_forward=0") {
		t.Fatalf("expected ip_forward restore command")
	}

	foundRuleCleanup := false
	for _, c := range ex.calls {
		if strings.HasPrefix(c, "iptables -D") || strings.HasPrefix(c, "iptables -t nat -D") {
			foundRuleCleanup = true
			break
		}
	}
	if !foundRuleCleanup {
		t.Fatalf("expected iptables cleanup command")
	}
}

func TestCreateWithDeps_IPTablesCheckError(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			cmd := callKey(name, args...)
			if cmd == "ip link show rtcemu0" {
				return errors.New("Device \"rtcemu0\" does not exist")
			}
			if name == "iptables" && containsArg(args, "-C") {
				return errors.New("permission denied")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "", nil
			}
			if callKey(name, args...) == "sysctl -n net.ipv4.ip_forward" {
				return "0\n", nil
			}
			return "", nil
		},
	}

	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "failed to check iptables rule") {
		t.Fatalf("expected strict iptables check error, got: %v", err)
	}
}

func TestCreateWithDeps_NodeNamespaceFilter(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			if callKey(name, args...) == "ip link show rtcemu0" {
				return errors.New("Device \"rtcemu0\" does not exist")
			}
			return errors.New("stop after checks")
		},
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node-exporter\n", nil
			}
			if callKey(name, args...) == "sysctl -n net.ipv4.ip_forward" {
				return "0\n", nil
			}
			return "", nil
		},
	}
	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || strings.Contains(err.Error(), "existing lab detected (node namespace") {
		t.Fatalf("unexpected node namespace conflict: %v", err)
	}
}

func TestCreateWithDeps_ManagedNodeNamespaceDetected(t *testing.T) {
	ex := &fakeExecutor{
		runFn: func(name string, args ...string) error {
			if callKey(name, args...) == "ip link show rtcemu0" {
				return errors.New("Device \"rtcemu0\" does not exist")
			}
			return nil
		},
		outputFn: func(name string, args ...string) (string, error) {
			if callKey(name, args...) == "ip netns list" {
				return "node1\n", nil
			}
			return "", nil
		},
	}
	_, err := createWithDeps(context.Background(), CreateOptions{Nodes: 1}, createDeps{
		exec:     ex,
		goos:     "linux",
		isRoot:   func() bool { return true },
		findPath: func(string) (string, error) { return "/bin/x", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "existing lab detected (node namespace") {
		t.Fatalf("expected node namespace detection error, got: %v", err)
	}
}
