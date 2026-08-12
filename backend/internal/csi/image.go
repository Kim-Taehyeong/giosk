// Package csi는 Giosk 전용 CSI 드라이버다. 세션 홈처럼 "노드 로컬이면서 크기를 진짜로
// 강제해야 하는" 볼륨을 파일시스템 이미지로 만들어 루프 디바이스로 붙인다.
//
// 왜 이미지인가. 노드 디렉터리를 그대로 내주면(local-path 방식) PVC 에 적은 용량이 지켜지지
// 않는다. 사용자는 df 에서 노드 디스크 전체를 보고, 실제로 그만큼 쓸 수 있다. 이미지 하나가
// 곧 파일시스템이면 그 크기가 경계 자체라, 넘기면 커널이 ENOSPC 로 막는다. 쿼터를 따로 걸
// 필요도 없다.
//
// 이미지는 희소 파일로 만든다. 파일시스템 크기는 처음부터 상한이라 나중에 늘릴 일이 없고
// (자라기를 기다리다 ENOSPC 를 맞는 경합이 없다), 물리 블록은 쓴 만큼만 잡힌다. 지운 공간은
// discard 마운트로 되돌아간다.
package csi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ImageStore는 노드 로컬 디스크 위의 볼륨 이미지 저장소다.
//   <root>/images/<volumeID>.img   파일시스템 이미지(희소)
//   <root>/mounts/<volumeID>       마운트 지점
type ImageStore struct {
	Root string // 예: /var/lib/giosk/csi
	FS   string // 이미지 파일시스템 종류(xfs). discard 로 공간 회수가 되는 것이어야 한다.
}

func (s *ImageStore) imagePath(volumeID string) string {
	return filepath.Join(s.Root, "images", volumeID+".img")
}

func (s *ImageStore) mountPath(volumeID string) string {
	return filepath.Join(s.Root, "mounts", volumeID)
}

