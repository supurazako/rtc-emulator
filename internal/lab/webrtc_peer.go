package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	webRTCPeerRoleOfferer  = "offerer"
	webRTCPeerRoleAnswerer = "answerer"
)

type WebRTCPeerOptions struct {
	Role          string
	RunID         string
	RunDir        string
	Node          string
	Peer          string
	Duration      time.Duration
	StatsInterval time.Duration
}

func RunWebRTCPeer(ctx context.Context, opts WebRTCPeerOptions) error {
	opts = normalizeWebRTCPeerOptions(opts)
	if err := validateWebRTCPeerOptions(opts); err != nil {
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}
	defer pc.Close()

	state := &webRTCPeerRuntimeState{
		peerConnection: webrtc.PeerConnectionStateNew.String(),
		iceConnection:  webrtc.ICEConnectionStateNew.String(),
	}
	connected := make(chan struct{})
	var connectedOnce sync.Once
	dataOpen := make(chan struct{})
	var dataOpenOnce sync.Once
	done := make(chan struct{})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		state.setPeerConnection(s.String())
		if s == webrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() { close(connected) })
		}
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		state.setICEConnection(s.String())
	})

	if opts.Role == webRTCPeerRoleOfferer {
		if err := configureOffererDataChannel(pc, dataOpen, &dataOpenOnce, done); err != nil {
			return err
		}
	} else {
		configureAnswererDataChannel(pc, dataOpen, &dataOpenOnce)
	}

	if opts.Role == webRTCPeerRoleOfferer {
		if err := runOffererSignaling(ctx, pc, opts.RunDir); err != nil {
			return err
		}
	} else {
		if err := runAnswererSignaling(ctx, pc, opts.RunDir); err != nil {
			return err
		}
	}

	if err := waitForSignal(ctx, connected, 20*time.Second, "peer connection connected"); err != nil {
		return err
	}
	if err := waitForSignal(ctx, dataOpen, 20*time.Second, "data channel open"); err != nil {
		return err
	}

	statsPath := filepath.Join(opts.RunDir, peerStatsFilename(opts.Node))
	logger, err := newStatsLogger(statsPath, func(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
		return os.OpenFile(path, flag, perm)
	})
	if err != nil {
		close(done)
		return err
	}
	err = writePeerStats(ctx, pc, state, logger, opts)
	close(done)
	if closeErr := logger.close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

type webRTCPeerRuntimeState struct {
	mu             sync.Mutex
	peerConnection string
	iceConnection  string
}

func (s *webRTCPeerRuntimeState) setPeerConnection(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerConnection = v
}

func (s *webRTCPeerRuntimeState) setICEConnection(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.iceConnection = v
}

func (s *webRTCPeerRuntimeState) snapshot() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peerConnection, s.iceConnection
}

func normalizeWebRTCPeerOptions(opts WebRTCPeerOptions) WebRTCPeerOptions {
	opts.Role = strings.TrimSpace(opts.Role)
	opts.RunID = strings.TrimSpace(opts.RunID)
	opts.RunDir = strings.TrimSpace(opts.RunDir)
	opts.Node = strings.TrimSpace(opts.Node)
	opts.Peer = strings.TrimSpace(opts.Peer)
	if opts.Duration <= 0 {
		opts.Duration = defaultWebRTCDuration
	}
	if opts.StatsInterval <= 0 {
		opts.StatsInterval = defaultWebRTCStatsInterval
	}
	return opts
}

func validateWebRTCPeerOptions(opts WebRTCPeerOptions) error {
	if opts.Role != webRTCPeerRoleOfferer && opts.Role != webRTCPeerRoleAnswerer {
		return fmt.Errorf("unsupported webrtc peer role %q", opts.Role)
	}
	if opts.RunID == "" {
		return errors.New("run id is required")
	}
	if opts.RunDir == "" {
		return errors.New("run dir is required")
	}
	if opts.Node == "" {
		return errors.New("node is required")
	}
	if opts.Peer == "" {
		return errors.New("peer is required")
	}
	return nil
}

