package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLabImpairHelpListsApplyAndClear(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Manage node network impairments", "apply", "clear"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabScenarioRunHelpListsBuiltInScenarioOptions(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "scenario", "run", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Run a named lab scenario", "--runs-dir", "--node", "--bw"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabImpairApplyRejectsPositionalArgs(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "apply", "--node", "node1", "--bw", "1mbit", "extra"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "extra"`) {
		t.Fatalf("expected positional arg error, got: %v", err)
	}
}

func TestLabImpairClearRejectsPositionalArgs(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "impair", "clear", "--node", "node1", "extra"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "extra"`) {
		t.Fatalf("expected positional arg error, got: %v", err)
	}
}
