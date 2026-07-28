package k8s

import (
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitPVCsBound은 지정한 PVC 들이 모두 Bound 될 때까지(또는 timeout) 기다린다.
// PVC 는 생성 즉시 바인딩되지 않는데(비동기), hami-scheduler 는 unbound PVC 로 스케줄 실패 시
// 재큐잉하지 않아 Pod 가 영구 Pending 이 된다 → Pod 생성 전에 바인딩을 보장한다.
func (c *Client) WaitPVCsBound(ctx context.Context, ns string, names []string, timeout time.Duration) {
	if !c.Available() || len(names) == 0 {
		return
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		allBound := true
		for _, n := range names {
			pvc, err := c.cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx2, n, metav1.GetOptions{})
			if err != nil || pvc.Status.Phase != corev1.ClaimBound {
				allBound = false
				break
			}
		}
		if allBound {
			return
		}
		select {
		case <-ctx2.Done():
			return // 타임아웃 — 그래도 Pod 생성 시도(최선)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// SessionSpec은 세션 Pod 프로비저닝 입력(도메인 → k8s 변환 결과).
type SessionSpec struct {
	Namespace   string
	Name        string // = instance_id
	Image       string // repo:tag
	GpuType     string // 노드 라벨 값(빈값=CPU 전용/지정 없음)
	GpuMode     string // shared | exclusive | cpu
	GpuCount    int
	VramMB      int
	CorePercent int
	CPUCores    int
	MemGB       int
	EphemeralGiB int             // 컨테이너 쓰기레이어+emptyDir 상한(GiB). 0=기본 캡. 노드 루트디스크 무단점유(DoS) 방지.
	Labels      map[string]string
	UID         int              // 안정 사용자 UID(uidBase+userID). 컨테이너를 이 UID 로 실행 → NFS 볼륨 파일 소유가 물리 SSH 와 일관.
	HomePVC     string           // 홈 영속 PVC 이름(/home/work 마운트). 빈값이면 미마운트.
	Volumes     []VolMountSpec   // 추가 볼륨 마운트(PVC 기반)
	WebChannels []WebChannelSpec // 활성 웹 채널(code-server/jupyter): 포트 + 인증 env
	SSHDImage   string           // 컨테이너 SSH 사이드카 이미지(빈값=사이드카 없음). 직접 SSH·게이트웨이 프록시 공용.
	SSHDPubKey  string           // 사이드카 sshd 가 신뢰할 게이트웨이 공개키(authorized_keys). 게이트웨이 off 면 빈값.
	// 사용자 등록 공개키를 담은 Secret 이름(키: authorized_keys). 사이드카에 read-only 로 마운트되어
	// sshd 가 접속마다 다시 읽는다 → 키 등록/교체가 실행 중 세션에도 즉시 반영. 빈값/미존재면 마운트 생략(Optional).
	UserKeysSecret string
	ScratchHost string           // 노드로컬 스크래치 계정폴더(hostPath). 빈값이면 미마운트.
	PreferNodes []string         // 이미지 캐시된 노드(소프트 선호) — 빠른 시작. nodeSelector(타입) 안에서만 효과.
	RequireNode string           // 하드 핀(required nodeAffinity hostname) — 데이터셋 노드 로컬 캐시 hostPath 마운트용.
}

// VolMountSpec은 세션 Pod 에 붙일 PVC 마운트 1건.
type VolMountSpec struct {
	PVCName   string
	HostPath  string // 설정 시 PVC 대신 노드 로컬 hostPath 마운트(데이터셋 노드 캐시·로컬 Home). RequireNode 와 함께 사용.
	EmptyDir  bool   // 설정 시 emptyDir(임시) 마운트 — 세션(파드) 소멸 시 데이터도 소멸. 임시 워크스페이스용.
	MountPath string
	ReadOnly  bool
}

// WebChannelSpec은 세션이 노출하는 웹 채널 1개(포트 + 인증 시크릿 env).
// code-server={Port:8080,EnvKey:PASSWORD}, jupyter={Port:8888,EnvKey:JUPYTER_TOKEN}.
type WebChannelSpec struct {
	Name   string // vscode | jupyter
	Port   int
	EnvKey string
	Secret string
}

// PodStatus는 라이브 Pod 상태(목록/상세 머지용).
type PodStatus struct {
	Phase string
	Node  string
	IP    string
}

// EnsureNamespace는 세션 네임스페이스를 멱등 생성한다(managed-by 라벨).
func (c *Client) EnsureNamespace(ctx context.Context, ns string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	_, err := c.cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
	}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// CreateSessionPod은 세션 Pod 를 생성한다(GPU 리소스/노드셀렉터는 mapper 가 구성).
func (c *Client) CreateSessionPod(ctx context.Context, s SessionSpec) error {
	if !c.Available() {
		return ErrNoCluster
	}
	pod := buildPod(s, c.gpuTypeLabel)
	_, err := c.cs.CoreV1().Pods(s.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// DeleteSessionPod은 세션 Pod 를 삭제한다(없어도 성공).
func (c *Client) DeleteSessionPod(ctx context.Context, ns, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	err := c.cs.CoreV1().Pods(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// PodStatus는 Pod 의 현재 phase/노드/IP 를 조회한다.
func (c *Client) PodStatus(ctx context.Context, ns, name string) (*PodStatus, error) {
	if !c.Available() {
		return nil, ErrNoCluster
	}
	p, err := c.cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &PodStatus{Phase: string(p.Status.Phase), Node: p.Spec.NodeName, IP: p.Status.PodIP}, nil
}

// PodDescribe는 kubectl describe 에 해당하는 요약(파드/컨테이너 상태·조건·이벤트)을 반환한다.
type PodDescribe struct {
	Phase      string           `json:"phase"`
	Node       string           `json:"node"`
	IP         string           `json:"ip"`
	Reason     string           `json:"reason,omitempty"`
	Message    string           `json:"message,omitempty"`
	StartTime  string           `json:"startTime,omitempty"`
	Containers []ContainerState `json:"containers"`
	Conditions []PodCondition   `json:"conditions"`
	Events     []PodEvent       `json:"events"`
}

// ContainerState — 컨테이너 하나의 상태(대기/실행/종료 사유 포함 — 오류 진단용).
type ContainerState struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state"`             // running | waiting | terminated
	Reason       string `json:"reason,omitempty"`  // CrashLoopBackOff | ImagePullBackOff | OOMKilled | Error ...
	Message      string `json:"message,omitempty"`
	ExitCode     int32  `json:"exitCode,omitempty"`
	Image        string `json:"image,omitempty"`
}

type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// PodEvent — 파드에 연결된 k8s 이벤트(Warning=오류 진단의 핵심).
type PodEvent struct {
	Type    string `json:"type"` // Normal | Warning
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Count   int32  `json:"count"`
	Last    string `json:"last"`
}

// PodDescribe는 파드 상세 + 관련 이벤트를 모아 반환한다(관제 진단용). 파드 없으면 ErrNotFound.
func (c *Client) PodDescribe(ctx context.Context, ns, name string) (*PodDescribe, error) {
	if !c.Available() {
		return nil, ErrNoCluster
	}
	p, err := c.cs.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d := &PodDescribe{
		Phase: string(p.Status.Phase), Node: p.Spec.NodeName, IP: p.Status.PodIP,
		Reason: p.Status.Reason, Message: p.Status.Message,
	}
	if p.Status.StartTime != nil {
		d.StartTime = p.Status.StartTime.Format("2006-01-02T15:04:05Z")
	}
	for _, cs := range p.Status.ContainerStatuses {
		st := ContainerState{Name: cs.Name, Ready: cs.Ready, RestartCount: cs.RestartCount, Image: cs.Image}
		switch {
		case cs.State.Waiting != nil:
			st.State, st.Reason, st.Message = "waiting", cs.State.Waiting.Reason, cs.State.Waiting.Message
		case cs.State.Terminated != nil:
			st.State, st.Reason, st.Message, st.ExitCode = "terminated", cs.State.Terminated.Reason, cs.State.Terminated.Message, cs.State.Terminated.ExitCode
		case cs.State.Running != nil:
			st.State = "running"
		}
		d.Containers = append(d.Containers, st)
	}
	for _, cond := range p.Status.Conditions {
		d.Conditions = append(d.Conditions, PodCondition{Type: string(cond.Type), Status: string(cond.Status), Reason: cond.Reason, Message: cond.Message})
	}
	// 이벤트 — 이 파드에 연결된 것만(오류=Warning 우선 진단).
	evs, err := c.cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", name),
	})
	if err == nil {
		for _, e := range evs.Items {
			last := e.LastTimestamp.Format("2006-01-02T15:04:05Z")
			if e.LastTimestamp.IsZero() {
				last = e.EventTime.Format("2006-01-02T15:04:05Z")
			}
			d.Events = append(d.Events, PodEvent{Type: e.Type, Reason: e.Reason, Message: e.Message, Count: e.Count, Last: last})
		}
	}
	return d, nil
}

// PodLogs는 세션 컨테이너 로그 tail 을 가져온다.
func (c *Client) PodLogs(ctx context.Context, ns, name string, tail int64) (string, error) {
	if !c.Available() {
		return "", ErrNoCluster
	}
	req := c.cs.CoreV1().Pods(ns).GetLogs(name, &corev1.PodLogOptions{TailLines: &tail})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	b, err := io.ReadAll(stream)
	return string(b), err
}
