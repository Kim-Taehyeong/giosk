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

// TypeAvail은 GPU 타입별 가용량(클러스터 capacity 에서 running 세션을 뺀 값).
// Fractional* 는 분할(공유·HAMi) 가능한 GPU 만 집계한다. share_mode=hami 인 노드의 GPU 다.
// HAMi 노드가 없으면 FractionalFree/Total 이 0 이라 공유 모드는 "가용 없음"이 된다.
type TypeAvail struct {
	GpuType         string `json:"gpuType"`
	Free            int    `json:"free"`
	Total           int    `json:"total"`
	Nodes           int    `json:"nodes"`
	FractionalFree  int    `json:"fractionalFree"`  // 분할 가능 물리 GPU 여유(대략치, 하위호환)
	FractionalTotal int    `json:"fractionalTotal"` // 분할 가능 물리 GPU 총수
	// HAMi 분할 가용을 "GPU 단위 소수"로(물리 GPU=1, 잔여 코어·VRAM 비율의 작은 쪽). 10슬롯 뻥튀기 대신.
	FractionalFreeUnits float64 `json:"fractionalFreeUnits"`
	// HAMi 분할 자원 잔여. 물리 GPU 개수가 아니라 실제 VRAM(MB), 코어(%), 태스크슬롯 기준이다.
	// 프론트가 "이 오퍼링(예: 4GB) 이 몇 개 더 들어가는지"를 노드별로 계산해 합산한다.
	FracVramFreeMB  int `json:"fracVramFreeMb"`
	FracVramTotalMB int `json:"fracVramTotalMb"`
	FracCoresFree   int `json:"fracCoresFree"` // 여유 코어 합(각 GPU 100% 기준)
	FracCoresTotal  int `json:"fracCoresTotal"`
	FracSlotsFree   int `json:"fracSlotsFree"` // 여유 태스크 슬롯(deviceSplitCount×GPU수 − 실행중 공유세션)
	FracSlotsTotal  int `json:"fracSlotsTotal"`
}

// NodeAvail은 노드별 가용량(대시보드 노드 박스). 분할 노드는 VRAM·코어·슬롯 잔여도 포함한다.
type NodeAvail struct {
	Node     string `json:"node"`
	Gpu      string `json:"gpu"`
	GpuType  string `json:"gpuType"` // 원시 GPU 모델(빠른 생성 배지: 노드↔타입 매핑)
	GpuTotal int    `json:"gpuTotal"`
	GpuFree  int    `json:"gpuFree"`
	Physical bool   `json:"physical"`
	// HAMi 분할 노드만 채워진다. 프론트 오퍼링 가용 계산용(노드 단위 패킹)이다.
	Fractional      bool `json:"fractional"`
	FracVramFreeMB  int  `json:"fracVramFreeMb"`
	FracVramTotalMB int  `json:"fracVramTotalMb"`
	FracCoresFree   int  `json:"fracCoresFree"`
	FracCoresTotal  int  `json:"fracCoresTotal"`
	FracSlotsFree   int  `json:"fracSlotsFree"`
	FracSlotsTotal  int  `json:"fracSlotsTotal"`
	// 이 노드에 로컬 캐시된 데이터셋(세션 생성 시 노드 선호 표시용).
	Cached []CachedDS `json:"cached"`
}

// CachedDS는 노드 로컬 캐시 데이터셋(세션 생성 노드 picker 에 표시).
type CachedDS struct {
	Name      string `json:"name"`
	SizeClass string `json:"sizeClass"`
	SizeGb    int    `json:"sizeGb"`
	SizeBytes int64  `json:"sizeBytes"` // 정확 크기(sub-GB 도 표시되게). 프론트는 formatBytes 로.
	Hash      string `json:"hash"`
	Owner     string `json:"owner"`
}

// ShareUse는 한 노드에서 실행 중인 공유(HAMi) 세션들의 자원 점유 합.
type ShareUse struct {
	VramMB int
	Cores  int
	Count  int
}

// Reservation은 아직 노드가 정해지지 않은(스케줄 대기) 세션들의 자원 예약분(GPU 타입별).
// 가용성 표시(Availability)에는 반영하지 않는다: 화면은 "실제로 놓인 것"을 보여주고,
// 관문(CanPlace)만 이 예약분을 빼고 판정한다. 표시는 정확하게, 승인은 보수적으로 하기 위해서다.
type Reservation struct {
	GpuByType    map[string]int      // 전용 예약 GPU 수
	SharedByType map[string]ShareUse // 분할 예약(VRAM·코어·슬롯)
}

