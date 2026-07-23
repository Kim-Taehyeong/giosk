# Architecture

Giosk is a control plane for running GPU workloads as **native Kubernetes Pods**. It is a
conventional web backend that drives the cluster through the Kubernetes API — not a
CRD/operator system. Domain state lives in MySQL; the cluster is the execution engine.

## Components

| Component | Path | Role |
|-----------|------|------|
| **API** | `backend/cmd/api` | Go (Gin) REST API. Auth, sessions, billing, orgs, datasets, volumes, alerts. Talks to the cluster via client-go. |
| **node-agent** | `backend/cmd/node-agent` | DaemonSet (hybrid mode). Provisions physical-node accounts (`useradd`, `authorized_keys`, NFS mounts) for SSH-lease sessions. |
| **Frontend** | `frontend/` | React console (admin + user), served by nginx which also proxies `/api`. |
| **MySQL** | chart-bundled or external | System of record: users, sessions, wallets, orgs, offerings, audit. |
| **Chart** | `charts/giosk/` | Helm chart for the platform + toggle-able infra. |

## What runs where

- **Container sessions** → a Pod in the user's namespace. The image provides the channels
  (VSCode/Jupyter/web); the API mounts home (RWX NFS), datasets (RO), and volumes.
- **Web terminal** → the API opens a WebSocket and streams `kubectl exec` (container) or SSH
  (physical) to the browser (xterm.js). No per-session sidecar.
- **Physical (hybrid) sessions** → no Pod; the API leases a whole node and `node-agent`
  provisions a real Linux account. Access is SSH-key based.
- **Image build** → in-cluster Kaniko Jobs push to the configured registry.
- **Dataset cache** → per-node copy Jobs stage NFS datasets onto node-local disk.

## Kubernetes primitives used

Pods, Jobs, PersistentVolumeClaims (RWX NFS), Services (LoadBalancer/NodePort),
`remotecommand` exec, node affinity / selectors / taints, ServiceAccount + ClusterRole (RBAC),
and the GPU stack — device-plugin, GPU Feature Discovery labels (`nvidia.com/gpu.product`),
HAMi (fractional GPU scheduling), DCGM (metrics). Giosk also **creates** other operators'
custom resources: Prometheus `PodMonitor`/`ServiceMonitor` and MetalLB `IPAddressPool`.

## Deployment topology

The API needs cluster access. Two models are supported:

- **In-cluster (default, recommended)** — API runs as a Deployment with a ServiceAccount;
  uses in-cluster config. No credentials to manage, least-privilege via RBAC.
- **External** — API runs outside the cluster with a kubeconfig (client-go falls back to it).
  Useful for managing multiple clusters, at the cost of credential management.

`controlPlane.enabled` pins the API (and gateway) to control-plane nodes so GPU workers stay
dedicated to workloads.
