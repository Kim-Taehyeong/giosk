package csi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newGCWith는 가짜 API 와 임시 저장소로 회수기를 만든다.
func newGCWith(t *testing.T, pvVolumeHandles []string, imageIDs []string) (*GC, *ImageStore) {
	t.Helper()
	objs := make([]runtime.Object, 0, len(pvVolumeHandles))
	for _, h := range pvVolumeHandles {
		objs = append(objs, &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: h},
			Spec: corev1.PersistentVolumeSpec{PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: DriverName, VolumeHandle: h},
			}},
		})
	}
	cs := fake.NewSimpleClientset(objs...)

	// 회수는 마운트 해제를 거치므로 마운트 테이블을 읽는다. 테스트 환경에는 procfs 가
	// 없을 수 있어 "아무것도 마운트되지 않은" 빈 테이블을 가리키게 한다.
	empty := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := mountInfoPath
	mountInfoPath = empty
	t.Cleanup(func() { mountInfoPath = old })

	store := &ImageStore{Root: t.TempDir(), FS: "xfs"}
	if err := os.MkdirAll(filepath.Join(store.Root, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range imageIDs {
		if err := os.WriteFile(store.imagePath(id), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &GC{Store: store, cs: cs, suspects: map[string]bool{}}, store
}

// 살아있는 PV 의 이미지는 절대 지우면 안 된다. 지우면 사용자 데이터가 사라진다.
func TestGCKeepsLiveVolumes(t *testing.T) {
	gc, store := newGCWith(t, []string{"pvc-live"}, []string{"pvc-live"})
	for i := 0; i < 3; i++ {
		if err := gc.sweep(context.Background()); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	if _, err := os.Stat(store.imagePath("pvc-live")); err != nil {
		t.Errorf("살아있는 볼륨의 이미지가 사라졌다: %v", err)
	}
}

// 고아는 연속 두 번 확인한 뒤에 지운다. 한 번 만에 지우면 볼륨 생성 직후처럼
// PV 가 아직 안 보이는 순간에 갓 만든 이미지를 날릴 수 있다.
func TestGCDeletesOrphanOnlyAfterTwoSweeps(t *testing.T) {
	gc, store := newGCWith(t, nil, []string{"pvc-orphan"})
	if err := gc.sweep(context.Background()); err != nil {
		t.Fatalf("첫 sweep: %v", err)
	}
	if _, err := os.Stat(store.imagePath("pvc-orphan")); err != nil {
		t.Fatalf("첫 스캔에서 바로 지워졌다: %v", err)
	}
	if err := gc.sweep(context.Background()); err != nil {
		t.Fatalf("두번째 sweep: %v", err)
	}
	if _, err := os.Stat(store.imagePath("pvc-orphan")); !os.IsNotExist(err) {
		t.Errorf("두 번 확인 후에도 고아가 남았다: %v", err)
	}
}

// API 목록을 못 얻으면 아무것도 지우지 않아야 한다. 목록이 비어 보인다고 지우면
// 일시적 장애에 전 노드의 볼륨을 날린다. 의심 목록도 리셋해 다시 두 번부터 센다.
func TestGCDeletesNothingWhenAPIFails(t *testing.T) {
	gc, store := newGCWith(t, nil, []string{"pvc-a"})
	if err := gc.sweep(context.Background()); err != nil { // 1회: 의심 등록
		t.Fatal(err)
	}
	cs := gc.cs.(*fake.Clientset)
	cs.PrependReactor("list", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api 장애")
	})
	if err := gc.sweep(context.Background()); err == nil {
		t.Fatal("API 실패가 오류로 보고되지 않았다")
	}
	if _, err := os.Stat(store.imagePath("pvc-a")); err != nil {
		t.Errorf("API 실패 중에 이미지를 지웠다: %v", err)
	}
	if len(gc.suspects) != 0 {
		t.Errorf("API 실패 후 의심 목록이 초기화되지 않았다: %v", gc.suspects)
	}
}
