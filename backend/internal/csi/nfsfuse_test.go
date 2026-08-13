package csi

import "testing"

// 서버나 경로가 없으면 FUSE 볼륨이 아니다. 이 판정이 곧 볼륨 타입 분기라, 잘못되면
// 로컬 이미지 볼륨을 NFS 로 붙이려 들거나 그 반대가 된다.
func TestNFSFuseFrom(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		ok    bool
	}{
		{"서버와 경로가 있으면 FUSE 볼륨", map[string]string{
			ParamNFSServer: "10.0.0.1", ParamNFSPath: "/export/vol",
		}, true},
		{"서버만 있으면 아님", map[string]string{ParamNFSServer: "10.0.0.1"}, false},
		{"경로만 있으면 아님", map[string]string{ParamNFSPath: "/export/vol"}, false},
		{"둘 다 없으면 아님(로컬 이미지 볼륨)", map[string]string{ParamSize: "1024"}, false},
		{"nil 이어도 죽지 않는다", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := nfsFuseFrom(tc.attrs, false)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
		})
	}
}

// 속성 캐시는 기본값이 있어야 한다. 0 으로 떨어지면 메타데이터 연산이 매번 하부로 내려가
// 파일이 많은 데이터셋에서 수십 배 느려진다(실측 4,105ms 대 62ms).
func TestNFSFuseAttrTimeoutDefaults(t *testing.T) {
	base := map[string]string{ParamNFSServer: "s", ParamNFSPath: "/p"}

	spec, _ := nfsFuseFrom(base, false)
	if spec.AttrTimeut != defaultAttrTimeoutSec {
		t.Errorf("미지정 시 기본값이어야 한다: %d", spec.AttrTimeut)
	}

	base[ParamAttrTimeout] = "이상한값"
	spec, _ = nfsFuseFrom(base, false)
	if spec.AttrTimeut != defaultAttrTimeoutSec {
		t.Errorf("파싱 실패 시 기본값이어야 한다: %d", spec.AttrTimeut)
	}

	// 0 은 "캐시 끔"이라는 유효한 선택이므로 기본값으로 덮으면 안 된다.
	base[ParamAttrTimeout] = "0"
	spec, _ = nfsFuseFrom(base, false)
	if spec.AttrTimeut != 0 {
		t.Errorf("명시한 0 이 기본값으로 덮였다: %d", spec.AttrTimeut)
	}

	base[ParamAttrTimeout] = "60"
	spec, _ = nfsFuseFrom(base, true)
	if spec.AttrTimeut != 60 {
		t.Errorf("지정값이 반영되지 않았다: %d", spec.AttrTimeut)
	}
	if !spec.ReadOnly {
		t.Error("읽기전용 플래그가 전달되지 않았다")
	}
}
