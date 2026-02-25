package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultStatePath = "/run/rtc-emulator/lab.json"

var ErrStateNotFound = errors.New("lab state not found")

type IPTablesRule struct {
	CheckArgs []string `json:"check_args"`
	AddArgs   []string `json:"add_args"`
	DelArgs   []string `json:"del_args"`
}

type LabState struct {
	Bridge          string         `json:"bridge"`
	Subnet          string         `json:"subnet"`
	Nodes           []string       `json:"nodes"`
	Rules           []IPTablesRule `json:"rules"`
	IPForwardBefore string         `json:"ip_forward_before"`
}

func loadState(_ context.Context, path string) (*LabState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrStateNotFound
		}
		return nil, fmt.Errorf("failed to read state file %s: %w", path, err)
	}
	var st LabState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}
	return &st, nil
}

func saveStateAtomic(_ context.Context, path string, state *LabState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create state dir for %s: %w", path, err)
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("failed to write state temp file %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to commit state file %s: %w", path, err)
	}
	return nil
}

func deleteStateFile(_ context.Context, path string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to remove state file %s: %w", path, err)
	}
	return nil
}
