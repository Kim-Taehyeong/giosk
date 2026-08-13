package k8s

import (
	"context"
	"fmt"
	"log"
	"time"

	"strconv"

	"giosk/internal/csi"

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
			PersistentVolumeSource:        c.sharedVolumeSource(pvName, s),
			ClaimRef:                      &corev1.ObjectReference{Namespace: s.Namespace, Name: s.Name},
		},
	}
	// 붙이는 방식이 바뀌었는데 PV 가 예전 방식으로 남아 있으면 갈아 끼운다. 정적 PV 는 이름으로
	// 재사용되므로 그냥 두면 영원히 옛 방식으로 붙는다. 실제로 NFS 직접 마운트에서 FUSE 로
	// 바꾼 뒤에도 예전 PV 를 쓰는 데이터셋만 컨테이너에 스토리지 주소가 그대로 보였다.
	// 데이터는 손대지 않는다. 회수 정책이 Retain 이고 원격 디렉터리는 건드리지 않는다.
	if err := c.replaceStaleSharedPV(ctx, pvName, s.Namespace, s.Name, pv.Spec.PersistentVolumeSource); err != nil {
		return err
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

// sharedVolumeSource는 이 PV 를 어떤 방식으로 붙일지 고른다.
//
// FUSE 가 켜져 있으면 우리 CSI 드라이버로 보낸다. 드라이버가 NFS 를 노드에서 마운트하고
// 그 위에 FUSE 를 얹으므로, 컨테이너에는 fuse 마운트만 보이고 스토리지 주소가 남지 않는다.
// 커널 NFS 마운트를 그대로 주면 devname 과 addr= 에 서버 주소가 찍히고, bind 로 다시 걸어도
// 슈퍼블록이 같아 지워지지 않는다.
//
// 꺼져 있으면 예전대로 NFS 를 직접 붙인다(드라이버 미설치 배포).
func (c *Client) sharedVolumeSource(pvName string, s SharedNFSSpec) corev1.PersistentVolumeSource {
	if !c.nfsFuse {
		return corev1.PersistentVolumeSource{
			NFS: &corev1.NFSVolumeSource{Server: s.NFSServer, Path: s.NFSPath},
		}
	}
	return corev1.PersistentVolumeSource{
		CSI: &corev1.CSIPersistentVolumeSource{
			Driver: csi.DriverName,
			// 볼륨 핸들은 PV 이름과 같게 둔다. 노드 플러그인의 고아 정리가 PV 목록과
			// 대조할 때 이 값을 쓰므로 PV 와 1:1 이어야 한다.
			VolumeHandle: pvName,
			VolumeAttributes: map[string]string{
				csi.ParamNFSServer:   s.NFSServer,
				csi.ParamNFSPath:     s.NFSPath,
				csi.ParamAttrTimeout: strconv.Itoa(c.nfsFuseAttrSec),
			},
		},
	}
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

// replaceStaleSharedPV는 기존 PV 의 붙이는 방식이 지금 방식과 다르면 PVC 와 함께 지운다.
// 쓰고 있는 파드가 있으면 건드리지 않는다. 실행 중인 세션의 마운트를 뽑는 것보다 다음 세션까지
// 옛 방식으로 붙는 편이 낫다. 세션이 끝나면 다음 생성에서 갈린다.
func (c *Client) replaceStaleSharedPV(ctx context.Context, pvName, ns, pvcName string, want corev1.PersistentVolumeSource) error {
	cur, err := c.cs.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return nil // 없으면 새로 만들면 된다. 조회 실패로 생성을 막지 않는다.
	}
	if sameVolumeKind(cur.Spec.PersistentVolumeSource, want) {
		return nil
	}
	used, err := c.pvcInUse(ctx, ns, pvcName)
	if err != nil || used {
		return nil
	}
	log.Printf("[k8s] 공유 볼륨 %s 를 지금 방식으로 다시 만든다(예전 PV 교체)", pvName)
	_ = c.cs.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, pvcName, metav1.DeleteOptions{})
	if err := c.cs.CoreV1().PersistentVolumes().Delete(ctx, pvName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	// PV 는 PVC 가 정리될 때까지 Terminating 으로 남는다. 사라진 뒤에 만들어야 이름이 겹치지 않는다.
	for i := 0; i < 30; i++ {
		if _, err := c.cs.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("공유 볼륨 %s 가 아직 삭제되지 않았다", pvName)
}

// sameVolumeKind는 두 볼륨 소스가 같은 방식(CSI 인지 NFS 직접인지)인지 본다.
func sameVolumeKind(a, b corev1.PersistentVolumeSource) bool {
	return (a.CSI != nil) == (b.CSI != nil) && (a.NFS != nil) == (b.NFS != nil)
}

// pvcInUse는 그 네임스페이스에 이 PVC 를 마운트한 파드가 있는지 본다.
func (c *Client) pvcInUse(ctx context.Context, ns, pvcName string) (bool, error) {
	pods, err := c.cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, err
	}
	for _, p := range pods.Items {
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
				return true, nil
			}
		}
	}
	return false, nil
}
