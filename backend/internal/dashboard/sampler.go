package dashboard

import (
	"context"
	"log"
	"time"
)

// IdleCounter는 running 세션 중 유휴 개수를 센다(session.Service 가 구현). 미가용이면 0.
type IdleCounter func(ctx context.Context) int

// RunSampler는 주기적으로 인프라 메트릭(Prometheus/k8s) + 세션 수를 metric_snapshots 에 적재한다.
// Prometheus/k8s 는 라이브만 주므로, 이 스냅샷이 대시보드 감시용 시계열의 유일한 영속 소스다.
// interval<=0 이면 1분. idle 은 nil 허용(유휴 0 으로 기록).
func (s *Service) RunSampler(ctx context.Context, interval time.Duration, idle IdleCounter) {
	if interval <= 0 {
		interval = time.Minute
	}
	log.Printf("dashboard: metric snapshot sampler every %s", interval)
	s.sampleOnce(ctx, idle) // 부팅 직후 1회
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sampleOnce(ctx, idle)
		}
	}
}

func (s *Service) sampleOnce(ctx context.Context, idle IdleCounter) {
	up, total := s.nodeCounts(ctx)
	gpusTotal := 0
	for _, c := range s.gpuCapacityByType(ctx) {
		gpusTotal += c
	}
	snap := Snapshot{
		TS:              time.Now(), // GORM 이 zero-value 를 넣어 DB 기본값을 덮어쓰지 않도록 명시.
		GpuUtil:         s.scalar(ctx, "avg(DCGM_FI_DEV_GPU_UTIL)"),
		VramUsedPct:     s.vramAllocPct(ctx),
		GpuTempAvg:      s.scalar(ctx, "avg(DCGM_FI_DEV_GPU_TEMP)"),
		GpuTempMax:      s.scalar(ctx, "max(DCGM_FI_DEV_GPU_TEMP)"),
		NodesUp:         up,
		NodesTotal:      total,
		GpusUsed:        s.repo.RunningGpuCount(),
		GpusTotal:       gpusTotal,
		RunningSessions: s.repo.ActiveSessions(),
	}
	if idle != nil {
		snap.IdleSessions = idle(ctx)
	}
	if err := s.repo.InsertSnapshot(snap); err != nil {
		log.Printf("dashboard: snapshot insert failed: %v", err)
	}
}
