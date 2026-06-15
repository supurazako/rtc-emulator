package lab

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ApplyOptions struct {
	Node   string
	Delay  string
	Loss   string
	Jitter string
	BW     string
}

type ApplyResult struct {
	Node   string
	Delay  string
	Loss   string
	Jitter string
	BW     string
}

type ClearOptions struct {
	Node string
}

type ClearResult struct {
	Node    string
	Cleared bool
}

func Apply(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	return applyWithDeps(ctx, opts, defaultCreateDeps())
}

func applyWithDeps(ctx context.Context, opts ApplyOptions, deps createDeps) (*ApplyResult, error) {
	deps = fillCreateDeps(deps)

	if err := validateImpairmentEnvironment(deps, "lab apply"); err != nil {
		return nil, err
	}

	opts.Node = strings.TrimSpace(opts.Node)
	if opts.Node == "" {
		return nil, errors.New("node is required")
	}
	if opts.Delay == "" && opts.Loss == "" && opts.Jitter == "" && opts.BW == "" {
		return nil, errors.New("at least one impairment flag is required (--delay/--loss/--jitter/--bw)")
	}
	if opts.Jitter != "" && opts.Delay == "" {
		return nil, errors.New("jitter requires delay")
	}

	node, err := validateImpairmentTarget(ctx, deps, opts.Node)
	if err != nil {
		return nil, err
	}
	opts.Node = node

	args := []string{
		"netns", "exec", opts.Node,
		"tc", "qdisc", "replace", "dev", "eth0", "root", "netem",
	}
	if opts.Delay != "" {
		args = append(args, "delay", opts.Delay)
		if opts.Jitter != "" {
			args = append(args, opts.Jitter)
		}
	}
	if opts.Loss != "" {
		args = append(args, "loss", opts.Loss)
	}
	if opts.BW != "" {
		args = append(args, "rate", opts.BW)
	}

	if err := deps.exec.Run(ctx, "ip", args...); err != nil {
		return nil, fmt.Errorf("failed to apply impairments to %s: %w", opts.Node, err)
	}

	return &ApplyResult{
		Node:   opts.Node,
		Delay:  opts.Delay,
		Loss:   opts.Loss,
		Jitter: opts.Jitter,
		BW:     opts.BW,
	}, nil
}

func Clear(ctx context.Context, opts ClearOptions) (*ClearResult, error) {
	return clearWithDeps(ctx, opts, defaultCreateDeps())
}

func clearWithDeps(ctx context.Context, opts ClearOptions, deps createDeps) (*ClearResult, error) {
	deps = fillCreateDeps(deps)

	if err := validateImpairmentEnvironment(deps, "lab impair clear"); err != nil {
		return nil, err
	}

	node, err := validateImpairmentTarget(ctx, deps, opts.Node)
	if err != nil {
		return nil, err
	}

	err = deps.exec.Run(ctx, "ip", "netns", "exec", node, "tc", "qdisc", "del", "dev", "eth0", "root")
	if err != nil {
		if isQdiscMissingError(err) {
			return &ClearResult{Node: node, Cleared: false}, nil
		}
		return nil, fmt.Errorf("failed to clear impairments from %s: %w", node, err)
	}

	return &ClearResult{Node: node, Cleared: true}, nil
}

func validateImpairmentEnvironment(deps createDeps, operation string) error {
	if deps.goos != "linux" {
		return fmt.Errorf("%s is supported only on linux: got %s", operation, deps.goos)
	}
	if !deps.isRoot() {
		return fmt.Errorf("%s requires root privileges", operation)
	}
	for _, cmd := range []string{"ip", "tc"} {
		if _, err := deps.findPath(cmd); err != nil {
			return fmt.Errorf("required command %q not found: %w", cmd, err)
		}
	}
	return nil
}

func validateImpairmentTarget(ctx context.Context, deps createDeps, node string) (string, error) {
	node = strings.TrimSpace(node)
	if node == "" {
		return "", errors.New("node is required")
	}

	if deps.loadState == nil {
		return "", errors.New("lab state loader is not configured")
	}
	state, err := deps.loadState(ctx)
	if errors.Is(err, ErrStateNotFound) {
		return "", errors.New("lab state not found: run `rtc-emulator lab create` first")
	}
	if err != nil {
		return "", fmt.Errorf("failed to load lab state: %w", err)
	}
	if !containsString(state.Nodes, node) {
		return "", fmt.Errorf("node %q is not managed by current lab", node)
	}

	namespaces, err := listNamespaces(ctx, deps.exec)
	if err != nil {
		return "", err
	}
	if !containsString(namespaces, node) {
		return "", fmt.Errorf("node %q namespace not found", node)
	}

	return node, nil
}

func isQdiscMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "cannot delete qdisc with handle of zero") ||
		strings.Contains(msg, "no qdisc")
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
