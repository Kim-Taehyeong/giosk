package resource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// Availability는 GPU 가용 현황(세션 마법사·대시보드 공용 단일 소스).
// 타입별(전용 GPU 풀)과 노드별 두 뷰를 함께 제공한다.
type Availability struct {
	ByType []TypeAvail `json:"byType"`
	ByNode []NodeAvail `json:"byNode"`
}

// TypeAvail — GPU 타입별 가용(클러스터 capacity − running 세션).
// Fractional* 는 분할(공유/HAMi) 가능한 GPU 만 집계 — share_mode='hami' 인 노드의 GPU.
// HAMi 노드가 없으면 FractionalFree/Total=0 → 공유 모드는 "가용 없음".
type TypeAvail struct {
	GpuType         string `json:"gpuType"`
	Free            int    `json:"free"`
	Total           int    `json:"total"`
	Nodes           int    `json:"nodes"`
	FractionalFree  int    `json:"fractionalFree"`  // 분할 가능 물리 GPU 여유(대략치, 하위호환)
	FractionalTotal int    `json:"fractionalTotal"` // 분할 가능 물리 GPU 총수
	// HAMi 분할 자원 잔여 — 물리 GPU 개수가 아니라 실제 VRAM(MB)/코어(%)/태스크슬롯 기준.
	// 프론트가 "이 오퍼링(예: 4GB) 이 몇 개 더 들어가는지"를 노드별로 계산해 합산한다.
	FracVramFreeMB  int `json:"fracVramFreeMb"`
	FracVramTotalMB int `json:"fracVramTotalMb"`
	FracCoresFree   int `json:"fracCoresFree"`  // 여유 코어 합(각 GPU 100% 기준)
	FracCoresTotal  int `json:"fracCoresTotal"`
	FracSlotsFree   int `json:"fracSlotsFree"` // 여유 태스크 슬롯(deviceSplitCount×GPU수 − 실행중 공유세션)
	FracSlotsTotal  int `json:"fracSlotsTotal"`
}

// NodeAvail — 노드별 가용(대시보드 노드 박스). 분할 노드는 VRAM/코어/슬롯 잔여도 포함.
type NodeAvail struct {
	Node     string `json:"node"`
	Gpu      string `json:"gpu"`
	GpuType  string `json:"gpuType"` // 원시 GPU 모델(빠른 생성 배지: 노드↔타입 매핑)
	GpuTotal int    `json:"gpuTotal"`
	GpuFree  int    `json:"gpuFree"`
	Physical bool   `json:"physical"`
	// HAMi 분할 노드(share_mode='hami')만 채워짐 — 프론트 오퍼링 가용 계산용(노드 단위 패킹).
	Fractional      bool `json:"fractional"`
	FracVramFreeMB  int  `json:"fracVramFreeMb"`
	FracVramTotalMB int  `json:"fracVramTotalMb"`
	FracCoresFree   int  `json:"fracCoresFree"`
	FracCoresTotal  int  `json:"fracCoresTotal"`
	FracSlotsFree   int  `json:"fracSlotsFree"`
	FracSlotsTotal  int  `json:"fracSlotsTotal"`
}

// ShareUse — 한 노드에서 실행 중인 공유(HAMi) 세션들의 자원 점유 합.
type ShareUse struct {
	VramMB int
	Cores  int
	Count  int
}

// SessionCounter는 running 세션 수(타입/노드별)를 센다.
type SessionCounter interface {
	RunningByGpuType() map[string]int
	RunningByNode() map[string]int
	RunningPhysicalNodes() map[string]bool     // 물리(SSH) 임대 중인 노드 = 노드 통째 점유
	HamiNodes() map[string]bool                // share_mode='hami' 노드(분할 가능) 집합
	NodeSplitCount() map[string]int            // 노드별 deviceSplitCount(nodes.split_count)
	SharedUsageByNode() map[string]ShareUse    // 노드별 실행중 공유세션 VRAM/코어/개수 합
}

