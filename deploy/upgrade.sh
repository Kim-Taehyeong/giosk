#!/usr/bin/env bash
# 이미 설치된 릴리스를 지금 체크아웃된 커밋으로 올린다(레지스트리에서 이미지를 받는 배포).
#
# 이미지 태그로 :latest 대신 커밋 SHA(sha-xxxxxxx)를 쓴다. latest 는 Deployment 스펙이 그대로라
# helm 이 파드를 다시 띄우지 않아, 새 이미지가 올라가 있어도 옛 코드가 계속 도는 일이 생긴다.
# SHA 태그면 스펙이 바뀌므로 helm 이 알아서 롤링하고, 무엇이 도는지 파드만 봐도 알 수 있다.
#
#   사용: ./deploy/upgrade.sh              (origin/main 최신으로)
#         REF=v0.1.0 ./deploy/upgrade.sh   (특정 태그·커밋으로)
#         SKIP_PULL=1 ./deploy/upgrade.sh  (지금 워킹트리 그대로)
set -euo pipefail

cd "$(dirname "$0")/.."
NS=${NS:-giosk}
RELEASE=${RELEASE:-giosk}
VALUES=${VALUES:-my-values.yaml}
REF=${REF:-origin/main}

[ -f "$VALUES" ] || { echo "values 파일이 없다: $VALUES" >&2; exit 1; }
helm status "$RELEASE" -n "$NS" >/dev/null 2>&1 || {
  echo "릴리스 $RELEASE 가 없다. 첫 설치는 helm install 로." >&2; exit 1; }

if [ -z "${SKIP_PULL:-}" ]; then
  git fetch --tags origin
  git reset --hard "$REF"
fi
SHA=$(git rev-parse --short=7 HEAD)
TAG="sha-$SHA"
echo "배포 대상: $(git log --oneline -1)"
echo "이미지 태그: $TAG"

# 이미지가 실제로 올라와 있는지 먼저 본다. 없으면 파드가 ImagePullBackOff 로 죽는다.
# (main push 후 GitHub Actions build-images 가 끝나야 존재한다.)
owner=$(git remote get-url origin | sed -E 's#.*[/:]([^/]+)/[^/]+(\.git)?$#\1#' | tr 'A-Z' 'a-z')
for img in api frontend; do
  repo="$owner/giosk-$img"
  tok=$(curl -fsSL "https://ghcr.io/token?scope=repository:$repo:pull" | sed 's/.*"token":"\([^"]*\)".*/\1/')
  curl -fsSI -o /dev/null -H "Authorization: Bearer $tok" \
    -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
    "https://ghcr.io/v2/$repo/manifests/$TAG" || {
      echo "ghcr.io/$repo:$TAG 가 아직 없다. build-images 워크플로가 끝났는지 확인하라." >&2; exit 1; }
done

helm upgrade "$RELEASE" charts/giosk -n "$NS" -f "$VALUES" \
  --set image.api.tag="$TAG" \
  --set image.frontend.tag="$TAG" \
  --set image.nodeAgent.tag="$TAG" \
  --set image.gateway.tag="$TAG" \
  --set image.sshd.tag="$TAG" \
  --wait --timeout "${TIMEOUT:-10m}"

# 무엇이 도는지 이미지 태그로 확인한다. 파드 라벨은 app=giosk-<컴포넌트> 라 이름으로 거른다.
echo
kubectl -n "$NS" get pods \
  -o custom-columns=NAME:.metadata.name,READY:.status.containerStatuses[0].ready,IMAGE:.spec.containers[0].image \
  | grep -E "NAME|$TAG" || true