func configureOffererDataChannel(pc *webrtc.PeerConnection, dataOpen chan struct{}, openOnce *sync.Once, done <-chan struct{}) error {
	channel, err := pc.CreateDataChannel("synthetic", nil)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %w", err)
	}
	channel.OnOpen(func() {
		openOnce.Do(func() { close(dataOpen) })
		go sendSyntheticMessages(channel, done)
	})
	channel.OnMessage(func(webrtc.DataChannelMessage) {})
	return nil
}

func configureAnswererDataChannel(pc *webrtc.PeerConnection, dataOpen chan struct{}, openOnce *sync.Once) {
	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnOpen(func() {
			openOnce.Do(func() { close(dataOpen) })
		})
		channel.OnMessage(func(msg webrtc.DataChannelMessage) {
			_ = channel.SendText("echo:" + string(msg.Data))
		})
	})
}

func sendSyntheticMessages(channel *webrtc.DataChannel, done <-chan struct{}) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	counter := 0
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			counter++
			if err := channel.SendText(fmt.Sprintf("synthetic-%d", counter)); err != nil {
				return
			}
		}
	}
}

func runOffererSignaling(ctx context.Context, pc *webrtc.PeerConnection, runDir string) error {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local offer: %w", err)
	}
	<-gatherComplete

	if err := writeSessionDescription(filepath.Join(runDir, "signal", "offer.json"), pc.LocalDescription()); err != nil {
		return err
	}
	answer, err := waitForSessionDescription(ctx, filepath.Join(runDir, "signal", "answer.json"), 20*time.Second)
	if err != nil {
		return err
	}
	if err := pc.SetRemoteDescription(*answer); err != nil {
		return fmt.Errorf("failed to set remote answer: %w", err)
	}
	return nil
}

func runAnswererSignaling(ctx context.Context, pc *webrtc.PeerConnection, runDir string) error {
	offer, err := waitForSessionDescription(ctx, filepath.Join(runDir, "signal", "offer.json"), 20*time.Second)
	if err != nil {
		return err
	}
	if err := pc.SetRemoteDescription(*offer); err != nil {
		return fmt.Errorf("failed to set remote offer: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("failed to create answer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("failed to set local answer: %w", err)
	}
	<-gatherComplete

	if err := writeSessionDescription(filepath.Join(runDir, "signal", "answer.json"), pc.LocalDescription()); err != nil {
		return err
	}
	return nil
}

func writeSessionDescription(path string, desc *webrtc.SessionDescription) error {
	if desc == nil {
		return errors.New("session description is nil")
	}
	b, err := json.Marshal(desc)
	if err != nil {
		return fmt.Errorf("failed to encode session description %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("failed to write session description %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to commit session description %s: %w", path, err)
	}
	return nil
}

func waitForSessionDescription(ctx context.Context, path string, timeout time.Duration) (*webrtc.SessionDescription, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			var desc webrtc.SessionDescription
			if err := json.Unmarshal(b, &desc); err != nil {
				return nil, fmt.Errorf("failed to parse session description %s: %w", path, err)
			}
			return &desc, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read session description %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for session description %s: %w", path, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func waitForSignal(ctx context.Context, ch <-chan struct{}, timeout time.Duration, name string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for %s: %w", name, ctx.Err())
	}
}

func writePeerStats(ctx context.Context, pc *webrtc.PeerConnection, state *webRTCPeerRuntimeState, logger *statsLogger, opts WebRTCPeerOptions) error {
	if err := logger.write(collectWebRTCStats(pc, state, opts)); err != nil {
		return err
	}
	ticker := time.NewTicker(opts.StatsInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(opts.Duration)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return logger.write(collectWebRTCStats(pc, state, opts))
		case <-ticker.C:
			if err := logger.write(collectWebRTCStats(pc, state, opts)); err != nil {
				return err
			}
		}
	}
}

