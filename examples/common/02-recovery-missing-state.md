# CLI Lab Recovery: Missing State File

This guide explains how `lab destroy` behaves when the state file is missing.

## Purpose

When `/run/rtc-emulator/lab.json` is missing, `lab destroy` uses a safe fallback:

- removes `rtcemu0` and managed bridge peers (`br-node<NUMBER>`)
- does not delete namespaces directly in fallback mode

## 1. Prepare a lab

```bash
sudo rtc-emulator lab create --nodes 2
```

## 2. Simulate missing state

```bash
sudo rm -f /run/rtc-emulator/lab.json
```

## 3. Run destroy

```bash
sudo rtc-emulator lab destroy
```

Checkpoints:

- Output includes `state-missing-fallback=true`
- Bridge cleanup still succeeds when `rtcemu0` exists

## 4. Verify bridge cleanup

```bash
sudo ip link show rtcemu0
```

Expected: bridge does not exist.

## 5. Optional manual namespace cleanup

Fallback mode intentionally avoids deleting namespaces to reduce accidental deletions.
If you need cleanup after a broken state, remove them manually:

```bash
sudo ip netns del node1
sudo ip netns del node2
```

Adjust names to your environment.
