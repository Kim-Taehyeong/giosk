# Installation

## Prerequisites (must exist before install)

- **Kubernetes cluster** with a working CNI on every schedulable node.
- **NFS server** (writable export) — required in **every** mode; persistent home (`~/nfs`) is RWX.
- **GPU nodes** (for GPU sessions): NVIDIA driver + container runtime + device-plugin +
  GPU Feature Discovery labels (`nvidia.com/gpu.product`). Without these, sessions are CPU-only.
- **LoadBalancer**: on bare metal, an IP range for MetalLB (bundled); on cloud, the provider LB.
- `helm` and `kubectl` (and, for the self-host installer, `ctr` on the control node).

Giosk can **bundle** the rest (MetalLB, NFS provisioner, Prometheus, DCGM, HAMi) — each is a
`install: true/false` toggle in values. Bundle-able infra defaults to on.

## Configure

```bash
cp deploy/values.example.yaml my-values.yaml
$EDITOR my-values.yaml     # NFS server/path, MetalLB ipRange, mode, GPU labels
```

Required fields are enforced at install time — the chart refuses to render with an unsafe
config (e.g. missing NFS StorageClass, `metallb.install=true` without `ipRange`).

## Install

### Option A — self-host installer (air-gapped friendly, no registry)

Builds images on the control node and distributes them to nodes via `ctr import`.

```bash
sudo VALUES=./my-values.yaml ./deploy/deploy.sh
# flags: --with-gateway  --with-node-agent  --skip-build  --no-monitoring
```

### Option B — Helm (images pulled from a registry)

```bash
helm install giosk charts/giosk -f my-values.yaml --set admin.password=<PW>
```

Set `image.*.repository` to your registry (e.g. `ghcr.io/<org>/giosk-api`) for this path.

## k3s notes

k3s works and is often simpler (flannel CNI + ServiceLB + local-path are built in), with two
adjustments for the self-host installer:

- kubeconfig: `export KUBECONFIG=/etc/rancher/k3s/k3s.yaml`
- image import: use `k3s ctr images import` (k3s has its own containerd socket)
- LoadBalancer: k3s ServiceLB can replace MetalLB (`metallb.install: false`)
- Storage: local-path is RWO — home still needs RWX, so keep `nfsProvisioner.install: true`

## Verify

```bash
kubectl -n giosk get pods
# frontend LoadBalancer IP:
kubectl -n giosk get svc giosk-giosk-frontend
```
