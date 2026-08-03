# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] — 2026-08-03

First public release. Giosk turns an existing Kubernetes cluster into a self-service GPU
cloud, and has been running a multi-node GPU lab (RTX 4090 workers, NFS, MetalLB,
kube-prometheus-stack, DCGM, HAMi) end to end.

### Added

- **Sessions** — VSCode (code-server), Jupyter, and an in-browser terminal, launched per user
  as native Kubernetes Pods. Access channels are declared by the image, so a session exposes
  exactly what its image provides.
- **GPU modes** — whole-GPU (exclusive), fractional (HAMi vGPU) via admin-defined offerings,
  CPU-only, and physical-node SSH leases (`deployment.mode: hybrid`).
- **Session reconfigure** — change the compute of a stopped session (attach or detach a GPU)
  without recreating it; home, volumes and datasets are preserved.
- **Direct container SSH** — user public keys mounted as a Secret, so a newly registered key
  reaches a running session immediately. The access gateway is an optional extra path, not a
  requirement.
- **Datasets** — register from an NFS inbox or a URL, per-node local caching with progress,
  and read-only auto-mount at `~/datasets`. The node picker shows which nodes already have a copy.
- **Storage** — session-local persistent home (kept across stop, removed on delete), RWX
  persistent home at `~/nfs`, shareable volumes with server-enforced RW/RO, node-local scratch,
  and a cleaner DaemonSet driven by disk thresholds.
- **Billing & governance** — per-membership credit wallets, GPU-type pricing, periodic
  billing of running sessions with automatic stop on insufficient balance, hierarchical
  org → team → member allocation, recurring and immediate refills, and showback reports.
- **Policy limits** — hierarchical hard limits (concurrent sessions, stopped sessions, GPU
  count, VRAM, ephemeral disk, volume quota) resolved global → org → team → user.
- **Observability** — Prometheus + DCGM dashboards for node and per-session GPU/CPU/VRAM,
  operations and infrastructure dashboards with selectable trend ranges, and rule-based
  alerts (node down, disk, GPU temperature, credit balance, per-session usage) delivered by
  email, webhook, and in-app inbox.
- **Multi-role console** — a single console that adapts to the viewer's level, with scope
  switching (`X-Console-Scope`) for admins who manage more than one org or team.
- **Image build** — guided Dockerfile generation and in-cluster Kaniko builds pushed to a
  bundled or external registry; external images can be registered directly.
- **Access gateway (optional)** — per-session web subdomains and copy-paste SSH behind a
  single entry point, using short-lived tokens so secrets are injected rather than shared.
- **Installation** — Helm chart with toggle-able infra bundling (MetalLB, NFS provisioner,
  kube-prometheus-stack, DCGM, HAMi, registry) and a self-hosting installer that builds and
  distributes images without a registry. Unsafe configurations are rejected at render time.
- **i18n** — the console ships localized UI across a broad set of languages.
- **Docs** — Korean documentation set under `docs/ko/` (architecture, installation,
  configuration, operations, troubleshooting, API reference, development, ADRs).

### Changed

- Exclusive and fractional GPU availability are accounted separately; physical GPU counts are
  read from GPU Feature Discovery labels rather than HAMi's advertised capacity, and the
  exclusive request cap is bounded by a single node's free GPUs.
- Session admission is serialized with a MySQL named lock and records its reservation before
  provisioning, so concurrent requests cannot pass the same capacity check.
- Session home moved from shared NFS to a per-session node-local persistent volume; sessions
  are node-pinned as a consequence.
- Physical-node leases are reclaimed on sustained idleness instead of a fixed timeout.
- Idle detection corroborates GPU utilization with power draw.
- Dataset registration dropped web upload in favour of the NFS inbox and URL download.
- Session exposure defaults to NodePort; the port-forward mode was removed.

### Fixed

- Gateway WebSocket disconnects (code 1006) caused by a rewritten `Host` header.
- Sessions stuck in `Pending` because the Pod was created before its PVCs were bound.
- Per-session GPU metrics returning empty because DCGM reports the workload pod under the
  `exported_pod` label.
- Repeated false "zero credit" alerts after wallets became per-membership.
- Admin and user console scopes contaminating each other.

[Unreleased]: https://github.com/Kim-Taehyeong/giosk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Kim-Taehyeong/giosk/releases/tag/v0.1.0
