// Giosk CSI 드라이버. 하나의 바이너리가 노드 플러그인과 컨트롤러 플러그인을 겸한다.
//
//	노드 플러그인   : GIOSK_CSI_NODE_ID 설정(DaemonSet, privileged). 이미지 생성·루프 마운트 담당.
//	컨트롤러 플러그인: GIOSK_CSI_NODE_ID 미설정(Deployment). 볼륨 등록과 토폴로지 반환만.
package main

import (
	"log"
	"os"

	"giosk/internal/csi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[csi] ")

	endpoint := env("CSI_ENDPOINT", "unix:///csi/csi.sock")
	d := &csi.Driver{NodeID: os.Getenv("GIOSK_CSI_NODE_ID")}
	if d.NodeID != "" {
		d.Store = &csi.ImageStore{
			Root: env("GIOSK_CSI_DATA_ROOT", "/var/lib/giosk/csi"),
			FS:   env("GIOSK_CSI_FSTYPE", "xfs"),
		}
	}
	if err := csi.Serve(endpoint, d); err != nil {
		log.Fatalf("드라이버 기동 실패: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
