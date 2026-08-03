package policy

import "testing"

func ptr(n int) *int { return &n }

// fakeRepo는 계층별 제한을 메모리로 제공한다.
type fakeRepo struct {
	user  Limits
	group map[int64]Limits
	org   map[int64]Limits
}

func (f fakeRepo) UserLimits(int64) (Limits, error)     { return f.user, nil }
func (f fakeRepo) GroupLimits(id int64) (Limits, error) { return f.group[id], nil }
func (f fakeRepo) OrgLimits(id int64) (Limits, error)   { return f.org[id], nil }
func (f fakeRepo) SetLimits(Level, int64, Limits) error { return nil }
func (f fakeRepo) ListPolicies() ([]PolicyRow, error)   { return nil, nil }
func (f fakeRepo) ListPoliciesScoped(int64, int64) ([]PolicyRow, error) { return nil, nil }

// fakeHier는 사용자 → 대표 그룹/조직 매핑.
type fakeHier struct {
	group  int64
	org    int64
	hasGrp bool
	hasOrg bool
}

func (h fakeHier) GroupOfUser(int64) (int64, bool)  { return h.group, h.hasGrp }
func (h fakeHier) OrgOfUser(int64) (int64, bool)    { return h.org, h.hasOrg }
func (h fakeHier) OrgOfGroup(int64) (int64, bool)   { return h.org, h.hasOrg }

var global = Resolved{MaxGpu: 64, MaxVramGB: 512, MaxVolumeGiB: 2000, MaxConcurrentSessions: 50}

func TestResolve_FallsBackToGlobal(t *testing.T) {
	r := NewResolver(fakeRepo{}, fakeHier{}, func() Resolved { return global })
	got := r.Resolve(1)
	if got != global {
		t.Fatalf("미지정이면 전역이어야 함: got %+v", got)
	}
}

func TestResolve_GroupOverridesGlobal(t *testing.T) {
	repo := fakeRepo{group: map[int64]Limits{7: {MaxGpu: ptr(2)}}}
	r := NewResolver(repo, fakeHier{group: 7, hasGrp: true}, func() Resolved { return global })
	got := r.Resolve(1)
	if got.MaxGpu != 2 {
		t.Fatalf("그룹 max_gpu=2 적용되어야 함: got %d", got.MaxGpu)
	}
	if got.MaxVramGB != 512 {
		t.Fatalf("미지정 항목은 전역 폴백: got %d", got.MaxVramGB)
	}
}

func TestResolve_UserOverridesGroup(t *testing.T) {
	repo := fakeRepo{
		user:  Limits{MaxGpu: ptr(1)},
		group: map[int64]Limits{7: {MaxGpu: ptr(2)}},
	}
	r := NewResolver(repo, fakeHier{group: 7, hasGrp: true}, func() Resolved { return global })
	if got := r.Resolve(1); got.MaxGpu != 1 {
		t.Fatalf("개인 오버라이드(1)가 그룹(2)보다 우선해야 함: got %d", got.MaxGpu)
	}
}

func TestResolve_OrgThenGroupThenUser(t *testing.T) {
	repo := fakeRepo{
		user:  Limits{MaxVolumeGiB: ptr(100)},
		group: map[int64]Limits{7: {MaxVramGB: ptr(128)}},
		org:   map[int64]Limits{3: {MaxGpu: ptr(8)}},
	}
	r := NewResolver(repo, fakeHier{group: 7, hasGrp: true, org: 3, hasOrg: true}, func() Resolved { return global })
	got := r.Resolve(1)
	// 각 항목은 그 항목을 지정한 가장 가까운 레벨에서 온다.
	if got.MaxVolumeGiB != 100 { // 개인
		t.Fatalf("개인 volume=100: got %d", got.MaxVolumeGiB)
	}
	if got.MaxVramGB != 128 { // 그룹
		t.Fatalf("그룹 vram=128: got %d", got.MaxVramGB)
	}
	if got.MaxGpu != 8 { // 조직
		t.Fatalf("조직 gpu=8: got %d", got.MaxGpu)
	}
	if got.MaxConcurrentSessions != 50 { // 전역
		t.Fatalf("전역 sessions=50: got %d", got.MaxConcurrentSessions)
	}
}
