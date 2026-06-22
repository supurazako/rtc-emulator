# Pion P2P Operations

This guide shows the minimum manual verification flow for the built-in Pion
P2P runner.

## Prerequisites

- Linux host
- Root privileges (or `sudo`)
- Installed commands: `ip`, `sysctl`, `iptables`, `ping`, `tc`
- Go toolchain

## 1. Build the CLI

```bash
go build -o bin/rtc-emulator ./cmd/rtc-emulator
```

## 2. Create a 2-node lab

```bash
sudo ./bin/rtc-emulator lab create --nodes 2
```

## 3. Run the built-in Pion P2P flow

```bash
sudo ./bin/rtc-emulator lab webrtc p2p \
  --node-a node1 \
  --node-b node2 \
  --duration 10s \
  --stats-interval 1s
```

The command starts one hidden peer process in each namespace with
`ip netns exec`, exchanges offer/answer files under the run directory, opens a
DataChannel, sends synthetic messages, and writes merged stats.

Expected output:

```text
run-id=<run-id>
run-dir=runs/<run-id>
events=runs/<run-id>/events.jsonl
stats=runs/<run-id>/stats.jsonl
```

## 4. Verify generated logs

Check the event phases:

```bash
jq -r '.phase + " " + .status' runs/<run-id>/events.jsonl
```

Expected phases:

```text
webrtc_start ok
connected ok
stats_complete ok
cleanup ok
```

Check stats records:

```bash
jq -c '{time,node,peer,peer_connection_state,ice_connection_state,bytes_sent,bytes_received,data_messages_sent,data_messages_received}' runs/<run-id>/stats.jsonl
```

Validation points:

- records exist for both `node1` and `node2`
- `peer_connection_state` and `ice_connection_state` reach `connected`
- timestamps in `stats.jsonl` can be compared with `events.jsonl`
- byte and DataChannel message counters increase during the run

## 5. Cleanup

```bash
sudo ./bin/rtc-emulator lab destroy
```

Confirm no leftovers:

```bash
sudo ip netns list
sudo ip link show rtcemu0
```
