package metrics

import (
	"strings"
	"testing"
)

// DCGM 워크로드 Pod 라벨은 배포에 따라 pod/exported_pod 로 갈린다. 한쪽만 보면 조용히 빈 결과가 되고
// (실제로 사용자 VRAM 이 늘 0, GPU 유휴 리퍼가 영구 무력이었다) 가짜 Prometheus 로는 못 잡는 종류라
// 최소한 두 항이 모두 들어갔는지를 문자열로 잠근다.
func TestDCGMPodSeries_CoversBothLabelLayouts(t *testing.T) {
	q := DCGMPodSeries("DCGM_FI_DEV_FB_USED", "ses-a|ses-b")

	if !strings.Contains(q, `DCGM_FI_DEV_FB_USED{exported_pod=~"ses-a|ses-b"}`) {
		t.Errorf("exported_pod 항 없음: %s", q)
	}
	if !strings.Contains(q, `DCGM_FI_DEV_FB_USED{pod=~"ses-a|ses-b"}`) {
		t.Errorf("pod 항 없음: %s", q)
	}
	if !strings.Contains(q, ` or `) {
		t.Errorf("두 항이 or 로 이어져 있지 않음: %s", q)
	}
	// 결과 라벨이 pod 으로 정규화돼야 호출부의 by(pod) 집계가 두 배포에서 같게 동작한다.
	if !strings.Contains(q, `label_replace(`) || !strings.Contains(q, `"pod", "$1", "exported_pod", "(.*)"`) {
		t.Errorf("exported_pod → pod 정규화 없음: %s", q)
	}
}

func TestDCGMPodScalar_CoversBothLabelLayouts(t *testing.T) {
	q := DCGMPodScalar(`avg_over_time(DCGM_FI_DEV_GPU_UTIL{%s}[5m])`, `"ses-a"`)

	if !strings.Contains(q, `DCGM_FI_DEV_GPU_UTIL{exported_pod="ses-a"}`) {
		t.Errorf("exported_pod 항 없음: %s", q)
	}
	if !strings.Contains(q, `DCGM_FI_DEV_GPU_UTIL{pod="ses-a"}`) {
		t.Errorf("pod 항 없음: %s", q)
	}
}
