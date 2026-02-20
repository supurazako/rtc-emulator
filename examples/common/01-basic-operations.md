# CLI Lab Basic Operations

This guide shows the basic `lab create` and `lab destroy` cycle on Linux.

## Prerequisites

- Linux host
- Root privileges (or `sudo`)
- Installed commands: `ip`, `sysctl`, `iptables`, `ping`

## 1. Create a lab

```bash
sudo rtc-emulator lab create --nodes 2
```

Expected output includes created nodes and a bridge:

```text
created bridge=rtcemu0 nodes=2
- node1 ip=10.200.0.2
- node2 ip=10.200.0.3
```

## 2. Verify created resources

```bash
sudo ip netns list
sudo ip link show rtcemu0
sudo ip netns exec node1 ping -c 1 10.200.0.1
```

Checkpoints:

- `node1` and `node2` exist in netns list
- `rtcemu0` exists
- Ping to `10.200.0.1` succeeds

## 3. Destroy the lab

```bash
sudo rtc-emulator lab destroy
```

Checkpoints:

- Output includes `destroyed bridge=true`
- Output includes `state-missing-fallback=false` in normal flow

## 4. Verify cleanup

```bash
sudo ip link show rtcemu0
sudo ip netns list
```

Checkpoints:

- `rtcemu0` no longer exists
- `node1` and `node2` are removed

## 5. Re-create to confirm no leftovers

```bash
sudo rtc-emulator lab create --nodes 2
sudo rtc-emulator lab destroy
```

Both commands should succeed again.
