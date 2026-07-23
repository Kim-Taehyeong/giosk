#!/usr/bin/env bash
# 실제 GPU 없이 테스트 — kind worker 노드에 가짜 GPU 확장 리소스를 광고한다.
# 기본 kube-scheduler 는 확장 리소스를 capacity 로 추적하므로,
# nvidia.com/gpu(전용) · nvidia.com/gpumem · nvidia.com/gpucores(HAMi 분할) 요청 파드가 스케줄된다.
# (device-plugin/HAMi 스케줄러 없이도 capacity 기반 배치 검증 가능)
set -euo pipefail
CTX="kind-giosk"

# 노드별 (gpu 장수, vGPU mem MiB, core %) 광고. node-agent/물리 노드는 제외해도 됨.
patch_node() {
  local node="$1" gpus="$2"
  echo "→ $node : nvidia.com/gpu=$gpus"
  kubectl --context "$CTX" patch node "$node" --subresource=status --type=json -p="[
    {\"op\":\"add\",\"path\":\"/status/capacity/nvidia.com~1gpu\",\"value\":\"$gpus\"},
    {\"op\":\"add\",\"path\":\"/status/capacity/nvidia.com~1gpumem\",\"value\":\"$((gpus*24576))\"},
    {\"op\":\"add\",\"path\":\"/status/capacity/nvidia.com~1gpucores\",\"value\":\"$((gpus*100))\"}
  ]" >/dev/null
}

# giosk.io/gpu-type 라벨이 있는 워커 노드 전부 대상
mapfile -t nodes < <(kubectl --context "$CTX" get nodes -l giosk.io/gpu-type -o name | sed 's|node/||')
for n in "${nodes[@]}"; do
  patch_node "$n" 4
done

echo "✓ fake GPU 광고 완료"
kubectl --context "$CTX" get nodes -L giosk.io/gpu-type -o custom-columns='NODE:.metadata.name,GPU-TYPE:.metadata.labels.giosk\.io/gpu-type,GPU:.status.capacity.nvidia\.com/gpu'
