package k8s

import "errors"

var (
	// ErrNoCluster — K8s 클러스터 미가용(상위에서 503 처리).
	ErrNoCluster = errors.New("k8s cluster unavailable")
	// ErrNotFound — 리소스 없음.
	ErrNotFound = errors.New("k8s resource not found")
)
