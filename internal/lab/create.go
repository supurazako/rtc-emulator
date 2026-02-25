package lab

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	bridgeName = "rtcemu0"
	bridgeCIDR = "10.200.0.1/24"
	bridgeIP   = "10.200.0.1"
	subnetCIDR = "10.200.0.0/24"
)

var managedNodePattern = regexp.MustCompile(`^node[1-9][0-9]*$`)

type CreateOptions struct {
	Nodes int
}

type Node struct {
	Name string
	IP   string
}

type CreateResult struct {
	Bridge            string
	Nodes             []Node
	InternetReachable bool
}

type createDeps struct {
	exec        Executor
	goos        string
	isRoot      func() bool
	findPath    func(string) (string, error)
	loadState   func(context.Context) (*LabState, error)
	saveState   func(context.Context, *LabState) error
	deleteState func(context.Context) error
}

func defaultCreateDeps() createDeps {
	return createDeps{
		exec:     osExecutor{},
		goos:     runtime.GOOS,
		isRoot:   func() bool { return os.Geteuid() == 0 },
		findPath: exec.LookPath,
		loadState: func(ctx context.Context) (*LabState, error) {
			return loadState(ctx, defaultStatePath)
		},
		saveState: func(ctx context.Context, state *LabState) error {
			return saveStateAtomic(ctx, defaultStatePath, state)
		},
		deleteState: func(ctx context.Context) error {
			return deleteStateFile(ctx, defaultStatePath)
		},
	}
}

func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	return createWithDeps(ctx, opts, defaultCreateDeps())
}

func createWithDeps(ctx context.Context, opts CreateOptions, deps createDeps) (*CreateResult, error) {
	deps = fillCreateDeps(deps)

	if opts.Nodes < 1 || opts.Nodes > 250 {
		return nil, fmt.Errorf("nodes must be between 1 and 250: got %d", opts.Nodes)
	}
	if deps.goos != "linux" {
		return nil, fmt.Errorf("lab create is supported only on linux: got %s", deps.goos)
	}
	if !deps.isRoot() {
		return nil, errors.New("lab create requires root privileges")
	}
	for _, cmd := range []string{"ip", "sysctl", "iptables", "ping"} {
		if _, err := deps.findPath(cmd); err != nil {
			return nil, fmt.Errorf("required command %q not found: %w", cmd, err)
		}
	}

	if deps.loadState != nil {
		if _, err := deps.loadState(ctx); err == nil {
			return nil, errors.New("existing lab detected (state file exists): run `rtc-emulator lab destroy` and retry")
		} else if !errors.Is(err, ErrStateNotFound) {
			return nil, fmt.Errorf("failed to check lab state: %w", err)
		}
	}

	bridgeExists, err := bridgeExists(ctx, deps.exec, bridgeName)
	if err != nil {
		return nil, err
	}
	if bridgeExists {
		return nil, errors.New("existing lab detected (bridge rtcemu0 already exists): run `rtc-emulator lab destroy` and retry")
	}

	nsList, err := listNamespaces(ctx, deps.exec)
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList {
		if isManagedNodeName(ns) {
			return nil, errors.New("existing lab detected (node namespace already exists): run `rtc-emulator lab destroy` and retry")
		}
	}

	cleanups := make([]func(context.Context), 0, opts.Nodes+8)
	rollback := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	}

	if err := deps.exec.Run(ctx, "ip", "link", "add", bridgeName, "type", "bridge"); err != nil {
		return nil, err
	}
	cleanups = append(cleanups, func(ctx context.Context) {
		_ = deps.exec.Run(ctx, "ip", "link", "del", bridgeName)
	})

	if err := deps.exec.Run(ctx, "ip", "addr", "add", bridgeCIDR, "dev", bridgeName); err != nil {
		rollback()
		return nil, err
	}
	if err := deps.exec.Run(ctx, "ip", "link", "set", bridgeName, "up"); err != nil {
		rollback()
		return nil, err
	}

	ipForwardBefore, err := readIPForward(ctx, deps.exec)
	if err != nil {
		rollback()
		return nil, err
	}
	if err := deps.exec.Run(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		rollback()
		return nil, err
	}
	cleanups = append(cleanups, func(ctx context.Context) {
		_ = restoreIPForward(ctx, deps.exec, ipForwardBefore)
	})

	rules := managedIPTablesRules()
	if err := ensureIPTablesRule(ctx, deps.exec, rules[0],
		&cleanups,
	); err != nil {
		rollback()
		return nil, err
	}
	if err := ensureIPTablesRule(ctx, deps.exec, rules[1],
		&cleanups,
	); err != nil {
		rollback()
		return nil, err
	}
	if err := ensureIPTablesRule(ctx, deps.exec, rules[2],
		&cleanups,
	); err != nil {
		rollback()
		return nil, err
	}

	result := &CreateResult{
		Bridge: bridgeName,
		Nodes:  make([]Node, 0, opts.Nodes),
	}

	for i := 1; i <= opts.Nodes; i++ {
		nodeName := "node" + strconv.Itoa(i)
		nodeIP := "10.200.0." + strconv.Itoa(i+1)
		peerHost := "br-" + nodeName
		nsIface := "veth-" + nodeName

		if err := deps.exec.Run(ctx, "ip", "netns", "add", nodeName); err != nil {
			rollback()
			return nil, err
		}
		cleanups = append(cleanups, func(ctx context.Context) {
			_ = deps.exec.Run(ctx, "ip", "netns", "del", nodeName)
		})

		if err := deps.exec.Run(ctx, "ip", "link", "add", nsIface, "type", "veth", "peer", "name", peerHost); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "link", "set", nsIface, "netns", nodeName); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "link", "set", peerHost, "master", bridgeName); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "link", "set", peerHost, "up"); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ip", "link", "set", "lo", "up"); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ip", "link", "set", nsIface, "name", "eth0"); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ip", "addr", "add", nodeIP+"/24", "dev", "eth0"); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ip", "link", "set", "eth0", "up"); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ip", "route", "add", "default", "via", bridgeIP); err != nil {
			rollback()
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "netns", "exec", nodeName, "ping", "-c", "1", "-W", "1", bridgeIP); err != nil {
			rollback()
			return nil, fmt.Errorf("connectivity check failed for %s -> %s: %w", nodeName, bridgeIP, err)
		}

		result.Nodes = append(result.Nodes, Node{Name: nodeName, IP: nodeIP})
	}

	if err := deps.exec.Run(ctx, "ip", "netns", "exec", "node1", "ping", "-c", "1", "-W", "1", "1.1.1.1"); err == nil {
		result.InternetReachable = true
	}

	if deps.saveState != nil {
		state := &LabState{
			Bridge:          bridgeName,
			Subnet:          subnetCIDR,
			Nodes:           make([]string, 0, len(result.Nodes)),
			Rules:           rules,
			IPForwardBefore: ipForwardBefore,
		}
		for _, n := range result.Nodes {
			state.Nodes = append(state.Nodes, n.Name)
		}
		if err := deps.saveState(ctx, state); err != nil {
			rollback()
			return nil, fmt.Errorf("failed to persist lab state: %w", err)
		}
	}

	return result, nil
}

