# Configuration

All install-time configuration is set through Helm values. See
[`deploy/values.example.yaml`](../deploy/values.example.yaml) for a starting point and
[`charts/giosk/values.yaml`](../charts/giosk/values.yaml) for the full set with comments.

## Modes

| Key | Values | Meaning |
|-----|--------|---------|
| `deployment.mode` | `container` \| `hybrid` | `hybrid` adds physical-node SSH lease sessions. |
| `billing.mode` | `credit` \| `dynamic` \| `free` | Credit accounting, first-come lease, or unrestricted. |

## Infra bundle toggles

Each infra dependency is installed by Giosk (`install: true`) or expected to already exist
(`install: false`). Bundle-able ones default to on; site-specific values are required when bundling.

| Key | Default | Notes |
|-----|---------|-------|
| `monitoring.install` | `true` | kube-prometheus-stack. `false` → set `monitoring.prometheusURL`. |
| `monitoring.dcgm.install` | `true` | dcgm-exporter. `false` if a GPU Operator provides DCGM. |
| `metallb.install` | `true` | Bare-metal LB. Requires `metallb.ipRange`. `false` on cloud. |
| `nfsProvisioner.install` | `true` | RWX StorageClass. Requires `nfsProvisioner.server`/`path`. |
| `hami.install` | `false` | Fractional GPU scheduler (opt-in). |
| `registry.install` | `false` | In-cluster registry for image builds (opt-in). |

## Storage

- `storage.persistence.storageClass` — RWX StorageClass for persistent home. Required in every
  mode. Match `nfsProvisioner.storageClassName` when bundling, or point to an existing RWX class.
- `storage.datasets.enabled` + `storage.datasets.nfs` — shared datasets (requires an NFS export).
- `storage.scratch.enabled` — node-local fast scratch.

## GPU labels

```yaml
k8s:
  gpuTypeLabel: nvidia.com/gpu.product        # GPU type (GFD)
  cudaLabel: nvidia.com/cuda.driver-version   # CUDA version (GFD)
```

## Scheduling

- `controlPlane.enabled` — pin API/gateway to control-plane nodes (adds nodeSelector +
  tolerations for the control-plane taint), keeping GPU workers for workloads.

## Access gateway (optional)

`gateway.enabled` deploys the single-entry gateway (per-session web subdomains + token SSH).
Requires wildcard DNS (`*.<domain>`) and, for HTTPS, a TLS secret. See `gateway.*` in values.
