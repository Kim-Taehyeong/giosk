package session

import (
	"context"
	"fmt"
	"strings"
)

// GPU 지표 출처 — 세션의 gpu_mode 가 결정한다.
const (
	gpuSrcDCGM = "dcgm" // 전용(카드 통째) — DCGM 이 곧 그 세션의 사용량
	gpuSrcHAMi = "hami" // 분할(HAMi) — vGPUmonitor 가 컨테이너별 실사용을 잰다
	gpuSrcNone = "none" // 측정 불가 — Reason 참조
)

// GPU 측정 불가 사유(gpuSrcNone).
const (
	gpuNoneCPU         = "cpu"         // CPU 전용 세션 — GPU 없음
	gpuNonePhysical    = "physical"    // 물리(SSH) 임대 — Pod 이 없어 Pod 메트릭 자체가 없다
	gpuNoneTimeslice   = "timeslice"   // 타임슬라이싱 — 아래 주석 참조
	gpuNoneUnavailable = "unavailable" // Prometheus 미연동 / 시리즈 없음(방금 뜬 세션 등)
)

// usageWindow는 CPU rate 산출 윈도(분). 스크레이프 간격(15s)의 배수로 넉넉히 잡는다.
const usageWindow = 2

// Usage는 세션 1개의 실사용 지표. 측정 불가한 값은 nil 이다 — 0 과 구분해야 한다
// ("안 쓰는 중"과 "못 재는 중"은 관제상 전혀 다른 상태다).
type Usage struct {
	ID string `json:"id"`

	CPUCores      *float64 `json:"cpuCores"` // 사용 코어(rate)
	CPULimitCores int      `json:"cpuLimitCores"`
	MemUsedMB     *float64 `json:"memUsedMb"`
	MemLimitMB    int      `json:"memLimitMb"`

	GpuUtil     *float64 `json:"gpuUtil"` // %
	VramUsedMB  *float64 `json:"vramUsedMb"`
	VramTotalMB int      `json:"vramTotalMb"`

	GpuSource string `json:"gpuSource"`           // dcgm | hami | none
	GpuReason string `json:"gpuReason,omitempty"` // none 인 사유
}

// Usage는 소유자 본인의 세션 1개 실사용 지표를 반환한다.
func (s *Service) Usage(ctx context.Context, instanceID string, userID int64) (*Usage, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return nil, err
	}
	return s.usageOf(ctx, []Session{*sess})[instanceID], nil
}

// SessionMetric은 알림 엔진용 — 특정 세션의 지표 1개(session_gpu/cpu/vram, %)를 평가한다.
// 신뢰 호출(엔진)이라 소유자 검증 없음. running 이 아니거나 측정 불가면 ok=false(발화 안 함).
func (s *Service) SessionMetric(ctx context.Context, instanceID, metric string) (float64, bool) {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil || sess.Phase != PhaseRunning {
		return 0, false
	}
	u := s.usageOf(ctx, []Session{*sess})[instanceID]
	if u == nil {
		return 0, false
	}
	switch metric {
	case "session_gpu":
		if u.GpuUtil == nil {
			return 0, false
		}
		return *u.GpuUtil, true
	case "session_cpu":
		if u.CPUCores == nil || u.CPULimitCores == 0 {
			return 0, false
		}
		return *u.CPUCores / float64(u.CPULimitCores) * 100, true
	case "session_vram":
		if u.VramUsedMB == nil || u.VramTotalMB == 0 {
			return 0, false
		}
		return *u.VramUsedMB / float64(u.VramTotalMB) * 100, true
	}
	return 0, false
}

// MyUsage는 호출자 본인의 running 세션 지표를 id→Usage 로 반환한다(세션 목록 폴링용).
func (s *Service) MyUsage(ctx context.Context, userID int64) (map[string]*Usage, error) {
	rows, err := s.repo.ListByUser(userID, 0)
	if err != nil {
		return nil, err
	}
	return s.usageOf(ctx, runningOnly(rows)), nil
}

// AdminUsage는 running 세션 전체의 실사용 지표를 id→Usage 로 반환한다(관제 화면).
func (s *Service) AdminUsage(ctx context.Context) (map[string]*Usage, error) {
	rows, err := s.repo.ListRunning()
	if err != nil {
		return nil, err
	}
	return s.usageOf(ctx, rows), nil
}

// runningOnly는 실행 중인 세션만 남긴다 — 정지된 세션은 잴 지표가 없다.
func runningOnly(rows []Session) []Session {
	out := make([]Session, 0, len(rows))
	for i := range rows {
		if rows[i].Phase == PhaseRunning {
			out = append(out, rows[i])
		}
	}
	return out
}

