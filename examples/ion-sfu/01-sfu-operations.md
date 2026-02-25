# ion-sfu Operations (Docker Compose)

This guide shows an SFU-based experiment flow using `rtc-emulator` and `ion-sfu`.

## Prerequisites

- Linux host
- Root privileges (or `sudo`)
- Installed commands: `ip`, `sysctl`, `iptables`, `ping`, `tc`
- Docker and Docker Compose
- A client app that can publish/subscribe through ion-sfu

## 1. Start ion-sfu

Use your ion-sfu compose setup:

```bash
docker compose up -d
```

Verify ion-sfu is running before continuing.

## 2. Create lab nodes

```bash
sudo rtc-emulator lab create --nodes 2
```

## 3. Run publisher/subscriber from namespaces

Example pattern:

```bash
sudo ip netns exec node1 <your-client-command> --role publisher
sudo ip netns exec node2 <your-client-command> --role subscriber
```

Note:

- `<your-client-command>` is a placeholder.
- Replace it with the actual command that starts your WebRTC client app in your environment.
- Example form: `./bin/webrtc-client --room demo --name node1`

Validation points:

- publisher can publish to room/session
- subscriber receives media from SFU
- end-to-end logs/metrics are visible

## 4. Observe behavior under network conditions

Apply and compare publisher/subscriber conditions:

```bash
sudo rtc-emulator lab apply --node node1 --bw 1mbit --delay 40ms
sudo rtc-emulator lab apply --node node2 --loss 2%
```

Verify applied qdisc state:

```bash
sudo ip netns exec node1 tc qdisc show dev eth0
sudo ip netns exec node2 tc qdisc show dev eth0
```

Compare:

- publisher-side degradation impact
- subscriber-side degradation impact
- SFU relay stability under different node conditions

## 5. Cleanup

```bash
sudo rtc-emulator lab destroy
docker compose down
```

Confirm bridge cleanup:

```bash
sudo ip link show rtcemu0
```
