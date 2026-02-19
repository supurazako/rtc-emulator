# Development Notes

## Linux-first

`rtc-emulator` is Linux-first.
The runtime model is based on Linux network namespaces (`netns`) and traffic
control (`tc` / `netem`).

## macOS development

Local runtime parity on macOS is not a goal for now.
macOS contributors can still work on CLI logic and documentation, and rely on
CI to validate Linux-specific behavior.
