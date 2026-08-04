package metrics

import "fmt"

// DCGM 메트릭에서 "GPU 를 쓰는 워크로드 Pod" 를 고르는 라벨은 배포에 따라 갈린다.
//
// dcgm-exporter 는 GPU 를 점유 중인 워크로드의 pod/namespace/container 를 라벨로 붙여 노출한다.
// 그런데 Prometheus 는 스크레이프 대상(= exporter 자신의 Pod)의 pod/namespace 라벨을 이미 붙이므로
// 이름이 충돌한다. honor_labels=false(ServiceMonitor/PodMonitor 기본)면 노출된 쪽이 밀려
// exported_pod 가 되고, pod 은 "exporter 의 Pod 이름"이 된다. 즉 pod=~"ses-..." 는 아무것도 못 잡는다.
// honor_labels=true 로 스크레이프하면 반대로 pod 이 워크로드가 된다.
//
// 운영에서 exporter 의 ServiceMonitor 는 NVIDIA GPU Operator 소유라 우리가 어느 쪽인지 강제할 수 없다.
// 그래서 두 배치를 모두 받는다. 선택자가 세션 Pod 이름(ses-*)이라 두 항이 동시에 매칭될 일은 없고
//   (exporter Pod 이름은 ses-* 형태가 아니다) or 로 이어도 중복 합산되지 않는다.

// DCGMPodSeries는 metric 을 워크로드 Pod 기준으로 고르고, 결과 라벨을 pod 으로 정규화한 식을 만든다.
// podRegex 는 앵커되는 PromQL 정규식(예: "ses-a|ses-b"). 집계는 호출부에서 `by(pod)` 로 한다.
func DCGMPodSeries(metric, podRegex string) string {
	return fmt.Sprintf(`(label_replace(%s{exported_pod=~"%s"}, "pod", "$1", "exported_pod", "(.*)") or %s{pod=~"%s"})`,
		metric, podRegex, metric, podRegex)
}

// DCGMPodScalar는 라벨 정규화 없이 값만 필요할 때(스칼라 집계) 쓰는 형태.
// 바깥에서 avg()/sum() 으로 접으므로 pod 라벨을 살릴 필요가 없다. inner 는 %s 자리에 셀렉터가
// 들어간 표현식 템플릿이다(예: `avg_over_time(DCGM_FI_DEV_GPU_UTIL{%s}[5m])`).
func DCGMPodScalar(innerTmpl, podMatcher string) string {
	return fmt.Sprintf("(%s or %s)",
		fmt.Sprintf(innerTmpl, "exported_pod="+podMatcher),
		fmt.Sprintf(innerTmpl, "pod="+podMatcher))
}