// SessionCounter는 자리를 차지한 세션(=관문을 통과해 admit 된 것)을 센다.
//
// "실행 중(running)"만 세면 안 된다. 방금 만들어져 아직 스케줄 중(provisioning)인 세션도
// 이미 그 GPU 를 예약한 상태다. running 만 세던 시절에는 동시에 들어온 요청들이 모두 같은
// 여유를 보고 통과해(TOCTOU) 뒤에 온 세션이 Pending 으로 매달렸다. 이 제품이 두지 않기로 한
// "보이지 않는 대기열"이 정확히 그렇게 생겼다.
type SessionCounter interface {
	RunningByGpuType() map[string]int
	RunningByNode() map[string]int
	// Reserved는 admit 됐지만 아직 노드가 정해지지 않은 세션(phase=provisioning, node='')의 예약분이다.
	// 노드에 귀속시킬 수 없으므로 후보 노드 어디든 들어올 수 있다고 보고 보수적으로 차감한다.
	Reserved() Reservation
	RunningPhysicalNodes() map[string]bool  // 물리(SSH) 임대 중인 노드 = 노드 통째 점유
	HamiNodes() map[string]bool             // share_mode='hami' 노드(분할 가능) 집합
	NodeSplitCount() map[string]int         // 노드별 deviceSplitCount(nodes.split_count)
	SharedUsageByNode() map[string]ShareUse // 노드별 실행중 공유세션 VRAM/코어/개수 합
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
	var cachedDS map[string][]CachedDS
	if s.cachedByNode != nil {
		cachedDS = s.cachedByNode()
	}
	physNodes := s.counter.RunningPhysicalNodes() // 물리 임대 노드는 통째로 점유된 것으로 본다
	hamiNodes := s.counter.HamiNodes()            // 분할 가능(share_mode='hami') 노드
	splitByNode := s.counter.NodeSplitCount()     // 노드별 deviceSplitCount
	shareUse := s.counter.SharedUsageByNode()     // 노드별 실행중 공유세션 자원 점유
	byType := map[string]*TypeAvail{}
	usedByType := map[string]int{}
	fracFreeByType := map[string]int{} // 분할 가능 노드의 여유 GPU 합(타입별, 하위호환)
	for _, n := range nodes {
		// capN = 물리 GPU 수. node_admin 이 GFD 라벨 nvidia.com/gpu.count 에서 읽어 HAMi vGPU 뻥튀기를
		// 이미 제거했으므로 여기서 추가 환산은 필요 없다.
		capN := atoiSafe(n.GpuCapacity)
		if capN == 0 {
			continue // GPU 미보유 노드 제외
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
		t.Nodes++
		na := NodeAvail{
			Node: n.Name, Gpu: gpuLabel(n.GpuType, capN), GpuType: n.GpuType,
			GpuTotal: capN, GpuFree: capN - used, Physical: n.Physical,
			Cached: cachedDS[n.Name],
		}
		if na.Cached == nil {
			na.Cached = []CachedDS{}
		}
		if hamiNodes[n.Name] && !physNodes[n.Name] { // 분할(HAMi) 노드 = 분할 전용(전용 가용에서 제외)
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
		} else {
			// 비-HAMi(전용·물리) 노드만 전용(whole-card) 가용에 넣는다. 전용과 HAMi 는 분리 집계다.
			t.Total += capN
			usedByType[n.GpuType] += used
		}
		out.ByNode = append(out.ByNode, na)
	}
	for gt, t := range byType {
		t.Free = t.Total - usedByType[gt]
		if t.Free < 0 {
			t.Free = 0
		}
		t.FractionalFree = clampNonNeg(fracFreeByType[gt])
		// HAMi 분할 가용을 "GPU 단위 소수"로 표기(물리 GPU=1, 잔여는 코어·VRAM 비율 중 작은 쪽).
		// 10슬롯으로 뻥튀기하지 않는다. 예를 들어 2장 중 한 장을 70% 쓰면 1.3 이 가용이다.
		if t.FractionalTotal > 0 {
			vf, cf := 1.0, 1.0
			if t.FracVramTotalMB > 0 {
				vf = float64(t.FracVramFreeMB) / float64(t.FracVramTotalMB)
			}
			if t.FracCoresTotal > 0 {
				cf = float64(t.FracCoresFree) / float64(t.FracCoresTotal)
			}
			frac := vf
			if cf < frac {
				frac = cf
			}
			t.FractionalFreeUnits = frac * float64(t.FractionalTotal)
		}
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

// cmpVersion은 "535.104.05" 나 "12.4" 같은 점 구분 버전을 숫자 튜플로 비교한다(a<b 면 -1).
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

// admitted 는 이미 자리를 차지한 세션이다. 스케줄 중(provisioning)도 포함한다. Pod 가 이미 그 GPU 를
// 요청한 상태라, 여유로 세면 같은 자리를 두 번 내주게 된다.
const admittedPhases = `phase IN ('provisioning','running')`

func (c *sessionCounter) RunningByGpuType() map[string]int {
	return c.group(`SELECT gpu_type AS k, ` + usedGpuExpr + ` AS n FROM sessions WHERE ` + admittedPhases + ` GROUP BY gpu_type`)
}

func (c *sessionCounter) RunningByNode() map[string]int {
	return c.group(`SELECT node AS k, ` + usedGpuExpr + ` AS n FROM sessions WHERE ` + admittedPhases + ` AND node<>'' GROUP BY node`)
}

// Reserved는 아직 노드가 안 붙은(스케줄 대기) 세션의 예약분을 GPU 타입별로 모은다.
func (c *sessionCounter) Reserved() Reservation {
	res := Reservation{GpuByType: map[string]int{}, SharedByType: map[string]ShareUse{}}
	var rows []struct {
		GpuType string
		Mode    string
		Gpus    int
		Vram    int
		Core    int
		Cnt     int
	}
	c.db.Raw(`SELECT gpu_type, gpu_mode AS mode,
			COALESCE(SUM(GREATEST(gpu_count,1)),0) AS gpus,
			COALESCE(SUM(vram_mb),0) AS vram, COALESCE(SUM(core_percent),0) AS core, COUNT(*) AS cnt
		FROM sessions
		WHERE phase='provisioning' AND node='' AND gpu_mode<>'cpu' AND gpu_type<>''
		GROUP BY gpu_type, gpu_mode`).Scan(&rows)
	for _, r := range rows {
		if r.Mode == "shared" {
			res.SharedByType[r.GpuType] = ShareUse{VramMB: r.Vram, Cores: r.Core, Count: r.Cnt}
			continue
		}
		res.GpuByType[r.GpuType] += r.Gpus
	}
	return res
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
// 노드 설정 화면에서 자원 공유(HAMi)를 켠 노드만 분할 스케줄 대상이며, 공유 가용 계산의 분모가 된다.
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
		FROM sessions WHERE ` + admittedPhases + ` AND gpu_mode='shared' AND node<>'' GROUP BY node`).Scan(&rows)
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

// PlaceReq는 세션 하나를 지금 배치할 수 있는지 묻는 요청(가용성 게이트).
type PlaceReq struct {
	GpuMode     string // shared|exclusive|cpu|timeslice
	GpuType     string
	GpuCount    int
	VramMB      int // shared 전용
	CorePercent int // shared 전용
	// Node 가 있으면 그 노드 안에서만 판정한다. 중단 세션의 사양 변경(GPU 붙이기)처럼
	// 홈 PVC 가 이미 특정 노드에 묶인 경우, 클러스터 전체에 자리가 있어도 그 노드에
	// 자리가 없으면 재개가 영구 Pending 이 되므로 노드 단위로 물어야 한다.
	Node string
}

// CanPlace는 "지금 이 세션이 들어갈 자리가 있는가"를 답한다.
//
// 대기열을 두지 않기로 한 제품 결정에 따라, 생성과 재시작 모두 이 관문을 통과해야 한다.
// 판정 소스는 세션 마법사·대시보드가 쓰는 Availability 와 같다. 화면이 "가용 없음"이라고
// 말하는데 API 는 받아주는(또는 그 반대의) 불일치가 생기지 않게 하기 위해서다.
//
// 모드별 기준:
//   - cpu       : GPU 를 쓰지 않으므로 항상 통과(CPU 부족은 스케줄러가 판단).
//   - exclusive : 그 타입의 여유 GPU 수 ≥ 요청 개수.
//   - shared    : 한 노드 안에서 VRAM·코어·슬롯이 모두 남는 노드가 하나라도 있어야 한다.
//     (합계로 보면 여러 노드에 흩어진 잔여가 합쳐져 "있다"고 나오지만 실제로는 못 들어간다.)
//   - timeslice : 그 타입에 여유 슬롯이 하나라도 있어야 한다.
//
// 클러스터 정보를 못 얻으면(빈 결과) 막지 않는다. 조회 실패로 사용자를 잠그는 것보다
// 스케줄러가 판단하게 두는 편이 낫다.
func (s *Service) CanPlace(ctx context.Context, r PlaceReq) bool {
	if r.GpuMode == "cpu" || r.GpuType == "" {
		return true
	}
	av := s.Availability(ctx)
	if len(av.ByType) == 0 && len(av.ByNode) == 0 {
		return true // 가용성 조회에 실패하면 게이트를 적용하지 않는다
	}
	// 스케줄 대기 중인(노드 미정) 세션의 예약분을 먼저 뺀다. 같은 자리를 두 번 내주지 않기 위해서다.
	res := s.counter.Reserved()
	if r.Node != "" {
		return canPlaceOnNode(av, r, res)
	}

	switch r.GpuMode {
	case "exclusive":
		need := r.GpuCount
		if need < 1 {
			need = 1
		}
		for _, t := range av.ByType {
			if t.GpuType == r.GpuType {
				return t.Free-res.GpuByType[r.GpuType] >= need
			}
		}
		return false // 그런 타입이 클러스터에 없음

	case "shared":
		for _, n := range av.ByNode {
			if !n.Fractional || n.GpuType != r.GpuType {
				continue
			}
			if fitsShared(n, r, res.SharedByType[r.GpuType]) {
				return true // 이 노드 하나에 통째로 들어간다
			}
		}
		return false

	case "timeslice":
		for _, t := range av.ByType {
			if t.GpuType == r.GpuType {
				return t.FracSlotsFree >= 1
			}
		}
		return false
	}
	return true
}

// fitsShared는 분할 요청이 이 노드에 들어가는지 본다(예약분 hold 를 뺀 잔여 기준).
// hold 는 노드가 정해지지 않은 분할 세션의 예약분이다. 어느 노드로 갈지 모르므로 후보 노드마다
// 빼고 본다(보수적). 그 결과 잠깐 실제보다 빡빡하게 판정될 수 있지만, 반대 방향의 실수(Pending)
// 보다 낫다: 여기서 거절당한 사용자는 즉시 알고 다시 시도하면 되지만, Pending 은 알 길이 없다.
func fitsShared(n NodeAvail, r PlaceReq, hold ShareUse) bool {
	if n.FracSlotsFree-hold.Count < 1 {
		return false
	}
	if r.VramMB > 0 && n.FracVramFreeMB-hold.VramMB < r.VramMB {
		return false
	}
	if r.CorePercent > 0 && n.FracCoresFree-hold.Cores < r.CorePercent {
		return false
	}
	return true
}

// canPlaceOnNode는 지정 노드 한 대 안에서만 자리를 판정한다(노드 고정 세션용).
// 그 노드가 인벤토리에 없으면(이름 오타, 노드 제거, NotReady) 막지 않는다. 조회 공백으로
// 사용자를 잠그기보다 스케줄러 판단에 맡기는 CanPlace 의 기존 원칙을 따른다.
func canPlaceOnNode(av Availability, r PlaceReq, res Reservation) bool {
	for _, n := range av.ByNode {
		if n.Node != r.Node {
			continue
		}
		if n.GpuType != r.GpuType {
			return false // 이 노드엔 그 GPU 모델이 없으니 그 사양으로는 여기서 못 뜬다
		}
		switch r.GpuMode {
		case "exclusive":
			need := r.GpuCount
			if need < 1 {
				need = 1
			}
			return n.GpuFree-res.GpuByType[r.GpuType] >= need
		case "shared":
			// 분할은 HAMi 노드에서만 뜬다. 같은 모델이어도 분할 노드가 아니면 스케줄되지 않는다.
			if !n.Fractional {
				return false
			}
			return fitsShared(n, r, res.SharedByType[r.GpuType])
		}
		return true
	}
	return true
}
