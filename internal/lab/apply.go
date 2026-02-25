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

func Apply(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	return applyWithDeps(ctx, opts, defaultCreateDeps())
}

func applyWithDeps(ctx context.Context, opts ApplyOptions, deps createDeps) (*ApplyResult, error) {
	deps = fillCreateDeps(deps)

	if deps.goos != "linux" {
		return nil, fmt.Errorf("lab apply is supported only on linux: got %s", deps.goos)
	}
	if !deps.isRoot() {
		return nil, errors.New("lab apply requires root privileges")
	}
	for _, cmd := range []string{"ip", "tc"} {
		if _, err := deps.findPath(cmd); err != nil {
			return nil, fmt.Errorf("required command %q not found: %w", cmd, err)
		}
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

	if deps.loadState == nil {
		return nil, errors.New("lab state loader is not configured")
	}
	state, err := deps.loadState(ctx)
	if errors.Is(err, ErrStateNotFound) {
		return nil, errors.New("lab state not found: run `rtc-emulator lab create` first")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load lab state: %w", err)
	}
	if !containsString(state.Nodes, opts.Node) {
		return nil, fmt.Errorf("node %q is not managed by current lab", opts.Node)
	}

	namespaces, err := listNamespaces(ctx, deps.exec)
	if err != nil {
		return nil, err
	}
	if !containsString(namespaces, opts.Node) {
		return nil, fmt.Errorf("node %q namespace not found", opts.Node)
	}

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

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
