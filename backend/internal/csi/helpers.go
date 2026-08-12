package csi

import "strconv"

// ParamSize는 볼륨 크기(바이트)를 VolumeContext 로 넘길 때 쓰는 키.
//
// NodePublishVolume 은 요청에 용량을 담아 주지 않는다(CSI 규격상 크기는 CreateVolume 의
// 관심사다). 그런데 우리는 노드에서 이미지를 만들어야 해서 크기를 알아야 하므로,
// CreateVolume 이 VolumeContext 에 실어 보내고 노드가 그걸 읽는다.
const ParamSize = DriverName + "/size-bytes"

// volumeSize는 VolumeContext 에서 볼륨 크기를 읽는다(없거나 이상하면 0).
func volumeSize(ctx map[string]string) int64 {
	n, err := strconv.ParseInt(ctx[ParamSize], 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// atoiDefault는 정수 문자열을 파싱하되 실패하면 기본값을 쓴다.
func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
