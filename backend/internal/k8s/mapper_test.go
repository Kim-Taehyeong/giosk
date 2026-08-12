package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

// evalReqs는 노드 라벨 집합이 NodeSelectorRequirement 들을 모두 만족하는지 본다(한 term 안은 AND).
// 스케줄러가 required nodeAffinity 를 평가하는 방식과 같다(여기서 쓰는 연산자는 In/NotIn 뿐).
func evalReqs(reqs []corev1.NodeSelectorRequirement, labels map[string]string) bool {
	for _, r := range reqs {
		v, has := labels[r.Key]
		in := false
		for _, want := range r.Values {
			if has && v == want {
				in = true
				break
			}
		}
		switch r.Operator {
		case corev1.NodeSelectorOpIn:
			if !in {
				return false
			}
		case corev1.NodeSelectorOpNotIn:
			// 라벨이 없는 노드도 NotIn 을 만족한다(스케줄러 동작과 동일).
			if in {
				return false
			}
		}
	}
	return true
}

// ShareModeAllows 는 하드 핀 후보를 고를 때 쓰이고, shareModeReqs 는 같은 제약을 파드에 굽는다.
// 둘이 어긋나면 핀과 affinity 가 서로를 배제해 세션이 영구 Pending 이 되므로, 모든 조합에서 같은 답인지 본다.
func TestShareModeAllowsMatchesAffinity(t *testing.T) {
	modes := []string{GpuModeExclusive, GpuModeShared, GpuModeTimeslice, GpuModeCPU, ""}
	nodeModes := []string{shareHami, shareTimeslicing, "exclusive", ""} // 빈 값 = 라벨 없는 노드

	for _, mode := range modes {
		for _, nodeMode := range nodeModes {
			labels := map[string]string{}
			if nodeMode != "" {
				labels[ShareModeLabel] = nodeMode
			}
			viaAffinity := evalReqs(shareModeReqs(SessionSpec{GpuMode: mode}), labels)
			viaPredicate := ShareModeAllows(mode, nodeMode)
			if viaAffinity != viaPredicate {
				t.Errorf("mode=%q node=%q: affinity=%v ShareModeAllows=%v (둘이 어긋나면 영구 Pending)",
					mode, nodeMode, viaAffinity, viaPredicate)
			}
		}
	}
}

// 실제로 사고가 났던 조합을 콕 집어 고정한다. 전용 세션은 HAMi 노드에 핀되면 안 된다.
func TestExclusiveNotPinnableToHamiNode(t *testing.T) {
	if ShareModeAllows(GpuModeExclusive, shareHami) {
		t.Error("전용 세션이 HAMi 노드에 배치 가능으로 판정됨: 데이터셋 캐시 핀이 여기 걸리면 영구 Pending")
	}
	if !ShareModeAllows(GpuModeExclusive, "") {
		t.Error("라벨 없는 노드는 전용 배치가 가능해야 한다")
	}
	if !ShareModeAllows(GpuModeShared, shareHami) {
		t.Error("공유 세션은 HAMi 노드에 배치 가능해야 한다")
	}
}
