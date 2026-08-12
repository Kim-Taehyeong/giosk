package csi

import (
	"context"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// GC는 PV 가 사라진 이미지를 노드에서 회수한다.
//
// 이미지는 노드 로컬이라 컨트롤러의 DeleteVolume 이 지울 수 없다. 컨트롤러는 성공만
// 돌려주고 실제 회수는 각 노드가 자기 디스크를 보며 한다. 이게 없으면 세션을 지워도
// 이미지와 루프 디바이스가 남아 디스크가 계속 줄어든다.
type GC struct {
	Store    *ImageStore
	Interval time.Duration

	cs kubernetes.Interface
	// 직전 스캔에서 고아로 본 볼륨. 연속 두 번 고아일 때만 지운다.
	// API 목록이 일시적으로 불완전하거나 볼륨 생성 직후 PV 가 아직 안 보이는 순간에
	// 남의 데이터를 지우지 않기 위한 안전장치다.
	suspects map[string]bool
}

// NewGC는 in-cluster 자격으로 API 에 붙는 회수기를 만든다.
func NewGC(store *ImageStore, interval time.Duration) (*GC, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &GC{Store: store, Interval: interval, cs: cs, suspects: map[string]bool{}}, nil
}

// Run은 주기적으로 고아 이미지를 회수한다(컨텍스트가 끝날 때까지).
func (g *GC) Run(ctx context.Context) {
	t := time.NewTicker(g.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := g.sweep(ctx); err != nil {
				// 실패는 다음 주기에 다시 시도한다. 실패한 채로 지우는 일은 없다.
				log.Printf("[csi] 고아 이미지 회수 실패: %v", err)
			}
		}
	}
}

// sweep은 한 바퀴 돈다. PV 목록을 못 얻으면 아무것도 지우지 않는다.
func (g *GC) sweep(ctx context.Context) error {
	pvs, err := g.cs.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		// 목록을 모르는 상태에서 지우면 살아있는 볼륨을 날린다. 의심 목록도 비워
		// 다음에 처음부터 두 번 확인하게 한다.
		g.suspects = map[string]bool{}
		return err
	}
	live := map[string]bool{}
	for i := range pvs.Items {
		if src := pvs.Items[i].Spec.CSI; src != nil && src.Driver == DriverName {
			live[src.VolumeHandle] = true
		}
	}

	ids, err := g.Store.ListVolumeIDs()
	if err != nil {
		return err
	}
	next := map[string]bool{}
	for _, id := range ids {
		if live[id] {
			continue
		}
		if !g.suspects[id] {
			// 처음 보는 고아는 한 번 더 확인하고 지운다.
			next[id] = true
			continue
		}
		if err := g.Store.Delete(ctx, id); err != nil {
			log.Printf("[csi] 이미지 %s 회수 실패: %v", id, err)
			next[id] = true // 다음 주기에 재시도
			continue
		}
		log.Printf("[csi] 고아 이미지 회수: %s (PV 없음)", id)
	}
	g.suspects = next
	return nil
}