func collectWebRTCStats(pc *webrtc.PeerConnection, state *webRTCPeerRuntimeState, opts WebRTCPeerOptions) WebRTCStatsRecord {
	pcState, iceState := state.snapshot()
	record := WebRTCStatsRecord{
		RunID:               opts.RunID,
		Time:                time.Now().UTC().Format(time.RFC3339Nano),
		Node:                opts.Node,
		Peer:                opts.Peer,
		PeerConnectionState: pcState,
		ICEConnectionState:  iceState,
	}

	var bytesSent uint64
	var haveBytesSent bool
	var bytesReceived uint64
	var haveBytesReceived bool
	var packetsSent uint64
	var havePacketsSent bool
	var packetsReceived uint64
	var havePacketsReceived bool
	var packetsLost int64
	var havePacketsLost bool
	var framesSent uint64
	var haveFramesSent bool
	var framesReceived uint64
	var haveFramesReceived bool
	var dataMessagesSent uint64
	var haveDataMessagesSent bool
	var dataMessagesReceived uint64
	var haveDataMessagesReceived bool

	for _, raw := range pc.GetStats() {
		switch stat := raw.(type) {
		case webrtc.ICECandidatePairStats:
			if stat.Nominated {
				bytesSent += stat.BytesSent
				haveBytesSent = true
				bytesReceived += stat.BytesReceived
				haveBytesReceived = true
				packetsSent += uint64(stat.PacketsSent)
				havePacketsSent = true
				packetsReceived += uint64(stat.PacketsReceived)
				havePacketsReceived = true
				if stat.CurrentRoundTripTime > 0 {
					record.RoundTripTime = float64Ptr(stat.CurrentRoundTripTime)
				} else if stat.ResponsesReceived > 0 && stat.TotalRoundTripTime > 0 {
					record.RoundTripTime = float64Ptr(stat.TotalRoundTripTime / float64(stat.ResponsesReceived))
				}
			}
		case webrtc.DataChannelStats:
			bytesSent += stat.BytesSent
			haveBytesSent = true
			bytesReceived += stat.BytesReceived
			haveBytesReceived = true
			dataMessagesSent += uint64(stat.MessagesSent)
			haveDataMessagesSent = true
			dataMessagesReceived += uint64(stat.MessagesReceived)
			haveDataMessagesReceived = true
		case webrtc.PeerConnectionStats:
			record.DataChannelsOpened = uint64Ptr(uint64(stat.DataChannelsOpened))
			record.DataChannelsClosed = uint64Ptr(uint64(stat.DataChannelsClosed))
		case webrtc.OutboundRTPStreamStats:
			bytesSent += stat.BytesSent
			haveBytesSent = true
			packetsSent += uint64(stat.PacketsSent)
			havePacketsSent = true
			framesSent += uint64(stat.FramesSent)
			haveFramesSent = true
		case webrtc.InboundRTPStreamStats:
			bytesReceived += stat.BytesReceived
			haveBytesReceived = true
			packetsReceived += uint64(stat.PacketsReceived)
			havePacketsReceived = true
			packetsLost += int64(stat.PacketsLost)
			havePacketsLost = true
			if stat.Jitter > 0 {
				record.Jitter = float64Ptr(stat.Jitter)
			}
			framesReceived += uint64(stat.FramesReceived)
			haveFramesReceived = true
		}
	}

	if haveBytesSent {
		record.BytesSent = uint64Ptr(bytesSent)
	}
	if haveBytesReceived {
		record.BytesReceived = uint64Ptr(bytesReceived)
	}
	if havePacketsSent {
		record.PacketsSent = uint64Ptr(packetsSent)
	}
	if havePacketsReceived {
		record.PacketsReceived = uint64Ptr(packetsReceived)
	}
	if havePacketsLost {
		record.PacketsLost = int64Ptr(packetsLost)
	}
	if haveFramesSent {
		record.FramesSent = uint64Ptr(framesSent)
	}
	if haveFramesReceived {
		record.FramesReceived = uint64Ptr(framesReceived)
	}
	if haveDataMessagesSent {
		record.DataMessagesSent = uint64Ptr(dataMessagesSent)
	}
	if haveDataMessagesReceived {
		record.DataMessagesReceived = uint64Ptr(dataMessagesReceived)
	}
	return record
}