// run은 외부 명령을 실행하고 실패 시 출력까지 담은 오류를 돌려준다.
// 스토리지 조작은 실패 원인이 명령 출력에만 있는 경우가 많아 그대로 보존한다.
func run(ctx context.Context, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(cctx, name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// EnsureImage는 지정 크기의 이미지가 없으면 만든다(희소 + mkfs). 이미 있으면 아무것도 하지 않는다.
// 크기를 줄이는 것은 XFS 가 지원하지 않으므로 기존 이미지의 크기는 건드리지 않는다.
func (s *ImageStore) EnsureImage(ctx context.Context, volumeID string, sizeBytes int64) error {
	img := s.imagePath(volumeID)
	if err := os.MkdirAll(filepath.Dir(img), 0o700); err != nil {
		return err
	}
	if st, err := os.Stat(img); err == nil {
		if st.Size() < sizeBytes { // 확장은 허용한다. 마운트 중이어도 losetup -c + growfs 로 반영된다.
			return s.growImage(ctx, volumeID, sizeBytes)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	// 희소 파일로 만든다. truncate 는 블록을 잡지 않으므로 이 시점 디스크 소비는 0 이다.
	f, err := os.OpenFile(img, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := f.Truncate(sizeBytes); err != nil {
		f.Close()
		_ = os.Remove(img)
		return err
	}
	f.Close()
	if _, err := run(ctx, "mkfs."+s.FS, "-q", img); err != nil {
		_ = os.Remove(img) // 반쯤 포맷된 이미지를 남기지 않는다
		return err
	}
	return nil
}

// growImage는 이미지와 그 위 파일시스템을 확장한다(마운트 중에도 가능).
func (s *ImageStore) growImage(ctx context.Context, volumeID string, sizeBytes int64) error {
	img := s.imagePath(volumeID)
	if err := os.Truncate(img, sizeBytes); err != nil {
		return err
	}
	// 마운트 중이면 루프 디바이스에 새 크기를 알리고 파일시스템을 늘린다. 마운트 전이면
	// 다음 마운트가 새 크기로 붙으므로 여기서 할 일이 없다.
	dev, err := s.loopDevice(ctx, volumeID)
	if err != nil || dev == "" {
		return nil
	}
	if _, err := run(ctx, "losetup", "-c", dev); err != nil {
		return err
	}
	if s.FS == "xfs" {
		_, err = run(ctx, "xfs_growfs", s.mountPath(volumeID))
	} else {
		_, err = run(ctx, "resize2fs", dev)
	}
	return err
}

// loopDevice는 이 볼륨 이미지에 연결된 루프 디바이스를 찾는다(없으면 빈 문자열).
func (s *ImageStore) loopDevice(ctx context.Context, volumeID string) (string, error) {
	out, err := run(ctx, "losetup", "-j", s.imagePath(volumeID))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "", nil
	}
	// "losetup -j" 출력 형식: /dev/loop0: [2049]:12345 (/path/to.img)
	dev, _, ok := strings.Cut(line, ":")
	if !ok {
		return "", nil
	}
	return dev, nil
}

// Mount는 이미지를 루프로 붙여 마운트 지점에 올린다(멱등). 이미 마운트돼 있으면 그대로 둔다.
// discard 를 켜서 이미지 안에서 파일을 지우면 백업 파일에 구멍이 뚫려 실제 공간이 회수된다.
func (s *ImageStore) Mount(ctx context.Context, volumeID string, uid int) (string, error) {
	mnt := s.mountPath(volumeID)
	if err := os.MkdirAll(mnt, 0o755); err != nil {
		return "", err
	}
	mounted, err := IsMountPoint(mnt)
	if err != nil {
		return "", err
	}
	if !mounted {
		if _, err := run(ctx, "mount", "-o", "loop,discard", s.imagePath(volumeID), mnt); err != nil {
			return "", err
		}
	}
	// 세션은 비-root 로 돌아가므로 마운트 루트를 세션 UID 소유로 만든다. 매 마운트마다
	// 확인하는 이유는 UID 가 바뀐 채 같은 이미지를 재사용하는 경우(재할당)를 덮기 위함이다.
	if uid > 0 {
		if err := os.Chown(mnt, uid, uid); err != nil {
			return "", err
		}
		if err := os.Chmod(mnt, 0o700); err != nil {
			return "", err
		}
	}
	return mnt, nil
}

// Unmount는 마운트를 해제하고 루프 디바이스를 떼어낸다(이미지는 남긴다).
// 볼륨이 사라진 게 아니라 이 노드에서 쓰지 않게 된 것뿐이므로 데이터는 보존한다.
func (s *ImageStore) Unmount(ctx context.Context, volumeID string) error {
	mnt := s.mountPath(volumeID)
	mounted, err := IsMountPoint(mnt)
	if err != nil {
		return err
	}
	if mounted {
		if _, err := run(ctx, "umount", mnt); err != nil {
			return err
		}
	}
	// mount -o loop 는 umount 시 루프를 자동 해제하지만, 마운트가 없는데 루프만 남은
	// 상태(중간 실패)가 있을 수 있어 한 번 더 확인한다.
	if dev, err := s.loopDevice(ctx, volumeID); err == nil && dev != "" {
		_, _ = run(ctx, "losetup", "-d", dev)
	}
	return os.RemoveAll(mnt)
}

// Delete는 볼륨 이미지를 지운다(마운트 해제 후). 되돌릴 수 없다.
func (s *ImageStore) Delete(ctx context.Context, volumeID string) error {
	if err := s.Unmount(ctx, volumeID); err != nil {
		return err
	}
	err := os.Remove(s.imagePath(volumeID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ListVolumeIDs는 이 노드에 이미지가 있는 볼륨 목록이다(고아 이미지 정리에 쓴다).
func (s *ImageStore) ListVolumeIDs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "images"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if n := strings.TrimSuffix(e.Name(), ".img"); n != e.Name() {
			out = append(out, n)
		}
	}
	return out, nil
}
