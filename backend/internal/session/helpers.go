package session

import (
	"fmt"

	"giosk/internal/k8s"
)

// labels는 세션 Pod 식별 라벨을 만든다.
func (s *Session) labels() map[string]string {
	l := map[string]string{
		"managed-by":       "giosk-system",
		"giosk.io/session": s.InstanceID,
		"giosk.io/user":    fmt.Sprintf("%d", s.UserID),
	}
	if s.GroupID != nil {
		l["giosk.io/group"] = fmt.Sprintf("%d", *s.GroupID)
	}
	return l
}

// mapOfferingMode는 오퍼링 mode(fractional/exclusive/mig)를 세션 gpuMode 로 변환한다.
func mapOfferingMode(mode string) string {
	if mode == "fractional" {
		return k8s.GpuModeShared
	}
	if mode == "" {
		return k8s.GpuModeCPU
	}
	return k8s.GpuModeExclusive // exclusive | mig
}

// mapPodPhase는 corev1 Pod phase 를 세션 phase 로 변환한다.
func mapPodPhase(podPhase string) string {
	switch podPhase {
	case "Running":
		return PhaseRunning
	case "Pending", "":
		return PhaseProvisioning
	case "Succeeded", "Failed":
		return PhaseStopped
	default:
		return PhaseProvisioning
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