func fillCreateDeps(deps createDeps) createDeps {
	d := defaultCreateDeps()
	if deps.exec == nil {
		deps.exec = d.exec
	}
	if deps.goos == "" {
		deps.goos = d.goos
	}
	if deps.isRoot == nil {
		deps.isRoot = d.isRoot
	}
	if deps.findPath == nil {
		deps.findPath = d.findPath
	}
	return deps
}

func bridgeExists(ctx context.Context, exec Executor, name string) (bool, error) {
	err := exec.Run(ctx, "ip", "link", "show", name)
	if err == nil {
		return true, nil
	}
	if isBridgeNotFoundError(err, name) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check bridge existence: %w", err)
}

func isManagedNodeName(ns string) bool {
	return managedNodePattern.MatchString(ns)
}

func listNamespaces(ctx context.Context, exec Executor) ([]string, error) {
	out, err := exec.Output(ctx, "ip", "netns", "list")
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	lines := strings.Split(out, "\n")
	ns := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ns = append(ns, fields[0])
	}
	return ns, nil
}

func ensureIPTablesRule(
	ctx context.Context,
	exec Executor,
	rule IPTablesRule,
	cleanups *[]func(context.Context),
) error {
	if err := exec.Run(ctx, "iptables", rule.CheckArgs...); err == nil {
		return nil
	} else if !isIPTablesRuleNotFoundError(err) {
		return fmt.Errorf("failed to check iptables rule %v: %w", rule.CheckArgs, err)
	}
	if err := exec.Run(ctx, "iptables", rule.AddArgs...); err != nil {
		return err
	}
	*cleanups = append(*cleanups, func(ctx context.Context) {
		_ = exec.Run(ctx, "iptables", rule.DelArgs...)
	})
	return nil
}

func managedIPTablesRules() []IPTablesRule {
	return []IPTablesRule{
		{
			CheckArgs: []string{"-t", "nat", "-C", "POSTROUTING", "-s", subnetCIDR, "!", "-o", bridgeName, "-j", "MASQUERADE"},
			AddArgs:   []string{"-t", "nat", "-A", "POSTROUTING", "-s", subnetCIDR, "!", "-o", bridgeName, "-j", "MASQUERADE"},
			DelArgs:   []string{"-t", "nat", "-D", "POSTROUTING", "-s", subnetCIDR, "!", "-o", bridgeName, "-j", "MASQUERADE"},
		},
		{
			CheckArgs: []string{"-C", "FORWARD", "-i", bridgeName, "-j", "ACCEPT"},
			AddArgs:   []string{"-A", "FORWARD", "-i", bridgeName, "-j", "ACCEPT"},
			DelArgs:   []string{"-D", "FORWARD", "-i", bridgeName, "-j", "ACCEPT"},
		},
		{
			CheckArgs: []string{"-C", "FORWARD", "-o", bridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			AddArgs:   []string{"-A", "FORWARD", "-o", bridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			DelArgs:   []string{"-D", "FORWARD", "-o", bridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		},
	}
}

func isBridgeNotFoundError(err error, bridge string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, `device "`+strings.ToLower(bridge)+`" does not exist`) ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "cannot find device")
}

func isIPTablesRuleNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "bad rule") ||
		strings.Contains(msg, "no chain/target/match")
}

func isNamespaceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory")
}

func readIPForward(ctx context.Context, exec Executor) (string, error) {
	out, err := exec.Output(ctx, "sysctl", "-n", "net.ipv4.ip_forward")
	if err != nil {
		return "", fmt.Errorf("failed to read net.ipv4.ip_forward: %w", err)
	}
	v := strings.TrimSpace(out)
	if v != "0" && v != "1" {
		return "", fmt.Errorf("unexpected net.ipv4.ip_forward value: %q", v)
	}
	return v, nil
}

func restoreIPForward(ctx context.Context, exec Executor, value string) error {
	if value == "" {
		return nil
	}
	if value != "0" && value != "1" {
		return fmt.Errorf("invalid net.ipv4.ip_forward restore value: %q", value)
	}
	if err := exec.Run(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+value); err != nil {
		return fmt.Errorf("failed to restore net.ipv4.ip_forward=%s: %w", value, err)
	}
	return nil
}
