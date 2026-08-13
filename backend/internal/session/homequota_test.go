package session

import (
	"errors"
	"testing"
)

// homeQuotaRepo는 checkHomeQuota 가 쓰는 부분만 흉내낸다.
type homeQuotaRepo struct {
	Repository // 나머지 메서드는 호출되지 않는다(호출되면 nil 패닉으로 드러난다)
	used int
}

func (r homeQuotaRepo) AllocatedHomeGiB(int64, int) int { return r.used }

// svcWithHome은 홈 기본값·기존 사용량·볼륨 사용량을 지정한 서비스를 만든다.
func svcWithHome(defaultGiB, homeUsed, volUsed int) *Service {
	return &Service{
		repo:           homeQuotaRepo{used: homeUsed},
		sessionHomeGiB: defaultGiB,
		volumeUsedFn:   func(int64) int { return volUsed },
	}
}

func TestHomeQuota(t *testing.T) {
	sizeOf := func(n int) *int { return &n }

	tests := []struct {
		name       string
		defaultGiB int
		homeUsed   int
		volUsed    int
		quota      int
		sess       *Session
		wantErr    bool
	}{
		{
			name: "홈과 볼륨 합이 쿼터 안이면 통과",
			// 기존 홈 100 + 볼륨 200 + 새 홈 50 = 350 <= 500
			defaultGiB: 50, homeUsed: 100, volUsed: 200, quota: 500,
			sess: &Session{Env: "container", HomeGiB: sizeOf(50)},
		},
		{
			name: "볼륨까지 더해 넘으면 거부",
			// 홈 100 + 볼륨 200 + 새 홈 250 = 550 > 500
			defaultGiB: 50, homeUsed: 100, volUsed: 200, quota: 500,
			sess:    &Session{Env: "container", HomeGiB: sizeOf(250)},
			wantErr: true,
		},
		{
			name: "정확히 쿼터에 맞으면 통과(경계)",
			// 홈 100 + 볼륨 200 + 새 홈 200 = 500 == 500
			defaultGiB: 50, homeUsed: 100, volUsed: 200, quota: 500,
			sess: &Session{Env: "container", HomeGiB: sizeOf(200)},
		},
		{
			name: "홈 크기 미지정이면 설치 기본값으로 센다",
			// 홈 0 + 볼륨 0 + 기본 300 = 300 > 200 이므로 거부돼야 한다
			defaultGiB: 300, homeUsed: 0, volUsed: 0, quota: 200,
			sess:    &Session{Env: "container"},
			wantErr: true,
		},
		{
			name: "물리(SSH) 임대는 홈 개념이 없어 제외",
			// 노드를 통째로 빌리는 것이라 쿼터를 아무리 넘겨도 이 검사는 통과해야 한다
			defaultGiB: 50, homeUsed: 9999, volUsed: 9999, quota: 1,
			sess: &Session{Env: "ssh", HomeGiB: sizeOf(9999)},
		},
		{
			name: "쿼터 0(미설정)이면 제한하지 않는다",
			// 다른 하드리밋과 같은 규칙이다
			defaultGiB: 50, homeUsed: 9999, volUsed: 9999, quota: 0,
			sess: &Session{Env: "container", HomeGiB: sizeOf(9999)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := svcWithHome(tc.defaultGiB, tc.homeUsed, tc.volUsed)
			err := s.checkHomeQuota(1, tc.sess, tc.quota)
			if tc.wantErr && !errors.Is(err, ErrHomeQuota) {
				t.Fatalf("거부돼야 하는데 통과했다: %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("통과해야 하는데 거부됐다: %v", err)
			}
		})
	}
}

// 볼륨 사용량 조회가 주입되지 않은 배포에서도 홈만으로 판정이 되어야 한다.
// 주입 실패로 nil 이 되었을 때 패닉하거나 무제한이 되면 안 된다.
func TestHomeQuotaWithoutVolumeUsage(t *testing.T) {
	s := &Service{repo: homeQuotaRepo{used: 90}, sessionHomeGiB: 20}
	n := 20
	if err := s.checkHomeQuota(1, &Session{Env: "container", HomeGiB: &n}, 100); !errors.Is(err, ErrHomeQuota) {
		t.Fatalf("홈 90+20 이 쿼터 100 을 넘는데 통과했다: %v", err)
	}
	if err := s.checkHomeQuota(1, &Session{Env: "container", HomeGiB: &n}, 200); err != nil {
		t.Fatalf("홈 90+20 이 쿼터 200 안인데 거부됐다: %v", err)
	}
}
