package k8s

import (
	"context"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// SetNodeLabel은 노드 라벨을 설정/제거한다(value="" → 제거). merge patch 사용.
func (c *Client) SetNodeLabel(ctx context.Context, node, key, value string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	v := `"` + value + `"`
	if value == "" {
		v = "null" // merge patch: null = 라벨 제거
	}
	patch := []byte(`{"metadata":{"labels":{"` + key + `":` + v + `}}}`)
	_, err := c.cs.CoreV1().Nodes().Patch(ctx, node, types.MergePatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// LiveNode는 K8s 에서 본 노드 라이브 상태.
type LiveNode struct {
	Name        string `json:"name"`
	GpuType     string `json:"gpuType"`
	Physical    bool   `json:"physical"`
	Cordoned    bool   `json:"cordoned"`
	Ready       bool   `json:"ready"`
	GpuCapacity string `json:"gpuCapacity"`
	CPUCores    int    `json:"cpuCores"`    // 노드 CPU capacity(코어)
	MemGB       int    `json:"memGb"`       // 노드 메모리 capacity(GiB)
	CudaVersion string `json:"cudaVersion"` // 노드 CUDA 툴킷(런타임) 버전(GFD 라벨). 라벨 없으면 빈 문자열.
	CudaMin     string `json:"cudaMin"`     // 이 GPU가 물리적으로 지원하는 최소 CUDA 툴킷(컴퓨트 능력 기준).
	CudaMax     string `json:"cudaMax"`     // 설치 드라이버가 지원하는 최대 CUDA 툴킷(드라이버 버전 기준).
	GpuMemMB    int    `json:"gpuMemMb"`    // 물리 GPU 1장의 VRAM(MB) — GFD 라벨 nvidia.com/gpu.memory. HAMi 분할 가용 계산용.
}

// ListNodes는 전체 노드의 라이브 상태를 반환한다.
func (c *Client) ListNodes(ctx context.Context) ([]LiveNode, error) {
	if !c.Available() {
		return nil, ErrNoCluster
	}
	nodes, err := c.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]LiveNode, 0, len(nodes.Items))
	for i := range nodes.Items {
		if isControlPlane(&nodes.Items[i]) {
			continue // 컨트롤플레인은 워크로드 노드가 아니므로 제외
		}
		out = append(out, toLiveNode(&nodes.Items[i], c.gpuTypeLabel, c.physLabel(), c.cudaLabel))
	}
	return out, nil
}

// isControlPlane은 컨트롤플레인/마스터 노드 여부.
func isControlPlane(n *corev1.Node) bool {
	_, cp := n.Labels["node-role.kubernetes.io/control-plane"]
	_, master := n.Labels["node-role.kubernetes.io/master"]
	return cp || master
}

func toLiveNode(n *corev1.Node, gpuTypeLabel, physicalLabel, cudaLabel string) LiveNode {
	gpu := n.Status.Capacity[corev1.ResourceName(resGPU)]
	cpu := n.Status.Capacity[corev1.ResourceCPU]
	mem := n.Status.Capacity[corev1.ResourceMemory]
	// 물리 GPU 개수는 GFD 라벨 nvidia.com/gpu.count 우선(HAMi 가 안 건드림). HAMi 는 nvidia.com/gpu
	// capacity 를 vGPU 수(물리×deviceSplitCount)로 부풀리므로 capacity 를 물리수로 쓰면 안 된다.
	// 라벨이 없으면(GFD 미설치·stock device-plugin) capacity 로 폴백(그땐 capacity=물리수).
	gpuCap := gpu.String()
	if c := atoiLabel(n.Labels["nvidia.com/gpu.count"]); c > 0 {
		gpuCap = strconv.Itoa(c)
	}
	return LiveNode{
		Name:        n.Name,
		GpuType:     n.Labels[gpuTypeLabel],
		Physical:    n.Labels[physicalLabel] == "true",
		Cordoned:    n.Spec.Unschedulable,
		Ready:       isNodeReady(n),
		GpuCapacity: gpuCap,
		CPUCores:    int(cpu.Value()),
		MemGB:       int(mem.Value() / (1024 * 1024 * 1024)),
		CudaVersion: cudaVersionOf(n.Labels, cudaLabel),
		CudaMin:     computeMinCuda(n.Labels["nvidia.com/gpu.compute.major"], n.Labels["nvidia.com/gpu.compute.minor"]),
		CudaMax:     driverMaxCuda(n.Labels["nvidia.com/cuda.driver-version"]),
		GpuMemMB:    atoiLabel(n.Labels["nvidia.com/gpu.memory"]),
	}
}

// atoiLabel은 정수 라벨을 파싱한다(없거나 비정수면 0).
func atoiLabel(v string) int { n, _ := strconv.Atoi(strings.TrimSpace(v)); return n }

// computeMinCuda는 GPU 컴퓨트 능력(major.minor)으로 물리적으로 지원 가능한 최소 CUDA 툴킷을 돌려준다.
// 아키텍처가 처음 지원된 CUDA 버전 기준(예: Turing 7.5→10.0, Ampere 8.0→11.0, Ada/Hopper→11.8, Blackwell→12.8).
func computeMinCuda(major, minor string) string {
	mj, _ := strconv.Atoi(major)
	if mj == 0 {
		return ""
	}
	mn, _ := strconv.Atoi(minor)
	k := mj*10 + mn // 7.5→75, 8.9→89, 12.0→120
	switch {
	case k >= 100:
		return "12.8" // Blackwell
	case k >= 89:
		return "11.8" // Ada / Hopper
	case k >= 86:
		return "11.1" // Ampere(소비자)
	case k >= 80:
		return "11.0" // Ampere A100
	case k >= 75:
		return "10.0" // Turing (T4)
	case k >= 70:
		return "9.0" // Volta
	default:
		return "8.0" // Pascal 이하
	}
}

// driverMaxCuda는 설치된 NVIDIA 드라이버 버전으로 지원 최대 CUDA 툴킷을 돌려준다(드라이버↔CUDA 호환표).
func driverMaxCuda(driver string) string {
	if driver == "" {
		return ""
	}
	maj := driver
	if i := strings.IndexByte(driver, '.'); i > 0 {
		maj = driver[:i]
	}
	d, _ := strconv.Atoi(maj)
	switch {
	case d >= 575:
		return "12.9"
	case d >= 570:
		return "12.8"
	case d >= 560:
		return "12.6"
	case d >= 550:
		return "12.4"
	case d >= 545:
		return "12.3"
	case d >= 535:
		return "12.2"
	case d >= 530:
		return "12.1"
	case d >= 525:
		return "12.0"
	case d >= 515:
		return "11.7"
	case d >= 470:
		return "11.4"
	default:
		return ""
	}
}

// cudaVersionOf는 노드 CUDA 툴킷(런타임) 버전을 라벨에서 뽑는다(예: 12.2, 11.8).
// GFD 조각 라벨 nvidia.com/cuda.runtime.major/.minor 를 우선 "major.minor" 로 합성한다.
// 없으면 설정된 단일 라벨(운영에서 combined 런타임 라벨을 쓰는 경우)을 본다.
// ⚠️ 드라이버 버전(nvidia.com/cuda.driver-version=535 등)은 CUDA 버전이 아니므로 쓰지 않는다.
func cudaVersionOf(labels map[string]string, primary string) string {
	if maj := labels["nvidia.com/cuda.runtime.major"]; maj != "" {
		if min := labels["nvidia.com/cuda.runtime.minor"]; min != "" {
			return maj + "." + min
		}
		return maj
	}
	if primary != "" {
		if v := labels[primary]; v != "" {
			return v
		}
	}
	return ""
}

func isNodeReady(n *corev1.Node) bool {
	for _, cond := range n.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// SetCordon은 노드 스케줄링을 막거나(true) 푼다(false).
func (c *Client) SetCordon(ctx context.Context, name string, on bool) error {
	if !c.Available() {
		return ErrNoCluster
	}
	patch := []byte(`{"spec":{"unschedulable":` + boolStr(on) + `}}`)
	_, err := c.cs.CoreV1().Nodes().Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
