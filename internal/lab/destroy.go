package lab

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type DestroyResult struct {
	BridgeDeleted         bool
	NodesDeleted          []string
	StateMissingFallback  bool
	IPForwardRestored     bool
	IPForwardRestoreValue string
}

func Destroy(ctx context.Context) (*DestroyResult, error) {
	return destroyWithDeps(ctx, defaultCreateDeps())
}

func destroyWithDeps(ctx context.Context, deps createDeps) (*DestroyResult, error) {
	deps = fillCreateDeps(deps)

	if deps.goos != "linux" {
		return nil, fmt.Errorf("lab destroy is supported only on linux: got %s", deps.goos)
	}
	if !deps.isRoot() {
		return nil, errors.New("lab destroy requires root privileges")
	}
	for _, cmd := range []string{"ip", "iptables"} {
		if _, err := deps.findPath(cmd); err != nil {
			return nil, fmt.Errorf("required command %q not found: %w", cmd, err)
		}
	}

	result := &DestroyResult{
		NodesDeleted: make([]string, 0),
	}

	if deps.loadState == nil {
		result.StateMissingFallback = true
		return destroyFallbackWithoutState(ctx, deps, result)
	}

	state, err := deps.loadState(ctx)
	if errors.Is(err, ErrStateNotFound) {
		result.StateMissingFallback = true
		return destroyFallbackWithoutState(ctx, deps, result)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load lab state: %w", err)
	}

	for _, ns := range state.Nodes {
		runErr := deps.exec.Run(ctx, "ip", "netns", "del", ns)
		if runErr != nil && !isNamespaceNotFoundError(runErr) {
			return nil, runErr
		}
		if runErr == nil {
			result.NodesDeleted = append(result.NodesDeleted, ns)
		}
	}

	targetBridge := state.Bridge
	if targetBridge == "" {
		targetBridge = bridgeName
	}
	exists, err := bridgeExists(ctx, deps.exec, targetBridge)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := deps.exec.Run(ctx, "ip", "link", "set", targetBridge, "down"); err != nil {
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "link", "del", targetBridge); err != nil {
			return nil, err
		}
		result.BridgeDeleted = true
	}

	for _, rule := range state.Rules {
		if err := deleteIPTablesRuleAll(ctx, deps.exec, rule); err != nil {
			return nil, err
		}
	}
	if err := restoreIPForward(ctx, deps.exec, state.IPForwardBefore); err != nil {
		return nil, err
	}
	if state.IPForwardBefore != "" {
		result.IPForwardRestored = true
		result.IPForwardRestoreValue = state.IPForwardBefore
	}
	if deps.deleteState != nil {
		if err := deps.deleteState(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func destroyFallbackWithoutState(ctx context.Context, deps createDeps, result *DestroyResult) (*DestroyResult, error) {
	members, bridgeFound, err := listBridgeMembers(ctx, deps.exec, bridgeName)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if !isManagedBridgePeer(member) {
			continue
		}
		if err := deps.exec.Run(ctx, "ip", "link", "del", member); err != nil && !isBridgeNotFoundError(err, member) {
			return nil, err
		}
	}

	if bridgeFound {
		if err := deps.exec.Run(ctx, "ip", "link", "set", bridgeName, "down"); err != nil {
			return nil, err
		}
		if err := deps.exec.Run(ctx, "ip", "link", "del", bridgeName); err != nil {
			return nil, err
		}
		result.BridgeDeleted = true
	}

	for _, rule := range managedIPTablesRules() {
		if err := deleteIPTablesRuleAll(ctx, deps.exec, rule); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func listBridgeMembers(ctx context.Context, exec Executor, bridge string) ([]string, bool, error) {
	exists, err := bridgeExists(ctx, exec, bridge)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	out, err := exec.Output(ctx, "ip", "-o", "link", "show", "master", bridge)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list bridge members for %s: %w", bridge, err)
	}
	lines := strings.Split(out, "\n")
	members := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSuffix(fields[1], ":")
		name = strings.SplitN(name, "@", 2)[0]
		members = append(members, name)
	}
	return members, true, nil
}

func isManagedBridgePeer(name string) bool {
	if !strings.HasPrefix(name, "br-node") {
		return false
	}
	nodeSuffix := strings.TrimPrefix(name, "br-")
	return isManagedNodeName(nodeSuffix)
}

func deleteIPTablesRuleAll(ctx context.Context, exec Executor, rule IPTablesRule) error {
	for {
		err := exec.Run(ctx, "iptables", rule.CheckArgs...)
		if err != nil {
			if isIPTablesRuleNotFoundError(err) {
				return nil
			}
			return fmt.Errorf("failed to check iptables rule %v: %w", rule.CheckArgs, err)
		}
		if err := exec.Run(ctx, "iptables", rule.DelArgs...); err != nil {
			if isIPTablesRuleNotFoundError(err) {
				return nil
			}
			return fmt.Errorf("failed to delete iptables rule %v: %w", rule.DelArgs, err)
		}
	}
}
