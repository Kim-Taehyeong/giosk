package session

import (
	"context"
	"fmt"
	"math"
	"time"

	"giosk/internal/metrics"
)

// HistPoint는 세션 사용률 이력의 한 시점(정렬된 시계열). 값은 % 로 정규화한다
// (한도 대비 비율이 있으면 그 비율, 없으면 원값). 측정 불가 구간은 0 으로 남긴다.
type HistPoint struct {
	T       int64 `json:"t"`       // unix seconds
	Cpu     int   `json:"cpu"`     // CPU 사용률 %(사용코어/한도)
	Mem     int   `json:"mem"`     // 메모리 사용률 %
	GpuUtil int   `json:"gpuUtil"` // GPU 사용률 %
	VramPct int   `json:"vramPct"` // VRAM 사용률 %
}

// HistInsights는 이력 상단 요약(구간 집계).
type HistInsights struct {
	Window     string `json:"window"`     // 예: "24h"
	AvgGpu     int    `json:"avgGpu"`     // 평균 GPU 사용률 %
	PeakGpu    int    `json:"peakGpu"`    // 최대 GPU 사용률 %
	GpuIdlePct int    `json:"gpuIdlePct"` // GPU 유휴(util<5%) 표본 비율 %
	AvgCpu     int    `json:"avgCpu"`     // 평균 CPU 사용률 %
	AvgMem     int    `json:"avgMem"`     // 평균 메모리 사용률 %
	PeakVram   int    `json:"peakVram"`   // 최대 VRAM 사용률 %
	Samples    int    `json:"samples"`    // 표본 수(0이면 메트릭 미가용/측정불가)
	GpuSource  string `json:"gpuSource"`  // dcgm | hami | none
	GpuReason  string `json:"gpuReason,omitempty"`
}

// UserHistory는 소유자 본인 세션의 사용률 이력을 반환한다(사용자 세션 상세 페이지).
func (s *Service) UserHistory(ctx context.Context, instanceID string, userID int64, hours int) ([]HistPoint, HistInsights, error) {
	sess, err := s.repo.Get(instanceID, userID)
	if err != nil {
		return nil, HistInsights{}, err
	}
	pts, ins := s.History(ctx, sess, hours)
	return pts, ins, nil
}

// AdminHistory는 임의 세션의 사용률 이력을 반환한다(관제 세션 상세 페이지).
func (s *Service) AdminHistory(ctx context.Context, instanceID string, hours int) ([]HistPoint, HistInsights, error) {
	sess, err := s.repo.GetByInstance(instanceID)
	if err != nil {
		return nil, HistInsights{}, err
	}
	pts, ins := s.History(ctx, sess, hours)
	return pts, ins, nil
}

