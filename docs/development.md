# Development

## Layout

```
backend/    Go — cmd/{api,node-agent}, internal/*, pkg/*
frontend/   React (Vite) — src/*
charts/     Helm chart
scripts/    dev / local-QA helpers
```

## Backend

```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

The API talks to Kubernetes via client-go (in-cluster, else `~/.kube/config`). For local work
without a cluster, most non-k8s features degrade gracefully.

## Frontend

```bash
cd frontend
npm install
npm run dev      # Vite dev server
npm run build    # production bundle
npm run lint
```

## Local QA bed

`scripts/` contains helpers to run the stack locally (MySQL + API + Vite):

```bash
scripts/qa-local.sh      # bring up local MySQL + API + frontend
scripts/qa-seed.sh       # seed demo users/orgs
scripts/run-api.sh       # run the API against a local DB
scripts/fast-frontend.sh # frontend-only fast loop
```

## Helm chart

```bash
helm lint charts/giosk
helm template giosk charts/giosk -f deploy/values.example.yaml \
  --set metallb.ipRange=10.0.0.200-10.0.0.210 \
  --set nfsProvisioner.server=10.0.0.5 --set nfsProvisioner.path=/export \
  --set admin.password=x
```

## Conventions

- Match the surrounding code's style; keep comments where the existing code has them.
- Backend: `go vet` + `go test` clean before a PR.
- Frontend: `npm run build` must pass; avoid leaking secrets/internal IPs into committed files.
