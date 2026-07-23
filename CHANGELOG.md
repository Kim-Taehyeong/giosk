# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- In-browser web terminal for sessions (xterm.js over an API WebSocket; container `exec` and
  physical-node SSH).
- Toggle-able infra bundling (`install: true/false`) for MetalLB, NFS provisioner,
  kube-prometheus-stack, DCGM, and HAMi.
- `controlPlane.enabled` to pin API/gateway to control-plane nodes.
- Public docs (`docs/`), example values, and open-source project scaffolding.

[Unreleased]: https://github.com/Kim-Taehyeong/giosk/commits/main
