# Scope

## Purpose

rtc-emulator is a CLI tool to reproduce network impairments for WebRTC experiments.
It provides a "lab mode" to create multiple isolated client nodes on a single Linux host,
apply different network conditions per node, and replay experiments reliably.

## Target users

- WebRTC developers who want repeatable "bad network" conditions
- Researchers who need reproducible network scenarios for experiments and comparisons

## In scope (v0.x)

### Lab mode (core)

- Create an isolated multi-node lab on a single Linux host using network namespaces
- Each node represents a WebRTC client endpoint (a participant)
- Apply different impairment settings per node:
  - delay
  - packet loss
  - jitter
  - bandwidth limitation
- Show current lab state (nodes + applied settings)
- Safe cleanup (destroy must restore the system state; commands should be idempotent)
- Export/import lab configuration as JSON for repeatability

### CLI/UX

- Commands optimized for scripting and automation
- Clear error messages and safe defaults
- Optional helper command to run a user-provided command inside a node (lab exec)

### Platform support

- Linux-first (requires iproute2 + tc/netem)
- macOS may be supported later via a different backend, but is not a v0.x goal

## Out of scope (for now)

- Bundling a WebRTC stack (built-in clients, SFU, signaling server)
- Flow-level shaping by PID / SSRC / deep packet inspection
- Full cross-platform parity (Windows support is not planned for v0.x)
- Distributed orchestration across multiple physical machines
- GUI/dashboard (planned after CLI stabilizes)

## Non-goals

- Being a general-purpose network emulator for all protocols.
  The project intentionally prioritizes WebRTC-oriented lab workflows and reproducibility.

## Success criteria (early)

- A developer can create a 3-node lab and apply different impairments with a few commands
- Lab configuration can be exported to JSON and replayed reliably
- Cleanup is safe and predictable (no "stuck" qdisc / namespaces)
