package lab

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWebRTCPeerNetNSArgs(t *testing.T) {
	args := webRTCPeerNetNSArgs("node1", "/tmp/rtc-emulator", WebRTCPeerOptions{
		Role:          "offerer",
		RunID:         "run-1",
		RunDir:        "runs/run-1",
		Node:          "node1",
		Peer:          "node2",
		Duration:      3 * time.Second,
		StatsInterval: 500 * time.Millisecond,
	})

	want := []string{
		"netns", "exec", "node1",
		"/tmp/rtc-emulator",
		"lab", "webrtc", "peer",
		"--role", "offerer",
		"--run-id", "run-1",
		"--run-dir", "runs/run-1",
		"--node", "node1",
		"--peer", "node2",
		"--duration", "3s",
		"--stats-interval", "500ms",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestValidateWebRTCP2POptionsRejectsSameNodeBeforeHostChecks(t *testing.T) {
	err := validateWebRTCP2POptions(t.Context(), WebRTCP2POptions{
		NodeA:         "node1",
		NodeB:         "node1",
		Duration:      time.Second,
		StatsInterval: time.Second,
	}, createDeps{})
	if err == nil || !strings.Contains(err.Error(), "node-a and node-b must be different") {
		t.Fatalf("expected same-node validation error, got %v", err)
	}
}

func TestValidateWebRTCPeerOptions(t *testing.T) {
	valid := WebRTCPeerOptions{
		Role:          "offerer",
		RunID:         "run-1",
		RunDir:        "runs/run-1",
		Node:          "node1",
		Peer:          "node2",
		Duration:      time.Second,
		StatsInterval: time.Second,
	}
	if err := validateWebRTCPeerOptions(valid); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	invalid := valid
	invalid.Role = "controller"
	err := validateWebRTCPeerOptions(invalid)
	if err == nil || !strings.Contains(err.Error(), "unsupported webrtc peer role") {
		t.Fatalf("expected unsupported role error, got %v", err)
	}

	invalid = valid
	invalid.RunDir = ""
	err = validateWebRTCPeerOptions(invalid)
	if err == nil || !strings.Contains(err.Error(), "run dir is required") {
		t.Fatalf("expected missing run dir error, got %v", err)
	}
}
