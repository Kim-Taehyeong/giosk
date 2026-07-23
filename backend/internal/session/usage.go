package session

import (
	"context"
	"fmt"
	"strings"

	"giosk/internal/metrics"
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

// MyUsage는 호출자 본인의 running 세션 지표를 id→Usage 로 반환한다(세션 목록 폴링용).
func (s *Service) MyUsage(ctx context.Context, userID int64) (map[string]*Usage, error) {
	rows, err := s.repo.ListByUser(userID)
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

	var pods []string     // Pod 메트릭(CPU/RAM) 대상 = 컨테이너 세션 전부
	var dcgmPods []string // 전용 GPU 세션
	var hamiPods []string // 분할(HAMi) GPU 세션
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
		default: // exclusive | mig — 카드를 통째로 받으므로 DCGM = 그 세션 사용량
			u.GpuSource = gpuSrcDCGM
			dcgmPods = append(dcgmPods, r.InstanceID)
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

	s.fillDCGM(ctx, out, dcgmPods)
	s.fillHAMi(ctx, out, hamiPods)
	markUnavailable(out)
	return out
}

// fillDCGM은 전용 GPU 세션의 util/VRAM 을 DCGM(pod 라벨)에서 채운다.
// GPU 여러 장이면 util 은 평균, VRAM 은 합이 맞다.
func (s *Service) fillDCGM(ctx context.Context, out map[string]*Usage, pods []string) {
	if len(pods) == 0 {
		return
	}
	// 워크로드 Pod 라벨은 배포에 따라 pod/exported_pod 로 갈린다 → DCGMPodSeries 로 정규화.
	sel := strings.Join(pods, "|")
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`avg by(pod) (%s)`, metrics.DCGMPodSeries("DCGM_FI_DEV_GPU_UTIL", sel)), "pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.GpuUtil = ptr(val)
			}
		}
	}
	// DCGM_FI_DEV_FB_USED 단위는 MiB.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(pod) (%s)`, metrics.DCGMPodSeries("DCGM_FI_DEV_FB_USED", sel)), "pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.VramUsedMB = ptr(val)
			}
		}
	}
	// 전용 세션은 vram_mb 를 안 쓴다(카드 통째) → 총량을 DB 에서 못 얻는다. 카드 용량으로 채운다.
	// USED+FREE 는 라벨셋이 같아 기본 매칭으로 붙는다(node 별 집계와 같은 방식).
	if v, ok := s.met.VectorByLabel(ctx, fmt.Sprintf(`sum by(pod) (%s + %s)`,
		metrics.DCGMPodSeries("DCGM_FI_DEV_FB_USED", sel),
		metrics.DCGMPodSeries("DCGM_FI_DEV_FB_FREE", sel)), "pod"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil && u.VramTotalMB == 0 {
				u.VramTotalMB = int(val)
			}
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
	// smUtil(%) — vGPU 여러 개면 평균.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`avg by(podname) (Device_utilization_desc_of_container{podname=~"%s"})`, sel), "podname"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.GpuUtil = ptr(val)
			}
		}
	}
	// 사용량은 vGPU_device_memory_usage_in_bytes(바이트)다.
	// Device_memory_desc_of_container 는 이름과 달리 "총량"이라 사용량이 아니다.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(podname) (vGPU_device_memory_usage_in_bytes{podname=~"%s"})`, sel), "podname"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil {
				u.VramUsedMB = ptr(val / (1024 * 1024))
			}
		}
	}
	// 총량은 DB(vram_mb)가 권위지만, 오퍼링 없이 만든 세션 등 0 이면 HAMi 가 강제한 한도로 채운다.
	if v, ok := s.met.VectorByLabel(ctx,
		fmt.Sprintf(`sum by(podname) (vGPU_device_memory_limit_in_bytes{podname=~"%s"})`, sel), "podname"); ok {
		for pod, val := range v {
			if u := out[pod]; u != nil && u.VramTotalMB == 0 {
				u.VramTotalMB = int(val / (1024 * 1024))
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
