# Browser 3-Pane Demo (Remote Linux over SSH)

This guide shows a beginner-friendly demo setup:

- run `rtc-emulator` and `ion-sfu` on a remote Linux host
- view the UI from your macOS browser
- compare Good / Medium / Bad conditions side-by-side
- use the bundled demo compose file in this repository

## Why this setup

`rtc-emulator` controls Linux network namespaces and `tc`, so runtime must be Linux.
macOS is used as the presentation device (browser + screen share).

## Prerequisites

- macOS laptop with SSH access to a Linux host
- Linux host with:
  - Docker and Docker Compose
  - `ip`, `tc`, `iptables`, `sysctl`, `ping`
  - `rtc-emulator` binary available on `PATH`
- this repository checked out on Linux host
- bundled files:
  - `examples/demo/compose.yml`
  - `examples/demo/ui/`
- A test video source (loop) for stable reproducibility

## 0. If you use macOS: Start SSH tunnel from macOS

Run from your macOS terminal:

```bash
ssh -L 7000:127.0.0.1:7000 -L 8080:127.0.0.1:8080 <user>@<linux-host>
```

Keep this SSH session open.

Notes:

- `7000`: SFU/signaling port (adjust to your setup)
- `8080`: browser UI port (adjust to your setup)

## 1. Start services on Linux

On Linux host:

```bash
cd /path/to/rtc-emulator/examples/demo
docker compose -f compose.yml up -d
docker compose -f compose.yml ps
```

This starts:

- `ion-sfu` on `:7000`
- static demo UI (`nginx`) on `:8080`

## 2. Create lab and apply 3-level presets

On Linux host:

```bash
sudo rtc-emulator lab create --nodes 3
sudo rtc-emulator lab apply --node node2 --delay 80ms --jitter 15ms
sudo rtc-emulator lab apply --node node3 --loss 2% --bw 800kbit
sudo rtc-emulator lab show
```

Preset meaning:

- `node1`: Good (no impairment)
- `node2`: Medium (delay + jitter)
- `node3`: Bad (loss + bandwidth limit)

## 3. Open the demo UI from macOS

Open your browser on macOS:

```text
http://localhost:8080
```

Show 3 panes and map:

- Good -> node1
- Medium -> node2
- Bad -> node3

If tracks are not visible yet, verify your publisher/subscriber clients are connected
to the same room shown in the UI.

## 4. What to say in a 3-5 minute demo

1. "All 3 panes receive the same source, only network conditions differ."
2. "Good stays smooth."
3. "Medium shows delay feeling."
4. "Bad shows visible degradation and lower quality."
5. "This is reproducible by command, not manual toggling."

If available in UI, mention `RTT` and `bitrate` alongside visible quality.

## 5. Cleanup

On Linux host:

```bash
sudo rtc-emulator lab destroy
docker compose -f compose.yml down
```

Optional check:

```bash
sudo ip link show rtcemu0
sudo ip netns list
```

Expected: no `rtcemu0`, and no `node1`/`node2`/`node3`.

## Troubleshooting

- Browser cannot connect:
  - check SSH tunnel is still alive
  - verify port mapping (`7000`, `8080`) matches your setup
- `lab create` fails:
  - run with `sudo`
  - verify required Linux commands exist
- No visible difference:
  - confirm `lab show` output includes node2/node3 settings
  - increase impairment values for stronger effect