// Availability는 라이브 노드 capacity 에서 running 세션을 빼 가용을 계산한다.
// 클러스터 미가용이면 빈 결과(0). GPU 없는 노드(control-plane)는 제외.
func (s *Service) Availability(ctx context.Context) Availability {
	out := Availability{ByType: []TypeAvail{}, ByNode: []NodeAvail{}}
	nodes, err := s.gpu.ListNodes(ctx)
	if err != nil {
		return out
	}
	usedNode := s.counter.RunningByNode()
	physNodes := s.counter.RunningPhysicalNodes() // 물리 임대 노드 → 통째 점유
	hamiNodes := s.counter.HamiNodes()            // 분할 가능(share_mode='hami') 노드
	splitByNode := s.counter.NodeSplitCount()     // 노드별 deviceSplitCount
	shareUse := s.counter.SharedUsageByNode()     // 노드별 실행중 공유세션 자원 점유
	byType := map[string]*TypeAvail{}
	usedByType := map[string]int{}
	fracFreeByType := map[string]int{} // 분할 가능 노드의 여유 GPU 합(타입별, 하위호환)
	for _, n := range nodes {
		capN := atoiSafe(n.GpuCapacity)
		if capN == 0 {
			continue // GPU 미보유 노드 제외
		}
		// HAMi 노드는 nvidia.com/gpu 를 "vGPU 수(= 물리 GPU 수 × deviceSplitCount)"로 광고한다.
		// 아래 계산은 capN 을 "물리 GPU 수"로 가정하므로(전용 Total, 분할 VRAM=perGPU×capN,
		// 코어=100×capN, 슬롯=split×capN), split 로 나눠 물리 수로 환산한다. 안 하면 전용 가용성이
		// split 배(예 2노드×10=20)로, 분할 총량이 10배로 부풀려진다.
		if hamiNodes[n.Name] {
			if split := splitByNode[n.Name]; split > 1 && capN/split >= 1 {
				capN /= split
			}
		}
		used := usedNode[n.Name]
		if physNodes[n.Name] {
			used = capN // 물리(SSH) 임대 = 노드 전체 GPU 점유(1개가 아니라 통째)
		}
		if used > capN {
			used = capN
		}
		t := byType[n.GpuType]
		if t == nil {
			t = &TypeAvail{GpuType: n.GpuType}
			byType[n.GpuType] = t
		}
		t.Total += capN
		t.Nodes++
		usedByType[n.GpuType] += used
		na := NodeAvail{
			Node: n.Name, Gpu: gpuLabel(n.GpuType, capN), GpuType: n.GpuType,
			GpuTotal: capN, GpuFree: capN - used, Physical: n.Physical,
		}
		if hamiNodes[n.Name] && !physNodes[n.Name] { // 분할(HAMi) 노드만 VRAM/코어/슬롯 잔여 계산
			t.FractionalTotal += capN
			fracFreeByType[n.GpuType] += capN - used
			// 노드 단위 분할 자원: VRAM=perGPU×GPU수, 코어=100×GPU수, 슬롯=deviceSplitCount×GPU수.
			split := splitByNode[n.Name]
			if split <= 0 {
				split = 10 // nodes.split_count 기본
			}
			u := shareUse[n.Name]
			na.Fractional = true
			na.FracVramTotalMB = n.GpuMemMB * capN
			na.FracVramFreeMB = clampNonNeg(na.FracVramTotalMB - u.VramMB)
			na.FracCoresTotal = 100 * capN
			na.FracCoresFree = clampNonNeg(na.FracCoresTotal - u.Cores)
			na.FracSlotsTotal = split * capN
			na.FracSlotsFree = clampNonNeg(na.FracSlotsTotal - u.Count)
			t.FracVramTotalMB += na.FracVramTotalMB
			t.FracVramFreeMB += na.FracVramFreeMB
			t.FracCoresTotal += na.FracCoresTotal
			t.FracCoresFree += na.FracCoresFree
			t.FracSlotsTotal += na.FracSlotsTotal
			t.FracSlotsFree += na.FracSlotsFree
		}
		out.ByNode = append(out.ByNode, na)
	}
	for gt, t := range byType {
		t.Free = t.Total - usedByType[gt]
		if t.Free < 0 {
			t.Free = 0
		}
		t.FractionalFree = clampNonNeg(fracFreeByType[gt])
		out.ByType = append(out.ByType, *t)
	}
	sort.Slice(out.ByType, func(i, j int) bool { return out.ByType[i].GpuType < out.ByType[j].GpuType })
	return out
}

