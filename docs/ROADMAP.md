# Development Roadmap

## Purpose

This roadmap explains the near-term implementation direction for
`rtc-emulator`.

The project is moving from a low-level lab and impairment CLI toward a
WebRTC-oriented tool that can reproduce named network events, run them in a
repeatable way, and connect those events with WebRTC-side observations.

This document is implementation-oriented. It does not track event schedules,
presentation plans, or research notes.

## Current baseline

The current public CLI focuses on local lab management:

- `rtc-emulator lab create`
- `rtc-emulator lab apply`
- `rtc-emulator lab destroy`

The existing impairment command is useful as a foundation, but the next work
will separate low-level impairment control from higher-level WebRTC event
scenarios.

## Roadmap

### 1. Lab lifecycle and impairment control

Goal:

- Make the local Linux lab and node-level impairment controls predictable.

Planned work:

- Standardize the 2-node lab flow used by the first WebRTC experiments.
- Add `lab impair apply` for explicit node-level impairment control.
- Add `lab impair clear` for explicit cleanup of node-level qdisc state.
- Keep `lab apply` available as a compatibility path while the CLI is being
  reorganized.
- Improve tests around generated `tc qdisc replace` and `tc qdisc del`
  commands.

### 2. WebRTC event scenario foundation

Goal:

- Expose WebRTC-oriented scenario names instead of making users think only in
  raw `tc/netem` operations.

Planned work:

- Add built-in scenario names such as `webrtc-uplink-congestion` and
  `webrtc-packet-loss-degradation`.
- Run scenarios as phases such as baseline, impaired, recovery, and cleanup.
- Store event logs for each run under a run-specific directory.
- Ensure cleanup is attempted even when an impairment phase fails.

### 3. WebRTC peer and stats logging

Goal:

- Connect impairment events with WebRTC-side observations.

Planned work:

- Run a headless P2P WebRTC flow inside the lab nodes.
- Send synthetic media traffic for repeatable experiments.
- Save WebRTC stats in the same run directory as the event log.
- Prefer stats that help explain quality changes, such as available outgoing
  bitrate, send/receive bitrate, frame counters, RTT, jitter, packet loss, ICE
  state, and PeerConnection state.

### 4. Scenario expansion and reporting

Goal:

- Expand from the first event scenarios into a more useful WebRTC experiment
  tool.

Possible work:

- Add RTT spike, burst loss, and downlink congestion scenarios.
- Add a small summary of before/during/after stats for a run.
- Add connectivity-oriented scenarios after the quality degradation scenarios
  are stable.
- Add scenario file replay after built-in scenarios are reliable.

## Near-term priority

The next implementation target is:

1. Add `lab impair apply`.
2. Add `lab impair clear`.
3. Keep existing `lab apply` behavior compatible.
4. Verify the flow with fake executor tests and Linux manual testing.

After that, the project can build `webrtc-uplink-congestion` on top of the
same impairment control path.

## Non-goals for the next step

The next step does not include:

- Bundling a WebRTC stack in the CLI.
- Implementing WebRTC event scenarios.
- Saving `events.jsonl` or `stats.jsonl`.
- Adding GUI or dashboard support.
- Adding SFU orchestration.
