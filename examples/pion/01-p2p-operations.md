# Pion P2P Operations

This guide shows a practical P2P experiment flow with `rtc-emulator` and Pion.
The focus is operational verification with 3 participants.

## Prerequisites

- Linux host
- Root privileges (or `sudo`)
- Installed commands: `ip`, `sysctl`, `iptables`, `ping`
- Go toolchain
- A runnable Pion peer app (your own app or sample in your environment)

## 1. Create a 3-node lab

```bash
sudo rtc-emulator lab create --nodes 3
```

## 2. Start 3 Pion participants

Run one participant process per namespace:

```bash
sudo ip netns exec node1 <your-pion-peer-command> --name node1
sudo ip netns exec node2 <your-pion-peer-command> --name node2
sudo ip netns exec node3 <your-pion-peer-command> --name node3
```

Note:

- `<your-pion-peer-command>` is a placeholder.
- Replace it with the actual command used in your environment to start a Pion peer process.
- Example form: `./bin/pion-peer --room demo --name node1`

Use your normal signaling setup for session join/offer/answer exchange.

## 3. Check P2P mesh behavior

Validation points:

- all 3 participants join the same session
- media path is established for every participant
- per-node logs/stats are visible and stable

Operational note:

- with mesh-style P2P, CPU and upstream usage increase with participant count

## 4. Optional impairment experiments

When `lab apply` is available in your workflow, apply different conditions per node
and compare effects participant-by-participant.

## 5. Cleanup

```bash
sudo rtc-emulator lab destroy
```

Confirm no leftovers:

```bash
sudo ip netns list
sudo ip link show rtcemu0
```
