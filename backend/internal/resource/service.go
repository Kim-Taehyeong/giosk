package resource

import (
	"context"
	"errors"
	"fmt"

	"giosk/internal/k8s"
)

// ErrOfferingDup은 같은 규격(GPU 타입·모드·VRAM·코어%)의 오퍼링이 이미 있을 때.
var ErrOfferingDup = errors.New("duplicate offering")

// GpuLister는 클러스터에서 GPU 타입/노드를 수집한다(k8s.Client 가 구현).
type GpuLister interface {
	GpuTypes(ctx context.Context) ([]k8s.GpuType, error)
	ListNodes(ctx context.Context) ([]k8s.LiveNode, error)
}

// Service는 자원 정의 비즈니스 로직.
type Service struct {
	repo        Repository
	gpu         GpuLister
	counter     SessionCounter
	cachedByNode func() map[string][]CachedDS // 노드별 캐시 데이터셋(세션 생성 노드 picker). nil=빈 목록.
}

func NewService(repo Repository, gpu GpuLister, counter SessionCounter) *Service {
	return &Service{repo: repo, gpu: gpu, counter: counter}
}

// WithCachedDatasets는 노드별 캐시 데이터셋 제공자를 주입한다(availability.byNode.cached 채움).
func (s *Service) WithCachedDatasets(fn func() map[string][]CachedDS) *Service {
	s.cachedByNode = fn
	return s
}

func (s *Service) ListOfferings(activeOnly bool) ([]Offering, error) {
	return s.repo.ListOfferings(activeOnly)
}
// SaveOffering은 저장 전에 같은 규격이 이미 있는지 본다.
//
// 오퍼링은 "GPU 를 이만큼 떼어 준다"는 규격이라, (GPU 타입, 모드, VRAM, 코어%)가 같으면
// 이름만 다른 같은 물건이다. 단가도 GPU 단가에서 자동 산출되므로 값까지 같아진다.
// 목록에 똑같은 줄이 둘 뜨면 사용자는 무엇이 다른지 찾다가 아무거나 고른다.
func (s *Service) SaveOffering(o *Offering) error {
	all, err := s.repo.ListOfferings(false)
	if err != nil {
		return err
	}
	for _, e := range all {
		if e.ID == o.ID {
			continue // 자기 자신(수정)
		}
		if e.GpuType == o.GpuType && e.Mode == o.Mode && e.VramMB == o.VramMB && e.CorePercent == o.CorePercent {
			return fmt.Errorf("%w: %s", ErrOfferingDup, e.Name)
		}
	}
	return s.repo.SaveOffering(o)
}
func (s *Service) DeleteOffering(id int64) error  { return s.repo.DeleteOffering(id) }

func (s *Service) ListPresets(activeOnly bool) ([]Preset, error) {
	return s.repo.ListPresets(activeOnly)
}
func (s *Service) SavePreset(p *Preset) error  { return s.repo.SavePreset(p) }
func (s *Service) DeletePreset(id int64) error { return s.repo.DeletePreset(id) }

// GpuTypeInfo는 GPU 타입, 노드 수, 단가(전용 시간당, 분할 GB·시간, 분할 코어%·시간)를 담는다.
// Timeslicing/Slots: 이 타입에 타임슬라이싱 노드가 있으면 true + 슬롯 수(사용자 화면의 슬롯 옵션 노출용).
//
// NodeCPU/NodeMemGB/NodeGPUs: 이 타입 후보 노드 중 "가장 작은" 노드의 사양.
// CPU·메모리 최소 보장 = NodeCPU × (내 GPU 지분). 최소 노드 기준이라 어느 후보에 떨어져도 보장이 성립한다.
// (requests 만 걸고 limits 는 안 걸어 여유가 있으면 초과 사용할 수 있어 큰 노드가 놀지 않는다.)
type GpuTypeInfo struct {
	Name         string `json:"name"`
	Nodes        int    `json:"nodes"`
	PricePerHour int    `json:"pricePerHour"`
	PricePerGB   int    `json:"pricePerGb"`
	PricePerCore int    `json:"pricePerCore"`
	Timeslicing  bool   `json:"timeslicing"`
	Slots        int    `json:"slots"`
	NodeCPU      int    `json:"nodeCpu"`     // 최소 후보 노드의 CPU 코어
	NodeMemGB    int    `json:"nodeMemGb"`   // 최소 후보 노드의 메모리(GB)
	NodeGPUs     int    `json:"nodeGpus"`    // 그 노드의 GPU 장수(지분 분모)
	CudaVersion  string `json:"cudaVersion"` // 이 타입 노드들 중 최소 CUDA 툴킷(런타임) 버전. 없으면 빈 문자열.
	CudaMin      string `json:"cudaMin"`     // 물리 지원 최소 CUDA(컴퓨트 능력). 이 타입 노드 공통.
	CudaMax      string `json:"cudaMax"`     // 지원 최대 CUDA(드라이버). 노드별 드라이버가 다르면 가장 낮은 값(어디서든 보장).
}

