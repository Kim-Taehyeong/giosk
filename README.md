# Giosk (GPU Kiosk)

**Open-source, Kubernetes-native GPU cloud.**

Giosk turns a Kubernetes cluster into a self-service GPU cloud — users launch VSCode / Jupyter / terminal sessions on shared or dedicated GPUs, with credits, quotas, and org/team governance built in.

> Sessions run as **real Kubernetes Pods** — native scheduling, RBAC, PVC storage, and the GPU stack (device-plugin, GFD, HAMi, DCGM). No VMs, no external scheduler.

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
![Kubernetes](https://img.shields.io/badge/Kubernetes-native-326CE5?logo=kubernetes&logoColor=white)

---

## Features

- **Self-service GPU sessions** — VSCode (code-server), Jupyter, and an in-browser terminal, launched per user as native Pods.
- **Flexible GPU modes** — whole-GPU (exclusive), fractional (HAMi vGPU: split VRAM/cores), CPU-only, and physical-node SSH lease (hybrid mode).
- **Datasets & volumes** — RWX NFS datasets with optional per-node local caching; personal persistent home (`~/nfs`) and shareable volumes.
- **Billing & governance** — credit accounting (per-GPU pricing, periodic recharge), hierarchical org → team → user credit allocation, quotas, and audit logs.
- **Observability & alerts** — Prometheus + DCGM dashboards (GPU util / VRAM / temperature), rule-based alerts (node down, disk usage, GPU temp, credit balance) via email/webhook/in-app.
- **Access gateway (optional)** — single entry point: per-session web subdomains + copy-paste SSH via short-lived tokens.

## Architecture

- **Backend** (`backend/`) — Go (Gin) API + `node-agent`. Drives the cluster via client-go (Pods / Jobs / PVCs / Services / exec). Domain state (users, sessions, orgs, wallets) lives in **MySQL**.
- **Frontend** (`frontend/`) — React console (admin + user).
- **Chart** (`deploy/helm/giosk/`) — Helm chart for the whole platform; infra dependencies (MetalLB, NFS provisioner, Prometheus, DCGM, HAMi) are toggle-able (`install: true/false`).

## Quick start

**Prerequisites** (must exist before install):
- A Kubernetes cluster with a working CNI on every schedulable node
- An **NFS server** (writable export) — required in every mode (persistent home is RWX)
- For GPUs: NVIDIA driver + container runtime + device-plugin + GFD labels (`nvidia.com/gpu.product`)
- (Bare metal, for LoadBalancer) an IP range for MetalLB — or use a cloud LB

**Install:**

```bash
# 1) copy the example values and fill in NFS server, LB range, etc.
cp deploy/values.example.yaml my-values.yaml
$EDITOR my-values.yaml

# 2a) self-host installer (builds images on the node, no registry needed)
sudo VALUES=./my-values.yaml ./deploy/deploy.sh

# 2b) or Helm directly (images must be pullable from a registry)
helm install giosk charts/giosk -f my-values.yaml --set admin.password=<PW>
```

The chart refuses to install with an unsafe config (missing NFS class, missing MetalLB range, etc.) — see `deploy/values.example.yaml` for the required fields.

## Documentation

- [Architecture](docs/architecture.md)
- [Installation](docs/installation.md) (incl. k3s notes)
- [Configuration](docs/configuration.md)
- [Development](docs/development.md)

## Repository layout

```
backend/            Go API + node-agent
frontend/           React console
charts/giosk/       Helm chart
deploy/
  values.example.yaml
  deploy.sh         self-host installer
docs/               architecture, installation, configuration, development
scripts/            dev / local-QA helpers
```

## Contributing

Issues and PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). Security reports: [SECURITY.md](SECURITY.md).

## License

[Apache-2.0](LICENSE).
