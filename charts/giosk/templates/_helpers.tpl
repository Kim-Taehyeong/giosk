{{/*
설치 시점 불변식 검증 — backend/internal/config/validate.go 와 동일 규칙.
Helm 설치 단계에서 잘못된 values 조합을 fail 시킨다.
*/}}
{{- define "giosk.validate" -}}
  {{- if not (has .Values.deployment.mode (list "container" "hybrid")) -}}
    {{- fail "deployment.mode 는 container 또는 hybrid 여야 합니다" -}}
  {{- end -}}
  {{- if not (has .Values.billing.mode (list "credit" "dynamic" "free")) -}}
    {{- fail "billing.mode 는 credit, dynamic, free 중 하나여야 합니다" -}}
  {{- end -}}
  {{- if .Values.storage.datasets.enabled -}}
    {{- $nfs := .Values.storage.datasets.nfs -}}
    {{- if or (not $nfs.server) (not $nfs.path) -}}
      {{- fail "datasets 기능을 켜려면 storage.datasets.nfs.server 와 path 가 반드시 필요합니다 (데이터셋은 <path>/dataset/<name> 정규경로에 저장되어 물리노드가 직접 마운트). 쓰기 가능한 NFS export 를 지정하세요." -}}
    {{- end -}}
  {{- end -}}
  {{- if .Values.physicalNodes.enabled -}}
    {{- if ne .Values.deployment.mode "hybrid" -}}
      {{- fail "physicalNodes.enabled 는 deployment.mode=hybrid 를 요구합니다" -}}
    {{- end -}}
    {{- if or (not .Values.physicalNodes.nfs.server) (not .Values.physicalNodes.nfs.path) -}}
      {{- fail "physicalNodes 는 외부 NFS server 와 path 가 필요합니다" -}}
    {{- end -}}
  {{- end -}}
  {{- /* NFS 는 어떤 모드에서도 필수 — 영속 home(~/nfs)이 RWX. 번들 provisioner 를 안 쓰면 기존 RWX SC 지정 필수. */ -}}
  {{- if not .Values.storage.persistence.storageClass -}}
    {{- fail "storage.persistence.storageClass 가 필요합니다 — NFS 는 모든 모드에서 필수(영속 home ~/nfs = RWX). nfsProvisioner.install=true 로 번들하거나 기존 RWX StorageClass 이름을 지정하세요." -}}
  {{- end -}}
  {{- /* 인프라 번들 토글 — 번들 설치 시 사이트별 값 필수(배포 스크립트가 이 값으로 별도 릴리스를 올린다). */ -}}
  {{- if .Values.nfsProvisioner.install -}}
    {{- if or (not .Values.nfsProvisioner.server) (not .Values.nfsProvisioner.path) -}}
      {{- fail "nfsProvisioner.install=true 면 nfsProvisioner.server 와 path 가 필요합니다 (NFS 서버는 외부에 준비돼 있어야 함)." -}}
    {{- end -}}
  {{- end -}}
  {{- if and .Values.metallb.install (not .Values.metallb.ipRange) -}}
    {{- fail "metallb.install=true 면 metallb.ipRange 가 필요합니다 (예: 192.168.0.200-192.168.0.220). 클라우드 LB 를 쓰면 metallb.install=false." -}}
  {{- end -}}
{{- end -}}

{{- define "giosk.fullname" -}}{{ .Release.Name }}-giosk{{- end -}}

{{- /*
giosk.controlPlaneScheduling — 컨트롤플레인(마스터) 노드 고정 스케줄 블록.
설치 시점 상주 컴포넌트(api·gateway 등 워커=GPU 노드에 두면 곤란한 것)를 컨트롤노드로 몰아넣는다.
controlPlane.enabled=true 일 때만 nodeSelector+tolerations 를 방출한다(기본 off → 클러스터 기본 스케줄).
호출: {{- include "giosk.controlPlaneScheduling" . | nindent 6 }}  (spec: 아래, 컨테이너와 같은 들여쓰기)
*/ -}}
{{- define "giosk.controlPlaneScheduling" -}}
{{- if .Values.controlPlane.enabled -}}
nodeSelector:
  {{- toYaml .Values.controlPlane.nodeSelector | nindent 2 }}
tolerations:
  {{- toYaml .Values.controlPlane.tolerations | nindent 2 }}
{{- end -}}
{{- end -}}