func gpuLabel(gpuType string, n int) string {
	if gpuType == "" {
		return fmt.Sprintf("GPU ×%d", n)
	}
	return fmt.Sprintf("%s ×%d", gpuType, n)
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return n
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// cmpVersion은 "535.104.05" / "12.4" 같은 점 구분 버전을 숫자 튜플로 비교한다(a<b→-1).
// 비숫자 세그먼트는 0으로 취급. CUDA/드라이버 버전의 "최소값" 선택용.
func cmpVersion(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			ai = atoiSafe(as[i])
		}
		if i < len(bs) {
			bi = atoiSafe(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

// sessionCounter는 sessions 테이블 기반 SessionCounter 구현.
type sessionCounter struct{ db *gorm.DB }

// NewSessionCounter는 가용 계산용 running 세션 카운터를 만든다.
func NewSessionCounter(db *gorm.DB) SessionCounter { return &sessionCounter{db: db} }

// 점유 GPU 수 = 전용은 gpu_count(전용 2개면 2), CPU 세션은 0, 분할(shared)은 GPU 1개 점유로 근사.
// (분할 세션의 실제 동거 배치는 추적하지 않으므로 보수적으로 1개로 본다)
const usedGpuExpr = `SUM(CASE WHEN gpu_mode='exclusive' THEN GREATEST(gpu_count,1) WHEN gpu_mode='cpu' THEN 0 ELSE 1 END)`

func (c *sessionCounter) RunningByGpuType() map[string]int {
	return c.group(`SELECT gpu_type AS k, ` + usedGpuExpr + ` AS n FROM sessions WHERE phase='running' GROUP BY gpu_type`)
}

func (c *sessionCounter) RunningByNode() map[string]int {
	return c.group(`SELECT node AS k, ` + usedGpuExpr + ` AS n FROM sessions WHERE phase='running' AND node<>'' GROUP BY node`)
}

// RunningPhysicalNodes는 물리(SSH) 세션이 활성인 노드 집합을 반환한다(노드 통째 점유 판정).
func (c *sessionCounter) RunningPhysicalNodes() map[string]bool {
	var nodes []string
	c.db.Raw(`SELECT DISTINCT node FROM sessions WHERE env='ssh' AND node<>'' AND phase IN ('provisioning','running')`).Scan(&nodes)
	out := map[string]bool{}
	for _, n := range nodes {
		out[n] = true
	}
	return out
}

// HamiNodes는 분할(공유) 가능한 노드 집합(nodes.share_mode='hami')을 반환한다.
// 노드 설정 화면에서 '자원 공유(HAMi)'를 켠 노드만 분할 스케줄 대상 → 공유 가용 계산의 분모.
func (c *sessionCounter) HamiNodes() map[string]bool {
	var nodes []string
	c.db.Raw(`SELECT name FROM nodes WHERE share_mode='hami'`).Scan(&nodes)
	out := map[string]bool{}
	for _, n := range nodes {
		out[n] = true
	}
	return out
}

// NodeSplitCount는 노드별 deviceSplitCount(한 물리 GPU가 수용하는 최대 공유 태스크 수)를 반환한다.
func (c *sessionCounter) NodeSplitCount() map[string]int {
	var rows []struct {
		Name       string
		SplitCount int
	}
	c.db.Raw(`SELECT name, split_count FROM nodes WHERE share_mode='hami'`).Scan(&rows)
	out := map[string]int{}
	for _, r := range rows {
		out[r.Name] = r.SplitCount
	}
	return out
}

// SharedUsageByNode는 노드별 실행중 공유(HAMi) 세션의 VRAM(MB)·코어(%)·개수 합을 반환한다.
// HAMi 잔여 가용 = 노드 총량 − 이 점유. 전용/CPU/타임슬라이스는 제외(공유 분할 자원과 무관).
func (c *sessionCounter) SharedUsageByNode() map[string]ShareUse {
	var rows []struct {
		Node string
		Vram int
		Core int
		Cnt  int
	}
	c.db.Raw(`SELECT node, COALESCE(SUM(vram_mb),0) AS vram, COALESCE(SUM(core_percent),0) AS core, COUNT(*) AS cnt
		FROM sessions WHERE phase='running' AND gpu_mode='shared' AND node<>'' GROUP BY node`).Scan(&rows)
	out := map[string]ShareUse{}
	for _, r := range rows {
		out[r.Node] = ShareUse{VramMB: r.Vram, Cores: r.Core, Count: r.Cnt}
	}
	return out
}

func clampNonNeg(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func (c *sessionCounter) group(q string) map[string]int {
	var rows []struct {
		K string
		N int
	}
	c.db.Raw(q).Scan(&rows)
	out := map[string]int{}
	for _, r := range rows {
		out[r.K] = r.N
	}
	return out
}
