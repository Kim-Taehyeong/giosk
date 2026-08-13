package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// 붙이는 방식이 바뀌었는지 판정하는 자리다. 여기가 틀리면 예전 PV 가 그대로 남아
// 컨테이너에 스토리지 주소가 계속 보이거나, 멀쩡한 PV 를 매번 다시 만든다.
func TestSameVolumeKind(t *testing.T) {
	nfs := corev1.PersistentVolumeSource{NFS: &corev1.NFSVolumeSource{Server: "10.0.0.1", Path: "/export"}}
	csiSrc := corev1.PersistentVolumeSource{CSI: &corev1.CSIPersistentVolumeSource{Driver: "csi.giosk.io"}}
	if sameVolumeKind(nfs, csiSrc) {
		t.Error("NFS 직접과 CSI 를 같다고 본다")
	}
	if !sameVolumeKind(csiSrc, csiSrc) || !sameVolumeKind(nfs, nfs) {
		t.Error("같은 방식을 다르다고 본다")
	}
}
