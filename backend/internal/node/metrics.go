package node

import (
	"context"
	"fmt"
	"math"
	"time"
)

// MetricPoint는 노드 상세 이력 차트의 한 시점(정렬된 시계열).
type MetricPoint struct {
	T       int64 `json:"t"` // unix seconds
	Util    int   `json:"util"`
	Temp    int   `json:"temp"`
	Power   int   `json:"power"`   // W(노드 내 GPU 합)
	VramPct int   `json:"vramPct"` // VRAM 할당 %
	Cpu     int   `json:"cpu"`
	Mem     int   `json:"mem"`
}

// NodeInsights는 상세 페이지 상단의 요약 인사이트(구간 집계).
type NodeInsights struct {
	Window    string  `json:"window"`    // 예: "24h"
	AvgUtil   int     `json:"avgUtil"`   // 평균 GPU 사용률 %
	PeakUtil  int     `json:"peakUtil"`  // 최대 GPU 사용률 %
	IdlePct   int     `json:"idlePct"`   // 유휴(util<5%) 표본 비율 %
	AvgTemp   int     `json:"avgTemp"`   // 평균 온도 °C
	MaxTemp   int     `json:"maxTemp"`   // 최대 온도 °C
	AvgPower  int     `json:"avgPower"`  // 평균 전력 W
	PeakPower int     `json:"peakPower"` // 최대 전력 W
	EnergyKwh float64 `json:"energyKwh"` // 구간 소비 전력량 kWh(평균전력×시간)
	PeakHour  int     `json:"peakHour"`  // 사용률이 가장 높은 시간대(0~23시), 데이터 없으면 -1
	Samples   int     `json:"samples"`   // 표본 수(0이면 메트릭 미가용)
}

// NodeMetrics는 노드 상세 페이지용 시계열 + 인사이트를 반환한다.
// DCGM/node-exporter 메트릭엔 node 라벨이 없어 kube_pod_info 조인으로 노드를 붙인다.
// Prometheus 미연동이거나 표본이 없으면 빈 시리즈(인사이트 Samples=0)를 반환한다.
func (s *Service) NodeMetrics(ctx context.Context, node string, hours int) ([]MetricPoint, NodeInsights) {
	if hours <= 0 {
		hours = 24
	}
	ins := NodeInsights{Window: fmt.Sprintf("%dh", hours), PeakHour: -1}
	if s.met == nil || !s.met.Enabled() {
		return []MetricPoint{}, ins
	}
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)
	// step: 구간을 ~120표본으로. 최소 60초.
	step := time.Duration(hours) * time.Hour / 120
	if step < time.Minute {
		step = time.Minute
	}
	join := fmt.Sprintf(` * on(pod,namespace) group_left(node) kube_pod_info{node=%q}`, node)

	util := s.rangeSeries(ctx, "avg by(node)(DCGM_FI_DEV_GPU_UTIL"+join+")", start, end, step)
	temp := s.rangeSeries(ctx, "max by(node)(DCGM_FI_DEV_GPU_TEMP"+join+")", start, end, step)
	power := s.rangeSeries(ctx, "sum by(node)(DCGM_FI_DEV_POWER_USAGE"+join+")", start, end, step)
	vram := s.rangeSeries(ctx, "100 * sum by(node)(DCGM_FI_DEV_FB_USED"+join+") / sum by(node)((DCGM_FI_DEV_FB_USED+DCGM_FI_DEV_FB_FREE)"+join+")", start, end, step)
	cpu := s.rangeSeries(ctx, "100 - avg by(node)(rate(node_cpu_seconds_total{mode=\"idle\"}[5m])"+join+")*100", start, end, step)
	mem := s.rangeSeries(ctx, "100 * (1 - sum by(node)(node_memory_MemAvailable_bytes"+join+") / sum by(node)(node_memory_MemTotal_bytes"+join+"))", start, end, step)

	// 타임스탬프 합집합(정렬)으로 포인트를 정렬한다.
	tset := map[int64]struct{}{}
	for _, m := range []map[int64]float64{util, temp, power, vram, cpu, mem} {
		for t := range m {
			tset[t] = struct{}{}
		}
	}
	ts := make([]int64, 0, len(tset))
	for t := range tset {
		ts = append(ts, t)
	}
	sortInt64(ts)

	points := make([]MetricPoint, 0, len(ts))
	hourUtil := map[int][]float64{} // 시간대별 사용률(피크 시간 계산)
	var sumUtil, sumTemp, sumPower float64
	var peakUtil, maxTemp, peakPower, idleN int
	for _, t := range ts {
		p := MetricPoint{
			T: t, Util: round(util[t]), Temp: round(temp[t]), Power: round(power[t]),
			VramPct: round(vram[t]), Cpu: round(cpu[t]), Mem: round(mem[t]),
		}
		points = append(points, p)
		sumUtil += util[t]
		sumTemp += temp[t]
		sumPower += power[t]
		if p.Util > peakUtil {
			peakUtil = p.Util
		}
		if p.Temp > maxTemp {
			maxTemp = p.Temp
		}
		if p.Power > peakPower {
			peakPower = p.Power
		}
		if util[t] < 5 {
			idleN++
		}
		h := time.Unix(t, 0).Hour()
		hourUtil[h] = append(hourUtil[h], util[t])
	}

	n := len(points)
	ins.Samples = n
	if n > 0 {
		ins.AvgUtil = round(sumUtil / float64(n))
		ins.PeakUtil = peakUtil
		ins.AvgTemp = round(sumTemp / float64(n))
		ins.MaxTemp = maxTemp
		ins.AvgPower = round(sumPower / float64(n))
		ins.PeakPower = peakPower
		ins.IdlePct = round(float64(idleN) / float64(n) * 100)
		ins.EnergyKwh = math.Round(sumPower/float64(n)*float64(hours)/1000*100) / 100
		ins.PeakHour = peakHourOf(hourUtil)
	}
	return points, ins
}

// rangeSeries는 range 쿼리 결과를 unix초 기준 맵으로 반환한다(미가용이면 빈 맵).
func (s *Service) rangeSeries(ctx context.Context, q string, start, end time.Time, step time.Duration) map[int64]float64 {
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

// peakHourOf는 평균 사용률이 가장 높은 시간대(0~23)를 반환한다. 데이터 없으면 -1.
func peakHourOf(m map[int][]float64) int {
	best, bestAvg := -1, -1.0
	for h, vals := range m {
		if len(vals) == 0 {
			continue
		}
		var sum float64
		for _, v := range vals {
			sum += v
		}
		avg := sum / float64(len(vals))
		if avg > bestAvg {
			bestAvg, best = avg, h
		}
	}
	return best
}

func sortInt64(a []int64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
