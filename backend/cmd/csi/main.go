// Giosk CSI 드라이버. 하나의 바이너리가 노드 플러그인과 컨트롤러 플러그인을 겸한다.
//
//	노드 플러그인   : GIOSK_CSI_NODE_ID 설정(DaemonSet, privileged). 이미지 생성·루프 마운트 담당.
//	컨트롤러 플러그인: GIOSK_CSI_NODE_ID 미설정(Deployment). 볼륨 등록과 토폴로지 반환만.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

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
		startGC(d.Store)
	}
	if err := csi.Serve(endpoint, d); err != nil {
		log.Fatalf("드라이버 기동 실패: %v", err)
	}
}

// startGC는 고아 이미지 회수기를 띄운다. 노드 로컬 이미지는 컨트롤러가 지울 수 없어
// 각 노드가 자기 디스크를 보며 PV 가 사라진 이미지를 회수한다. 이게 없으면 세션을
// 지워도 이미지가 남아 디스크가 계속 줄어든다.
//
// API 에 못 붙어도 드라이버는 계속 뜬다. 마운트는 API 없이도 되어야 하고, 회수가
// 늦어지는 것이 세션이 안 뜨는 것보다 낫다.
func startGC(store *csi.ImageStore) {
	mins, err := strconv.Atoi(env("GIOSK_CSI_GC_INTERVAL_MIN", "10"))
	if err != nil || mins <= 0 {
		log.Printf("고아 이미지 회수 비활성(GIOSK_CSI_GC_INTERVAL_MIN=%q)", os.Getenv("GIOSK_CSI_GC_INTERVAL_MIN"))
		return
	}
	gc, err := csi.NewGC(store, time.Duration(mins)*time.Minute)
	if err != nil {
		log.Printf("고아 이미지 회수기 기동 실패(마운트는 계속 동작): %v", err)
		return
	}
	go gc.Run(context.Background())
	log.Printf("고아 이미지 회수기 기동 (%d분 주기)", mins)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
