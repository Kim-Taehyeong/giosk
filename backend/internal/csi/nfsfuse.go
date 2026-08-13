package csi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// NFS 위에 FUSE 를 얹는 볼륨의 파라미터. PV 의 volumeAttributes 로 전달된다.
const (
	ParamNFSServer = DriverName + "/nfs-server"
	ParamNFSPath   = DriverName + "/nfs-path"
	// ParamAttrTimeout은 FUSE 속성 캐시 유지 시간(초).
	//
	// bindfs 기본값은 0(매번 하부에 물어본다)이라 메타데이터가 많은 워크로드에서 대단히 느리다.
	// 실측으로 파일 11,791개 stat 이 웜 상태에서 4,105ms 였는데, 캐시를 켜니 62ms 로 NFS 직접
	// 마운트(67ms)와 같아졌다. 다만 캐시가 있으면 다른 노드의 변경이 그만큼 늦게 보인다.
	// NFS 자체도 속성 캐시(acregmax 기본 60초)를 두므로 성격이 다르지는 않지만, 캐시가 두 겹이
	// 되므로 NFS 기본값보다 짧게 잡는다.
	ParamAttrTimeout = DriverName + "/attr-timeout-sec"
)

// defaultAttrTimeoutSec는 속성 캐시 기본값(초).
const defaultAttrTimeoutSec = 30

// nfsStagePath는 이 볼륨의 NFS 마운트 지점(노드 내부, 컨테이너에는 안 보인다).
func (s *ImageStore) nfsStagePath(volumeID string) string {
	return filepath.Join(s.Root, "nfs", volumeID)
}

// ListStageIDs는 이 노드에 NFS 스테이지가 남아 있는 볼륨 목록이다(고아 정리용).
func (s *ImageStore) ListStageIDs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "nfs"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// DeleteStage는 NFS 스테이지 마운트를 풀고 디렉터리를 지운다(없어도 성공).
// 원격 데이터는 건드리지 않는다. 이 노드가 더는 그 볼륨을 쓰지 않는다는 뜻일 뿐이다.
func (s *ImageStore) DeleteStage(ctx context.Context, volumeID string) error {
	stage := s.nfsStagePath(volumeID)
	mounted, err := IsMountPoint(stage)
	if err != nil {
		return err
	}
	if mounted {
		if _, err := run(ctx, "umount", stage); err != nil {
			return err
		}
	}
	return os.RemoveAll(stage)
}

// nfsFuseSpec은 NodePublish 요청에서 뽑은 FUSE 볼륨 입력.
type nfsFuseSpec struct {
	Server     string
	Path       string
	AttrTimeut int
	ReadOnly   bool
}

// nfsFuseFrom은 volumeAttributes 에서 FUSE 볼륨 입력을 읽는다(서버·경로가 없으면 ok=false).
func nfsFuseFrom(attrs map[string]string, readOnly bool) (nfsFuseSpec, bool) {
	server, path := attrs[ParamNFSServer], attrs[ParamNFSPath]
	if server == "" || path == "" {
		return nfsFuseSpec{}, false
	}
	sec := defaultAttrTimeoutSec
	if v, err := strconv.Atoi(attrs[ParamAttrTimeout]); err == nil && v >= 0 {
		sec = v
	}
	return nfsFuseSpec{Server: server, Path: path, AttrTimeut: sec, ReadOnly: readOnly}, true
}

// mountNFSFuse는 NFS 를 노드에 마운트하고 그 위에 FUSE 를 얹어 target 에 붙인다.
//
// 컨테이너가 보는 것은 FUSE 마운트뿐이다. NFS 마운트는 노드 안에만 있고 컨테이너 마운트
// 네임스페이스에 들어가지 않으므로, 세션에서 스토리지 주소를 알아낼 경로가 없다.
// (커널 NFS 마운트를 그대로 주면 devname 과 addr= 에 서버 주소가 그대로 찍힌다. bind 로
//
//	다시 걸어도 슈퍼블록이 같아 소용없다.)
func (d *Driver) mountNFSFuse(ctx context.Context, volID, target string, spec nfsFuseSpec) error {
	stage := d.Store.nfsStagePath(volID)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return fmt.Errorf("스테이지 경로 생성: %w", err)
	}
	staged, err := IsMountPoint(stage)
	if err != nil {
		return err
	}
	if !staged {
		// 노드가 NFS 를 마운트한다. 이 마운트는 컨테이너에 노출되지 않는다.
		src := spec.Server + ":" + spec.Path
		if _, err := run(ctx, "mount", "-t", "nfs", "-o", "vers=4.2", src, stage); err != nil {
			return fmt.Errorf("NFS 마운트: %w", err)
		}
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		return fmt.Errorf("대상 경로 생성: %w", err)
	}
	mounted, err := IsMountPoint(target)
	if err != nil {
		return err
	}
	if mounted {
		return nil // 멱등. kubelet 은 같은 호출을 여러 번 보낸다.
	}

	opts := fmt.Sprintf("allow_other,default_permissions,attr_timeout=%d,entry_timeout=%d",
		spec.AttrTimeut, spec.AttrTimeut)
	if spec.ReadOnly {
		opts += ",ro"
	}
	// bindfs 는 데몬으로 남아 있어야 마운트가 유지된다. 드라이버 컨테이너의 자식으로 띄우면
	// 파드가 재시작할 때 그 노드 모든 세션의 마운트가 한꺼번에 끊긴다. 호스트 네임스페이스에서
	// 띄워 드라이버 수명과 분리한다.
	if _, err := run(ctx, "nsenter", "-t", "1", "-m", "-p", "--",
		"bindfs", "-o", opts, stage, target); err != nil {
		return fmt.Errorf("bindfs: %w", err)
	}
	return nil
}

// unmountNFSFuse는 FUSE 마운트를 풀고, 이 볼륨을 쓰는 곳이 없으면 NFS 스테이지도 정리한다.
func (d *Driver) unmountNFSFuse(ctx context.Context, volID, target string) error {
	mounted, err := IsMountPoint(target)
	if err != nil {
		return err
	}
	if mounted {
		// FUSE 는 fusermount 로 푼다. 데몬이 호스트에 있으므로 해제도 호스트에서 한다.
		if _, err := run(ctx, "nsenter", "-t", "1", "-m", "-p", "--", "fusermount3", "-u", target); err != nil {
			// 데몬이 이미 죽어 스테일 마운트가 된 경우 fusermount 가 실패한다. 강제 분리로 정리한다.
			if _, err2 := run(ctx, "umount", "-l", target); err2 != nil {
				return fmt.Errorf("FUSE 해제: %w", err)
			}
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	// NFS 스테이지는 같은 노드의 다른 파드가 같은 볼륨을 쓰고 있을 수 있어 참조가 없을 때만 푼다.
	// 남겨 두어도 동작에는 문제가 없고 고아 정리가 걷어가므로, 실패는 무시한다.
	stage := d.Store.nfsStagePath(volID)
	if staged, err := IsMountPoint(stage); err == nil && staged {
		_, _ = run(ctx, "umount", stage)
	}
	return nil
}
