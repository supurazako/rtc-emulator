# rtc-emulator
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[Contributing Guide](CONTRIBUTING.md)
[Code of Conduct](CODE_OF_CONDUCT.md)

A CLI tool to emu­late network impairments for WebRTC experiments (loss/jitter/delay/bandwidth)

## Examples

```bash
rtc-emulator lab create --nodes 3
rtc-emulator lab apply --node node1 --delay 50ms --loss 1% --jitter 10ms --bw 2mbit
rtc-emulator lab show
rtc-emulator lab destroy
```

### Operation Guides

- Common
  - [01 Basic Operations](examples/common/01-basic-operations.md)
  - [02 Recovery: Missing State](examples/common/02-recovery-missing-state.md)
- Demo
  - [01 Browser 3-Pane Demo (Remote Linux over SSH)](examples/demo/01-browser-three-pane-remote-linux.md)
- Pion
  - [01 P2P Operations](examples/pion/01-p2p-operations.md)
- ion-sfu
  - [01 SFU Operations](examples/ion-sfu/01-sfu-operations.md)

Note: runtime control commands (`lab create/apply/show/destroy`) require Linux.
Use macOS as a browser/presentation client via SSH tunneling.
For a bundled starter setup, use `examples/demo/compose.yml` and `examples/demo/ui/`.
