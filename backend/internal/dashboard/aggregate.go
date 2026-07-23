package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"giosk/internal/metrics"
)

// userSessions는 최근 세션을 대시보드 카드 shape 으로 변환한다.
// Spec 은 GPU/CPU 를 명확히 구분(GPU 점유율은 "GPU N%"로 라벨), Credit 은 시간당 단가,
// Conn 은 이미지 channels 기반(running 일 때만).
func (s *Service) userSessions(userID int64) []SessionCard {
	rows := s.repo.UserSessions(userID, 5)
	out := make([]SessionCard, 0, len(rows))
	for _, r := range rows {
		card := SessionCard{ID: r.InstanceID, Name: r.Name, Status: r.Phase, Conn: []string{}, Credit: r.PricePerHour}
		switch {
		case r.Env == "ssh":
			card.Kind, card.Spec = "물리 노드", r.Node
		case r.GpuMode == "cpu":
			card.Kind, card.Spec = "CPU", fmt.Sprintf("%d vCPU·%dGB", r.CPUCores, r.MemGB)
		default:
			card.Kind, card.Spec = "GPU", gpuSpec(r)
		}
		if r.Phase == "running" {
			card.Conn = connOf(r)
		}
		out = append(out, card)
	}
	return out
}

// gpuSpec은 GPU 세션 스펙 문자열(전용=타입×개수, 분할=타입·VRAM·GPU점유율).
// "GPU N%"로 라벨해 CPU 사용률과 혼동되지 않게 한다.
func gpuSpec(r SessionRow) string {
	if r.GpuMode == "exclusive" {
		n := r.GpuCount
		if n < 1 {
			n = 1
		}
		if r.GpuType != "" {
			return fmt.Sprintf("%s ×%d", r.GpuType, n)
		}
		return fmt.Sprintf("GPU ×%d", n)
	}
	base := fmt.Sprintf("%dGB · GPU %d%%", r.VramMB/1024, r.CorePercent)
	if r.GpuType != "" {
		return r.GpuType + " · " + base
	}
	return base
}

// connOf는 이미지 channels JSON → 접속 채널 표시 라벨(물리=SSH, 미설정=VSCode 기본).
func connOf(r SessionRow) []string {
	if r.Env == "ssh" {
		return []string{"SSH"}
	}
	var c struct {
		VSCode  bool `json:"vscode"`
		Jupyter bool `json:"jupyter"`
		SSH     bool `json:"ssh"`
	}
	if r.Channels != "" {
		_ = json.Unmarshal([]byte(r.Channels), &c)
	}
	out := []string{}
	if c.VSCode {
		out = append(out, "VSCode")
	}
	if c.Jupyter {
		out = append(out, "Jupyter")
	}
	if len(out) == 0 {
		out = append(out, "VSCode") // channels 미설정 이미지 = code-server 기본
	}
	// 웹터미널 — 모든 컨테이너 세션에 노출(API exec, 사이드카/게이트웨이 불요).
	// "terminal" 키를 그대로 내보내면 프론트 CONN_CH 가 SSH 버튼으로 표시하고 모달 터미널 탭을 연다.
	out = append(out, "terminal")
	return out
}

// userVRAM은 (사용 GB, 총 GB) 를 반환한다.
//   - 총량: 사용자의 running 세션 오퍼링 VRAM 합(DB).
//   - 사용량: DCGM(DCGM_FI_DEV_FB_USED, MiB)을 사용자의 running GPU 세션 pod 들에 대해 합산.
//     Prometheus 미연동/메트릭 없음/GPU 세션 없음이면 0(보수적 폴백).
func (s *Service) userVRAM(ctx context.Context, userID int64) (int, int) {
	totalMB := s.repo.UserVramTotalMB(userID)
	usedGB := 0
	if s.met.Enabled() {
		if pods := s.repo.UserRunningGpuPods(userID); len(pods) > 0 {
			// PromQL 라벨 정규식은 완전일치(앵커됨) → pod 가 목록 중 하나와 정확히 일치.
			// 워크로드 Pod 라벨은 배포에 따라 pod/exported_pod 로 갈린다(metrics.DCGMPodSeries 참조).
			// pod 만 보면 항상 빈 결과라 사용자 VRAM 이 늘 0 이었다.
			q := fmt.Sprintf(`sum(%s)`, metrics.DCGMPodSeries("DCGM_FI_DEV_FB_USED", strings.Join(pods, "|")))
			if v, ok := s.met.Scalar(ctx, q); ok {
				usedGB = int(v/1024 + 0.5) // MiB → GB
			}
		}
	}
	return usedGB, totalMB / 1024
}

// regions는 오퍼링별 가용 GPU(총/여유)를 노드 capacity − running 세션으로 산출.
func (s *Service) regions(ctx context.Context) []Region {
	capByType := s.gpuCapacityByType(ctx)
	usedByType := s.repo.RunningByGpuType()
	var out []Region
	for _, o := range s.repo.OfferingCapacity() {
		total := capByType[o.GpuType]
		free := total - usedByType[o.GpuType]
		if free < 0 {
			free = 0
		}
		out = append(out, Region{Name: o.Name, Free: free, Total: total, Queue: 0})
	}
	return out
}

// gpuCapacityByType는 클러스터 노드의 GPU 타입별 총 capacity 합.
func (s *Service) gpuCapacityByType(ctx context.Context) map[string]int {
	cap := map[string]int{}
	nodes, err := s.nodes.ListNodes(ctx)
	if err != nil {
		return cap
	}
	for _, n := range nodes {
		c, _ := strconv.Atoi(n.GpuCapacity)
		cap[n.GpuType] += c
	}
	return cap
}

// nodeCounts는 (Ready 노드 수, 전체 노드 수).
func (s *Service) nodeCounts(ctx context.Context) (int, int) {
	nodes, err := s.nodes.ListNodes(ctx)
	if err != nil {
		return 0, 0
	}
	up := 0
	for _, n := range nodes {
		if n.Ready && !n.Cordoned {
			up++
		}
	}
	return up, len(nodes)
}
