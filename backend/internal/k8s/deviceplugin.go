package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DevicePluginConfigLabel은 NVIDIA GPU Operator 가 노드별 device plugin 설정을 고르는 라벨.
// 이 라벨을 바꾸면 operator 가 해당 노드의 device plugin 을 재설정한다(= 슬롯 광고 즉시 반영).
const DevicePluginConfigLabel = "nvidia.com/device-plugin.config"

// TimeSlicingKey는 슬롯 수별 device plugin 설정 프로파일 키(ConfigMap 의 data 키).
func TimeSlicingKey(replicas int) string { return fmt.Sprintf("giosk-ts-%d", replicas) }

// timeSlicingYAML은 GPU 1개를 replicas 개로 광고하는 device plugin 설정.
func timeSlicingYAML(replicas int) string {
	return fmt.Sprintf(`version: v1
flags:
  migStrategy: none
sharing:
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      replicas: %d
`, replicas)
}

// EnsureTimeSlicingKey는 device plugin 설정 ConfigMap 에 타임슬라이싱 프로파일을 upsert 한다(멱등).
// ConfigMap 이 없으면 만든다. 노드에 DevicePluginConfigLabel=<key> 를 붙이면 그 프로파일이 적용된다.
func (c *Client) EnsureTimeSlicingKey(ctx context.Context, ns, name string, replicas int) (string, error) {
	if !c.Available() {
		return "", ErrNoCluster
	}
	key := TimeSlicingKey(replicas)
	cms := c.cs.CoreV1().ConfigMaps(ns)
	cm, err := cms.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = cms.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"managed-by": "giosk-system"}},
			Data:       map[string]string{key: timeSlicingYAML(replicas)},
		}, metav1.CreateOptions{})
		return key, err
	}
	if err != nil {
		return "", err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	if cm.Data[key] == timeSlicingYAML(replicas) {
		return key, nil // 이미 같은 값이다. 불필요한 업데이트로 plugin 재시작을 유발하지 않는다
	}
	cm.Data[key] = timeSlicingYAML(replicas)
	_, err = cms.Update(ctx, cm, metav1.UpdateOptions{})
	return key, err
}
