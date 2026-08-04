package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SharedNFSSpec은 다른 네임스페이스의 NFS 볼륨을 세션 네임스페이스에 정적 PV+PVC 로 복제하는 입력.
type SharedNFSSpec struct {
	Namespace string // 세션(빌리는 사람) 네임스페이스
	Name      string // 세션 ns 안에서 만들 PVC 이름
	NFSServer string
	NFSPath   string
	SizeGiB   int
}

// PVCBackingNFS는 PVC 에서 PV 를 따라가 NFS 백엔드(server, path)를 반환한다(NFS 백엔드가 아니면 ok=false).
// 교차 네임스페이스 공유와 물리노드 직접 마운트에서 같은 NFS 경로를 재노출할 때 쓴다.
func (c *Client) PVCBackingNFS(ctx context.Context, ns, name string) (server, path string, ok bool) {
	if !c.Available() {
		return "", "", false
	}
	pvc, err := c.cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil || pvc.Spec.VolumeName == "" {
		return "", "", false
	}
	pv, err := c.cs.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil || pv.Spec.NFS == nil {
		return "", "", false
	}
	return pv.Spec.NFS.Server, pv.Spec.NFS.Path, true
}

// EnsureSharedNFSPVC는 동일 NFS 경로를 가리키는 정적 PV+PVC 를 세션 네임스페이스에 멱등 생성한다.
// 동적 NFS PV 는 1:1 바인딩이라 다른 ns 세션에 직접 못 붙으므로, 같은 server:path 를 가리키는
// 정적 PV(ReclaimPolicy=Retain)와 그 PV 에 바인딩되는 PVC 를 만들어 공유 마운트를 가능케 한다.
// RO/RW 강제는 여기서 굽지 않고 Pod 마운트(readOnly)가 담당한다. 공유 권한이 ro 에서 rw 로
// 바뀌어도 정적 PV 가 ro 로 고정돼 rw 를 막는 stale 문제를 피하기 위함(권한은 매 마운트 재평가).
func (c *Client) EnsureSharedNFSPVC(ctx context.Context, s SharedNFSSpec) error {
	if !c.Available() {
		return ErrNoCluster
	}
	if err := c.EnsureNamespace(ctx, s.Namespace); err != nil {
		return err
	}
	gib := s.SizeGiB
	if gib < 1 {
		gib = 1
	}
	size := resource.MustParse(fmt.Sprintf("%dGi", gib))
	pvName := fmt.Sprintf("giosk-shared-%s-%s", s.Namespace, s.Name)
	static := "" // 정적 바인딩이라 동적 프로비저너를 쓰지 않는다

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName, Labels: map[string]string{"managed-by": "giosk-system"}},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: size},
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              static,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{Server: s.NFSServer, Path: s.NFSPath},
			},
			ClaimRef: &corev1.ObjectReference{Namespace: s.Namespace, Name: s.Name},
		},
	}
	if _, err := c.cs.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.Name, Namespace: s.Namespace,
			Labels: map[string]string{"managed-by": "giosk-system"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			StorageClassName: &static,
			VolumeName:       pvName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if _, err := c.cs.CoreV1().PersistentVolumeClaims(s.Namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// DeleteSharedNFSPVC는 EnsureSharedNFSPVC 로 만든 정적 PVC+PV 를 삭제한다(없어도 성공).
// 정적 PV 라 PVC 삭제만으로는 회수되지 않으므로 PV 도 함께 지운다(원본 NFS 데이터는 보존).
func (c *Client) DeleteSharedNFSPVC(ctx context.Context, ns, name string) error {
	if !c.Available() {
		return ErrNoCluster
	}
	pvName := fmt.Sprintf("giosk-shared-%s-%s", ns, name)
	if err := c.cs.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := c.cs.CoreV1().PersistentVolumes().Delete(ctx, pvName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