// usageOf는 세션 목록의 지표를 채운다. 세션 수와 무관하게 Prometheus 쿼리는 최대 6번이다
// (pod 라벨로 그룹핑) — 세션마다 쿼리하면 관제 화면 한 번에 수십 건이 나간다.
func (s *Service) usageOf(ctx context.Context, rows []Session) map[string]*Usage {
	out := make(map[string]*Usage, len(rows))

	var pods []string                // Pod 메트릭(CPU/RAM) 대상 = 컨테이너 세션 전부
	dcgmNodes := map[string]string{} // 전용 GPU 세션: instanceID→node(세션이 놓인 노드의 GPU 로 귀속)
	var hamiPods []string            // 분할(HAMi) GPU 세션
	for i := range rows {
		r := rows[i]
		u := &Usage{
			ID: r.InstanceID, CPULimitCores: r.CPUCores, MemLimitMB: r.MemGB * 1024,
			VramTotalMB: r.VramMB, GpuSource: gpuSrcNone,
		}
		out[r.InstanceID] = u

		if r.Env == "ssh" {
			// 물리 임대는 노드를 통째로 쓴다 — 노드 지표가 곧 이 세션의 지표지만
			// 컨테이너 경계가 없어 Pod 메트릭으로는 잡히지 않는다.
			u.GpuReason = gpuNonePhysical
			continue
		}
		pods = append(pods, r.InstanceID)

		switch r.GpuMode {
		case "cpu":
			u.GpuReason = gpuNoneCPU
		case "shared":
			u.GpuSource = gpuSrcHAMi
			hamiPods = append(hamiPods, r.InstanceID)
		case "timeslice":
			// 타임슬라이싱은 컨테이너별 계측 지점이 없다. DCGM 은 물리 카드 단위로만 보고하고,
			// 한 카드에 슬롯 N개가 붙으므로 카드 전체 사용률이 모든 세션에 똑같이 복사된다.
			// 그럴듯하지만 틀린 값이라 아예 내주지 않는다.
			u.GpuReason = gpuNoneTimeslice
		default: // exclusive | mig — 카드를 통째로 받으므로 노드 GPU = 그 세션 사용량
			u.GpuSource = gpuSrcDCGM
			dcgmNodes[r.InstanceID] = r.Node
		}
	}

	if len(pods) == 0 || !s.met.Enabled() {
		markUnavailable(out)
		return out
	}

	// CPU/RAM — cgroup 기반이라 GPU 공유 방식과 무관하게 항상 정확하다.
	// container!="" 로 pause 컨테이너를 제외한다(안 그러면 Pod 합계가 중복된다).
	sel := strings.Join(pods, "|")
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(pod) (rate(container_cpu_usage_seconds_total{pod=~"%s",container!=""}[%dm]))`, sel, usageWindow),
		"pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.CPUCores = ptr(val)
			}
		}
	}
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(pod) (container_memory_working_set_bytes{pod=~"%s",container!=""})`, sel),
		"pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.MemUsedMB = ptr(val / (1024 * 1024))
			}
		}
	}

	s.fillDCGM(ctx, out, dcgmNodes)
	s.fillHAMi(ctx, out, hamiPods)
	markUnavailable(out)
	return out
}

// fillDCGM은 전용 GPU 세션의 util/VRAM 을 그 세션이 놓인 "노드"의 DCGM 지표로 채운다.
// nodes: instanceID→node. 왜 pod 가 아니라 node 인가: HAMi 가 클러스터 전체 device-plugin 이면
// DCGM 의 pod-resources 기반 GPU→pod 매핑이 HAMi vGPU device ID 와 안 맞아 워크로드 pod 라벨이
// 안 붙는다(exported_pod 없음). 전용 세션은 노드 GPU 를 통째로 쓰므로 노드 GPU 지표 = 세션 지표다.
// dcgm-exporter pod→node 는 kube_pod_info 로 매핑(노드 대시보드와 동일 방식).
// ⚠️ 노드에 GPU 가 여러 장이면 노드 평균/합이라 같은 노드의 전용 세션들엔 동일 값이 간다
//    (노드당 1 GPU 전용 배치에서 정확 — 세션별 카드 귀속은 DCGM pod 매핑이 필요한데 HAMi 가 막음).
func (s *Service) fillDCGM(ctx context.Context, out map[string]*Usage, nodes map[string]string) {
	if len(nodes) == 0 {
		return
	}
	const j = ` * on(pod,namespace) group_left(node) kube_pod_info`
	util, _ := s.met.VectorByLabel(ctx, `avg by(node) (DCGM_FI_DEV_GPU_UTIL`+j+`)`, "node")
	vramUsed, _ := s.met.VectorByLabel(ctx, `sum by(node) (DCGM_FI_DEV_FB_USED`+j+`)`, "node") // MiB
	vramTot, _ := s.met.VectorByLabel(ctx, `sum by(node) ((DCGM_FI_DEV_FB_USED+DCGM_FI_DEV_FB_FREE)`+j+`)`, "node")
	for iid, node := range nodes {
		u := out[iid]
		if u == nil || node == "" {
			continue
		}
		if v, ok := util[node]; ok {
			u.GpuUtil = ptr(v)
		}
		if v, ok := vramUsed[node]; ok {
			u.VramUsedMB = ptr(v)
		}
		if v, ok := vramTot[node]; ok && u.VramTotalMB == 0 { // 전용은 vram_mb 미사용 → 카드 총량으로
			u.VramTotalMB = int(v)
		}
	}
}

