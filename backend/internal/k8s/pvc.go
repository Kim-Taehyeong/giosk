package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCSpec은 볼륨 PVC 프로비저닝 입력.
type PVCSpec struct {
	Namespace    string
	Name         string
	SizeGiB      int
	StorageClass string
	AccessMode   string // RWO | RWX
}

// CreatePVC는 PVC 를 멱등 생성한다(네임스페이스는 미리 보장돼야 함).
func (c *Client) CreatePVC(ctx context.Context, s PVCSpec) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, s.Namespace); err != nil {
		return err
	}
	pvc := buildPVC(s)
	_, err := c.cs.CoreV1().PersistentVolumeClaims(s.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// PVCPhase는 PVC 의 phase(Pending/Bound/...)를 반환한다.
func (c *Client) PVCPhase(ctx context.Context, ns, name string) (string, error) {
	if !c.Available() {
		return "", ErrNoCluster
	}
	pvc, err := c.cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(pvc.Status.Phase), nil
}

// DeletePVC는 PVC 를 삭제한다(없어도 성공).
func (c *Client) DeletePVC(ctx context.Context, ns, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	err := c.cs.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// PVCRef는 클러스터 전역 PVC 조회 결과(네임스페이스 + 이름 + 생성시각).
// CreatedAt 은 고아 판정에 꼭 필요하다. 세션 생성은 PVC 를 먼저 만들고 DB 행을 나중에 쓰므로,
// 갓 만들어진 PVC 는 "아직 행이 없을 뿐" 고아가 아니다(유예 없이 지우면 생성 중 세션을 깬다).
type PVCRef struct {
	Namespace, Name string
	CreatedAt       time.Time
}

// ListPVCsByPrefix는 전 네임스페이스에서 giosk 가 만든 PVC 중 이름이 prefix 로 시작하는 것을 반환한다.
// 세션 홈(sh-*) 고아 PVC 탐지용이다. 세션 ns 가 팀별로 갈리므로 ns 를 열거하지 않고 전역으로 훑는다.
// managed-by 라벨로 좁혀 외부 PVC 는 애초에 후보에 들어오지 않는다(오삭제 방지).
func (c *Client) ListPVCsByPrefix(ctx context.Context, prefix string) ([]PVCRef, error) {
	if !c.Available() {
		return nil, ErrNoCluster
	}
	list, err := c.cs.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).
		List(ctx, metav1.ListOptions{LabelSelector: "managed-by=giosk-system"})
	if err != nil {
		return nil, err
	}
	var out []PVCRef
	for i := range list.Items {
		p := &list.Items[i]
		if strings.HasPrefix(p.Name, prefix) {
			out = append(out, PVCRef{Namespace: p.Namespace, Name: p.Name, CreatedAt: p.CreationTimestamp.Time})
		}
	}
	return out, nil
}

func buildPVC(s PVCSpec) *corev1.PersistentVolumeClaim {
	sc := s.StorageClass
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.Name, Namespace: s.Namespace,
			Labels: map[string]string{"managed-by": "giosk-system"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{accessMode(s.AccessMode)},
			StorageClassName: &sc,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", s.SizeGiB)),
				},
			},
		},
	}
}

func accessMode(m string) corev1.PersistentVolumeAccessMode {
	if m == "RWX" {
		return corev1.ReadWriteMany
	}
	return corev1.ReadWriteOnce
}