// GpuTypes는 노드 라벨에서 수집한 GPU 타입에 단가(전용/분할 단위)를 병합해 반환한다.
func (s *Service) GpuTypes(ctx context.Context) ([]GpuTypeInfo, error) {
	types, err := s.gpu.GpuTypes(ctx)
	if err != nil {
		return nil, err
	}
	prices, _ := s.repo.ListGpuPricing()
	pm := map[string]GpuPrice{}
	for _, p := range prices {
		pm[p.GpuType] = p
	}
	// 타임슬라이싱 노드(DB)와 라이브 노드의 GPU 타입(K8s 라벨)을 조인해 타입별 슬롯 제공 여부를 낸다.
	// 동시에 타입별 "가장 작은 후보 노드"의 사양을 모은다(CPU·메모리 최소 보장 산출용).
	tsNodes := s.repo.TimeslicingNodes()
	tsByType := map[string]int{}
	minByType := map[string]k8s.LiveNode{}
	cudaByType := map[string]string{}    // 타입별 최소 CUDA 런타임(버전 비교). 노드가 이질적일 수 있어 별도 집계.
	cudaMinByType := map[string]string{} // 물리 지원 최소(컴퓨트 능력). 타입 공통이다.
	cudaMaxByType := map[string]string{} // 지원 최대(드라이버). 노드별로 다르면 가장 낮은 값을 쓴다.
	if live, err := s.gpu.ListNodes(ctx); err == nil {
		for _, n := range live {
			if n.GpuType == "" || !n.Ready {
				continue
			}
			if slots, ok := tsNodes[n.Name]; ok {
				tsByType[n.GpuType] = slots
			}
			// 최소 노드 = CPU 코어가 가장 적은 후보(그 기준으로 보장하면 어디든 스케줄 가능).
			if cur, ok := minByType[n.GpuType]; !ok || n.CPUCores < cur.CPUCores {
				minByType[n.GpuType] = n
			}
			if n.CudaVersion != "" {
				if cur, ok := cudaByType[n.GpuType]; !ok || cmpVersion(n.CudaVersion, cur) < 0 {
					cudaByType[n.GpuType] = n.CudaVersion
				}
			}
			if n.CudaMin != "" {
				cudaMinByType[n.GpuType] = n.CudaMin // 타입 공통이라 마지막 값으로 충분
			}
			if n.CudaMax != "" {
				if cur, ok := cudaMaxByType[n.GpuType]; !ok || cmpVersion(n.CudaMax, cur) < 0 {
					cudaMaxByType[n.GpuType] = n.CudaMax // 어디서든 보장되도록 가장 낮은 max
				}
			}
		}
	}
	out := make([]GpuTypeInfo, 0, len(types))
	for _, t := range types {
		p := pm[t.Name]
		slots := tsByType[t.Name]
		mn := minByType[t.Name]
		out = append(out, GpuTypeInfo{
			Name: t.Name, Nodes: t.Nodes,
			PricePerHour: p.PricePerHour, PricePerGB: p.PricePerGB, PricePerCore: p.PricePerCore,
			Timeslicing: slots > 0, Slots: slots,
			NodeCPU: mn.CPUCores, NodeMemGB: mn.MemGB, NodeGPUs: atoiSafe(mn.GpuCapacity),
			CudaVersion: cudaByType[t.Name],
			CudaMin:     cudaMinByType[t.Name], CudaMax: cudaMaxByType[t.Name],
		})
	}
	return out, nil
}

// CpuPrice는 CPU 전용 세션의 시간당 단가(gpu_pricing 의 'cpu' 행). 없으면 0.
// CPU 도 과금되므로 세션 마법사가 이 값을 예상 비용으로 보여준다(더는 '무료' 아님).
func (s *Service) CpuPrice() int {
	prices, _ := s.repo.ListGpuPricing()
	for _, p := range prices {
		if p.GpuType == "cpu" {
			return p.PricePerHour
		}
	}
	return 0
}

// GpuPricing은 전체 GPU 타입 단가 목록(설정된 것). admin 화면용.
func (s *Service) GpuPricing() ([]GpuPrice, error) { return s.repo.ListGpuPricing() }

// SetGpuPricing은 gpuType 단가(전용/GB/코어)를 설정한다.
func (s *Service) SetGpuPricing(p GpuPrice) error {
	return s.repo.SetGpuPricing(p)
}