// fillHAMi는 분할 세션의 vGPU util/VRAM 을 HAMi vGPUmonitor 에서 채운다.
// HAMi 는 CUDA hook(libvgpu.so)으로 컨테이너별 실사용을 재므로 한 카드를 나눠 써도 값이 갈린다.
// 라벨이 pod 가 아니라 podname 이다(DCGM 과 다름).
func (s *Service) fillHAMi(ctx context.Context, out map[string]*Usage, pods []string) {
	if len(pods) == 0 {
		return
	}
	sel := strings.Join(pods, "|")
	// HAMi v2.9.0 vGPUmonitor 메트릭(hami_* 접두). 세션 Pod 이름은 exported_pod 라벨이다
	// (Prometheus 가 스크레이프 대상 pod 라벨과 충돌을 피해 워크로드 pod 을 exported_pod 로 옮긴다).
	// util 은 이름이 ratio 지만 0-100(%). vGPU 여러 개면 평균.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`avg by(exported_pod) (hami_container_device_utilization_ratio{exported_pod=~"%s"})`, sel), "exported_pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.GpuUtil = ptr(val)
			}
		}
	}
	// 사용 VRAM(바이트) → MB. vGPU 여러 개면 합.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(exported_pod) (hami_vgpu_memory_used_bytes{exported_pod=~"%s"})`, sel), "exported_pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.VramUsedMB = ptr(val / (1024 * 1024))
			}
		}
	}
	// 총량은 DB(vram_mb)가 권위지만, 오퍼링 없이 만든 세션 등 0 이면 HAMi 강제 한도(바이트)로 채운다.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(exported_pod) (hami_vgpu_memory_limit_bytes{exported_pod=~"%s"})`, sel), "exported_pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil && u.VramTotalMB == 0 {
				u.VramTotalMB = int(val / (1024 * 1024))
			}
		}
	}

	// 유휴 보정 — HAMi vGPUmonitor 는 컨테이너가 CUDA(libvgpu)를 실제로 올린 동안만
	// 컨테이너별 시리즈를 낸다. 갓 뜬/유휴 세션은 시리즈가 아예 없어 markUnavailable 이
	// "미가용"으로 되돌리지만, 실제로는 GPU 를 안 쓰는 0% 상태다. 모니터가 살아있음
	// (host 시리즈 존재)을 확인했을 때만 남은 세션을 0%(유휴)로 채운다 — 모니터 자체가
	// 죽었으면(스크레이프 실패) 그대로 "미가용"으로 남겨 관제상 구분한다.
	needIdle := false
	for _, p := range pods {
		if u := out[p]; u != nil && u.GpuUtil == nil {
			needIdle = true
			break
		}
	}
	if !needIdle {
		return
	}
	if mon, ok := s.met.VectorByLabel(ctx, `hami_host_gpu_utilization_ratio`, "device_uuid"); ok && len(mon) > 0 {
		for _, p := range pods {
			if u := out[p]; u != nil && u.GpuUtil == nil {
				u.GpuUtil = ptr(0)
				if u.VramUsedMB == nil {
					u.VramUsedMB = ptr(0)
				}
			}
		}
	}
}

// markUnavailable은 출처는 있으나 시리즈를 못 받은 세션을 "측정 불가"로 되돌린다
// (Prometheus 미연동, exporter 미배포, 방금 떠서 아직 스크레이프 전 등).
func markUnavailable(out map[string]*Usage) {
	for _, u := range out {
		if u.GpuSource != gpuSrcNone && u.GpuUtil == nil && u.VramUsedMB == nil {
			u.GpuSource, u.GpuReason = gpuSrcNone, gpuNoneUnavailable
		}
	}
}

func ptr(v float64) *float64 { return &v }
