package lab

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ShowResult struct {
	Bridge string
	Subnet string
	Nodes  []NodeStatus
}

type NodeStatus struct {
	Name      string
	Interface string
	Delay     string
	Loss      string
	Jitter    string
	BW        string
	RawQDisc  string
}

func Show(ctx context.Context) (*ShowResult, error) {
	return showWithDeps(ctx, defaultCreateDeps())
}

func showWithDeps(ctx context.Context, deps createDeps) (*ShowResult, error) {
	deps = fillCreateDeps(deps)

	if deps.goos != "linux" {
		return nil, fmt.Errorf("lab show is supported only on linux: got %s", deps.goos)
	}
	if !deps.isRoot() {
		return nil, errors.New("lab show requires root privileges")
	}
	for _, cmd := range []string{"ip", "tc"} {
		if _, err := deps.findPath(cmd); err != nil {
			return nil, fmt.Errorf("required command %q not found: %w", cmd, err)
		}
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

	namespaces, err := listNamespaces(ctx, deps.exec)
	if err != nil {
		return nil, err
	}

	result := &ShowResult{
		Bridge: state.Bridge,
		Subnet: state.Subnet,
		Nodes:  make([]NodeStatus, 0, len(state.Nodes)),
	}

	for _, node := range state.Nodes {
		if !containsString(namespaces, node) {
			return nil, fmt.Errorf("node %q namespace not found", node)
		}

		out, err := deps.exec.Output(ctx, "ip", "netns", "exec", node, "tc", "qdisc", "show", "dev", "eth0")
		if err != nil {
			return nil, fmt.Errorf("failed to inspect qdisc for %s: %w", node, err)
		}

		status := NodeStatus{
			Name:      node,
			Interface: "eth0",
			RawQDisc:  "none",
		}

		if netemLine, ok := extractNetemLine(out); ok {
			status.RawQDisc = netemLine
			status.Delay, status.Jitter, status.Loss, status.BW = parseNetemValues(netemLine)
		}

		result.Nodes = append(result.Nodes, status)
	}

	return result, nil
}

func extractNetemLine(out string) (string, bool) {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		normalized := normalizeSpaces(strings.TrimSpace(line))
		if normalized == "" {
			continue
		}
		if strings.HasPrefix(normalized, "qdisc netem ") || normalized == "qdisc netem" {
			return normalized, true
		}
	}
	return "", false
}

func parseNetemValues(line string) (delay, jitter, loss, bw string) {
	fields := strings.Fields(line)
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "delay":
			if i+1 < len(fields) {
				delay = fields[i+1]
			}
			if i+2 < len(fields) && !isNetemKeyword(fields[i+2]) {
				jitter = fields[i+2]
			}
		case "loss":
			if i+1 < len(fields) {
				loss = fields[i+1]
			}
		case "rate":
			if i+1 < len(fields) {
				bw = fields[i+1]
			}
		}
	}
	return delay, jitter, loss, bw
}

func isNetemKeyword(token string) bool {
	switch token {
	case "delay", "loss", "rate", "limit", "distribution", "duplicate", "corrupt", "reorder", "gap", "ecn", "slot":
		return true
	default:
		return false
	}
}

func normalizeSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
