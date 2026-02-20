package lab

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Executor interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) (string, error)
}

type osExecutor struct{}

func (e osExecutor) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("%s %v: %w", name, args, err)
		}
		return fmt.Errorf("%s %v: %s (%w)", name, args, msg, err)
	}
	return nil
}

func (e osExecutor) Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("%s %v: %w", name, args, err)
		}
		return "", fmt.Errorf("%s %v: %s (%w)", name, args, msg, err)
	}

	return stdout.String(), nil
}
