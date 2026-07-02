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
latest-dir=runs/latest
events=runs/<run-id>/events.jsonl
stats=runs/<run-id>/stats.jsonl
```

## 4. Verify generated logs

Check the event phases:

```bash
jq -r '.phase + " " + .status' runs/latest/events.jsonl
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
jq -c '{time,node,peer,peer_connection_state,ice_connection_state,bytes_sent,bytes_received,data_messages_sent,data_messages_received}' runs/latest/stats.jsonl
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

## 6. Run the uplink congestion scenario

Create a fresh 2-node lab, then run the named end-to-end scenario:

```bash
sudo ./bin/rtc-emulator lab create --nodes 2

sudo ./bin/rtc-emulator lab scenario run webrtc-uplink-congestion \
  --node node1 \
  --peer node2 \
  --bw 1mbit \
  --baseline 5s \
  --impaired 10s \
  --recovery 5s \
  --stats-interval 1s
```

Expected output:

```text
run-id=<run-id>
run-dir=runs/<run-id>
latest-dir=runs/latest
events=runs/<run-id>/events.jsonl
stats=runs/<run-id>/stats.jsonl
```

Check the phase window:

```bash
jq -r '.time + " " + .phase + " " + .status + " " + (.condition.bw // "")' runs/latest/events.jsonl
```

Check WebRTC-side counters near that window:

```bash
jq -r 'select(.node=="node1") | [.time,.bytes_sent,.data_messages_sent] | @tsv' runs/latest/stats.jsonl
```

Check cleanup state:

```bash
sudo ip netns exec node1 tc qdisc show dev eth0
```

Manual verification notes:

- `impaired` should show `1mbit` for `node1`.
- `cleanup` should leave no managed qdisc behind on `node1`.
- compare `node1` `bytes_sent` deltas before, during, and after the impaired window; this DataChannel-only runner does not produce video frame counters.
