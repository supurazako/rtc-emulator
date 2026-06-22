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

func TestLabWebRTCP2PHelpListsStatsOptions(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "webrtc", "p2p", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Run a lab WebRTC P2P flow",
		"--node-a",
		"--node-b",
		"--duration",
		"--stats-interval",
		"--runs-dir",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestLabWebRTCHelpHidesInternalPeerCommand(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lab", "webrtc", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "peer") {
		t.Fatalf("expected help not to expose internal peer command, got:\n%s", got)
	}
	if !strings.Contains(got, "p2p") {
		t.Fatalf("expected help to expose p2p command, got:\n%s", got)
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
