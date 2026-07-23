package k8s

import (
	"context"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GpuType은 클러스터에서 수집한 GPU 모델 1종.
type GpuType struct {
	Name  string `json:"name"`
	Nodes int    `json:"nodes"` // 해당 타입 노드 수
}

// GpuTypes는 노드 라벨(gpuTypeLabel)에서 GPU 타입을 수집한다.
// 운영=GFD 라벨, 개발=fake 라벨. 클러스터 미가용이면 빈 목록.
func (c *Client) GpuTypes(ctx context.Context) ([]GpuType, error) {
	if c == nil {
		return nil, nil
	}
	nodes, err := c.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: c.gpuTypeLabel})
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, n := range nodes.Items {
		if t := n.Labels[c.gpuTypeLabel]; t != "" {
			counts[t]++
		}
	}
	return sortedGpuTypes(counts), nil
}

func sortedGpuTypes(counts map[string]int) []GpuType {
	out := make([]GpuType, 0, len(counts))
	for name, n := range counts {
		out = append(out, GpuType{Name: name, Nodes: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Available은 클러스터 접속 가능 여부.
func (c *Client) Available() bool { return c != nil && c.cs != nil }
