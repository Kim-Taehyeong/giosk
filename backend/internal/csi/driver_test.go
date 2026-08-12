package csi

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateVolume 은 노드를 모르면 실패해야 한다. 이미지가 노드 로컬이라 노드가 정해지지
// 않은 채 볼륨을 만들면 나중에 파드가 엉뚱한 노드로 가서 영영 못 뜬다.
func TestCreateVolumeRequiresTopology(t *testing.T) {
	d := &Driver{}
	_, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:          "vol-1",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1 << 30},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("토폴로지 없는 CreateVolume 이 거부되지 않았다: %v", err)
	}
}

// 스케줄러가 고른 노드가 응답의 accessible_topology 로 나가야 external-provisioner 가
// PV 에 nodeAffinity 를 세운다. 크기는 NodePublish 가 읽을 수 있게 VolumeContext 에 실린다.
func TestCreateVolumeCarriesNodeAndSize(t *testing.T) {
	d := &Driver{}
	const size = int64(5) << 30
	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:          "vol-1",
		CapacityRange: &csi.CapacityRange{RequiredBytes: size},
		Parameters:    map[string]string{ParamUID: "100001"},
		AccessibilityRequirements: &csi.TopologyRequirement{
			Preferred: []*csi.Topology{{Segments: map[string]string{TopologyKey: "gpu2-1"}}},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume 실패: %v", err)
	}
	top := resp.GetVolume().GetAccessibleTopology()
	if len(top) != 1 || top[0].GetSegments()[TopologyKey] != "gpu2-1" {
		t.Errorf("토폴로지가 선택 노드를 담지 않았다: %+v", top)
	}
	vc := resp.GetVolume().GetVolumeContext()
	if vc[ParamSize] != strconv.FormatInt(size, 10) {
		t.Errorf("크기가 VolumeContext 에 실리지 않았다: %q", vc[ParamSize])
	}
	if vc[ParamUID] != "100001" {
		t.Errorf("StorageClass 파라미터가 전달되지 않았다: %+v", vc)
	}
}

// requisite 만 있어도(preferred 없음) 노드를 찾아야 한다.
func TestSelectedNodeFallsBackToRequisite(t *testing.T) {
	got := selectedNode(&csi.TopologyRequirement{
		Requisite: []*csi.Topology{{Segments: map[string]string{TopologyKey: "gpu2-2"}}},
	})
	if got != "gpu2-2" {
		t.Errorf("requisite 폴백 실패: %q", got)
	}
	if n := selectedNode(nil); n != "" {
		t.Errorf("토폴로지 없음은 빈 값이어야 한다: %q", n)
	}
}

// 크기를 모르면 이미지를 만들 수 없으므로 NodePublish 는 거부해야 한다.
// 조용히 기본 크기로 만들면 사용자가 요청한 것과 다른 볼륨을 받게 된다.
func TestNodePublishRejectsUnknownSize(t *testing.T) {
	d := &Driver{NodeID: "n1", Store: &ImageStore{Root: t.TempDir(), FS: "xfs"}}
	_, err := d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId: "vol-1", TargetPath: filepath.Join(t.TempDir(), "t"),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("크기 없는 NodePublish 가 거부되지 않았다: %v", err)
	}
}

// 마운트 지점 판정은 커널 마운트 테이블을 읽는다. 볼륨 이름에 공백이 들어갈 수 있어
// (실제 랩에 "테스트 볼륨" 이 있다) 8진 이스케이프를 되돌리지 않으면 마운트를 놓친다.
func TestIsMountPointHandlesEscapedPaths(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mountinfo")
	body := "" +
		"25 30 0:22 / /proc rw,relatime shared:5 - proc proc rw\n" +
		"82 29 7:0 / /home/work/테스트\\040볼륨 rw,relatime - xfs /dev/loop0 rw\n"
	if err := os.WriteFile(fake, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	old := mountInfoPath
	mountInfoPath = fake
	defer func() { mountInfoPath = old }()

	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/home/work/테스트 볼륨", true},
		{`/home/work/테스트\040볼륨`, false}, // 이스케이프된 형태로는 매칭되면 안 된다
		{"/proc", true},
		{"/home/work", false},
	} {
		got, err := IsMountPoint(tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if got != tc.want {
			t.Errorf("IsMountPoint(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// 이미지가 없으면 만들고, 있으면 건드리지 않아야 한다(재시작 때 데이터를 날리면 안 된다).
// mkfs 는 리눅스에서만 되므로 파일 생성 단계까지만 본다.
func TestEnsureImageIsIdempotent(t *testing.T) {
	s := &ImageStore{Root: t.TempDir(), FS: "xfs"}
	img := s.imagePath("vol-1")
	if err := os.MkdirAll(filepath.Dir(img), 0o700); err != nil {
		t.Fatal(err)
	}
	// mkfs 를 태우지 않으려고 이미 포맷된 것처럼 파일을 미리 만들어 둔다.
	if err := os.WriteFile(img, make([]byte, 1<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(img)
	if err := s.EnsureImage(context.Background(), "vol-1", 1<<20); err != nil {
		t.Fatalf("기존 이미지에 EnsureImage 실패: %v", err)
	}
	after, _ := os.Stat(img)
	if before.Size() != after.Size() {
		t.Errorf("기존 이미지 크기가 바뀌었다: %d -> %d", before.Size(), after.Size())
	}
}
