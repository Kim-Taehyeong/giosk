// Package k8s는 client-go 래퍼다. in-cluster 를 먼저 시도하고 안 되면 kubeconfig 로 붙으며,
// 클러스터 미가용 시 nil Client 를 반환해(에러 아님) 상위에서 503 처리하게 한다.
package k8s

import (
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client는 K8s 접근 래퍼.
type Client struct {
	cs            *kubernetes.Clientset
	cfg           *rest.Config // 웹터미널 exec(remotecommand SPDY) 등 REST 스트리밍에 필요
	gpuTypeLabel  string
	physicalLabel string // 물리노드 식별 라벨(멀티 인스턴스 격리용, 기본 giosk.io/physical)
	cudaLabel     string // 노드 CUDA(드라이버) 버전 라벨(GFD). 빈값이면 CUDA 표기 생략.
	// nfsFuse는 공유 NFS 볼륨(공유 볼륨·데이터셋)을 우리 CSI 드라이버로 붙일지 여부다.
	// 켜면 컨테이너에 FUSE 마운트만 보여 스토리지 주소가 남지 않는다. 끄면 NFS 를 직접 붙인다.
	nfsFuse bool
	// nfsFuseAttrSec는 FUSE 속성 캐시 유지 시간(초). 0 이면 드라이버 기본값을 쓴다.
	nfsFuseAttrSec int
}

// WithNFSFuse는 공유 NFS 볼륨을 CSI(FUSE) 경로로 붙일지 정한다(드라이버가 설치된 배포에서만 켠다).
// attrSec 은 FUSE 속성 캐시 유지 시간(초)이며 0 이면 드라이버 기본값을 쓴다.
func (c *Client) WithNFSFuse(on bool, attrSec int) *Client {
	if c != nil {
		c.nfsFuse, c.nfsFuseAttrSec = on, attrSec
	}
	return c
}

// WithCudaLabel은 노드 CUDA 버전 라벨명을 설정한다(빈 값이면 무시).
func (c *Client) WithCudaLabel(label string) *Client {
	if c != nil && label != "" {
		c.cudaLabel = label
	}
	return c
}

// WithPhysicalLabel은 물리노드 식별 라벨을 설정한다(빈 값이면 기본 유지).
func (c *Client) WithPhysicalLabel(label string) *Client {
	if c != nil && label != "" {
		c.physicalLabel = label
	}
	return c
}

// physLabel은 유효 물리노드 라벨(미설정 시 기본).
func (c *Client) physLabel() string {
	if c.physicalLabel != "" {
		return c.physicalLabel
	}
	return "giosk.io/physical"
}

// New는 in-cluster 를 먼저 시도하고 안 되면 kubeconfig 로 접속한다.
// 실패하면 (nil, nil)을 준다. 클러스터 없이도 비-K8s 기능은 계속 동작해야 한다.
func New(kubeconfig, gpuTypeLabel string) (*Client, error) {
	cfg, err := restConfig(kubeconfig)
	if err != nil {
		return nil, nil
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil
	}
	return &Client{cs: cs, cfg: cfg, gpuTypeLabel: gpuTypeLabel}, nil
}

// restConfig는 in-cluster 우선, 실패 시 kubeconfig 경로(또는 ~/.kube/config).
func restConfig(kubeconfig string) (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	path := kubeconfig
	if path == "" {
		if home := homedir.HomeDir(); home != "" {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	return clientcmd.BuildConfigFromFlags("", path)
}
