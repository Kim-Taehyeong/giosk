package k8s

import (
	"context"
	"fmt"

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