// History는 세션 1개의 사용률 24h(기본) 시계열 + 요약을 반환한다.
//
// DB 에 저장하지 않고 Prometheus Range 쿼리로 매번 계산한다(이력 테이블 폭주 방지).
// 물리(SSH)·타임슬라이싱·CPU 세션은 GPU 를 못 재므로 GpuReason 으로 사유를 알린다.
// Prometheus 미연동이거나 표본이 없으면 빈 시리즈(Samples=0)를 반환한다.
func (s *Service) History(ctx context.Context, sess *Session, hours int) ([]HistPoint, HistInsights) {
	if hours <= 0 {
		hours = 24
	}
	ins := HistInsights{Window: fmt.Sprintf("%dh", hours), GpuSource: gpuSrcNone}

	// GPU 출처 판정 — usageOf 와 같은 규칙.
	switch {
	case sess.Env == "ssh":
		ins.GpuReason = gpuNonePhysical
	case sess.GpuMode == "cpu":
		ins.GpuReason = gpuNoneCPU
	case sess.GpuMode == "timeslice":
		ins.GpuReason = gpuNoneTimeslice
	case sess.GpuMode == "shared":
		ins.GpuSource = gpuSrcHAMi
	default:
		ins.GpuSource = gpuSrcDCGM
	}

	if s.met == nil || !s.met.Enabled() || sess.Env == "ssh" {
		if sess.Env == "ssh" {
			return []HistPoint{}, ins // 물리는 Pod 메트릭이 없다
		}
		return []HistPoint{}, ins
	}

	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)
	// step: 구간을 ~120표본으로. 최소 60초.
	step := time.Duration(hours) * time.Hour / 120
	if step < time.Minute {
		step = time.Minute
	}
	pod := sess.InstanceID

	// CPU/RAM — cgroup 기반, GPU 방식 무관하게 항상 정확.
	cpuCores := s.rangeMap(ctx, fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{pod=%q,container!=""}[5m]))`, pod), start, end, step)
	memBytes := s.rangeMap(ctx, fmt.Sprintf(`sum(container_memory_working_set_bytes{pod=%q,container!=""})`, pod), start, end, step)

	// GPU — 출처별 쿼리.
	var gpuUtil, vramUsedMB map[int64]float64
	switch ins.GpuSource {
	case gpuSrcDCGM:
		gpuUtil = s.rangeMap(ctx, fmt.Sprintf(`avg(%s)`, metrics.DCGMPodSeries("DCGM_FI_DEV_GPU_UTIL", pod)), start, end, step)
		vramUsedMB = s.rangeMap(ctx, fmt.Sprintf(`sum(%s)`, metrics.DCGMPodSeries("DCGM_FI_DEV_FB_USED", pod)), start, end, step)
	case gpuSrcHAMi:
		// HAMi v2.9.0 메트릭명(hami_* 접두, 라벨 exported_pod). 옛 Device_utilization_desc_of_container/
		// vGPU_device_memory_usage_in_bytes{podname=} 는 v2.9.0 에서 사라져 이력이 항상 빈 값→"미가용"이 됐다.
		gpuUtil = s.rangeMap(ctx, fmt.Sprintf(`avg(hami_container_device_utilization_ratio{exported_pod=%q})`, pod), start, end, step)
		vramB := s.rangeMap(ctx, fmt.Sprintf(`sum(hami_vgpu_memory_used_bytes{exported_pod=%q})`, pod), start, end, step)
		vramUsedMB = map[int64]float64{}
		for t, v := range vramB {
			vramUsedMB[t] = v / (1024 * 1024)
		}
	}

	// 한도(정규화 분모). 0 이면 원값을 %로 간주(그대로 사용).
	cpuLimit := float64(sess.CPUCores)
	memLimitMB := float64(sess.MemGB * 1024)
	vramTotalMB := float64(sess.VramMB)

	// 타임스탬프 합집합 정렬.
	tset := map[int64]struct{}{}
	for _, m := range []map[int64]float64{cpuCores, memBytes, gpuUtil, vramUsedMB} {
		for t := range m {
			tset[t] = struct{}{}
		}
	}
	ts := make([]int64, 0, len(tset))
	for t := range tset {
		ts = append(ts, t)
	}
	sortInt64Hist(ts)

	points := make([]HistPoint, 0, len(ts))
	var sumGpu, sumCpu, sumMem float64
	var peakGpu, peakVram, idleN, gpuSamples int
	for _, t := range ts {
		p := HistPoint{T: t}
		if c, ok := cpuCores[t]; ok {
			if cpuLimit > 0 {
				p.Cpu = clampPct(c / cpuLimit * 100)
			} else {
				p.Cpu = round(c * 100)
			}
		}
		if m, ok := memBytes[t]; ok {
			mb := m / (1024 * 1024)
			if memLimitMB > 0 {
				p.Mem = clampPct(mb / memLimitMB * 100)
			} else {
				p.Mem = round(mb)
			}
		}
		if g, ok := gpuUtil[t]; ok {
			p.GpuUtil = clampPct(g)
			gpuSamples++
			sumGpu += g
			if p.GpuUtil > peakGpu {
				peakGpu = p.GpuUtil
			}
			if g < 5 {
				idleN++
			}
		}
		if vu, ok := vramUsedMB[t]; ok && vramTotalMB > 0 {
			p.VramPct = clampPct(vu / vramTotalMB * 100)
			if p.VramPct > peakVram {
				peakVram = p.VramPct
			}
		}
		sumCpu += float64(p.Cpu)
		sumMem += float64(p.Mem)
		points = append(points, p)
	}

	n := len(points)
	ins.Samples = n
	if n > 0 {
		ins.AvgCpu = round(sumCpu / float64(n))
		ins.AvgMem = round(sumMem / float64(n))
		ins.PeakVram = peakVram
	}
	if gpuSamples > 0 {
		ins.AvgGpu = round(sumGpu / float64(gpuSamples))
		ins.PeakGpu = peakGpu
		ins.GpuIdlePct = round(float64(idleN) / float64(gpuSamples) * 100)
	}
	// 출처는 있으나 GPU 표본이 하나도 없으면 측정 불가로 되돌린다.
	if ins.GpuSource != gpuSrcNone && gpuSamples == 0 {
		ins.GpuSource, ins.GpuReason = gpuSrcNone, gpuNoneUnavailable
	}
	return points, ins
}

// rangeMap은 range 쿼리 결과를 unix초→값 맵으로 반환한다(미가용이면 빈 맵).
func (s *Service) rangeMap(ctx context.Context, q string, start, end time.Time, step time.Duration) map[int64]float64 {
	out := map[int64]float64{}
	pts, ok := s.met.Range(ctx, q, start, end, step)
	if !ok {
		return out
	}
	for _, p := range pts {
		out[int64(p.T)] = p.V
	}
	return out
}

func clampPct(v float64) int {
	r := round(v)
	if r < 0 {
		return 0
	}
	if r > 100 {
		return 100
	}
	return r
}

func round(v float64) int { return int(math.Round(v)) }

func sortInt64Hist(a []int64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
