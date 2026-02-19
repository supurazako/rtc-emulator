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
