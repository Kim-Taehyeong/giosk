// Package policy는 하드 리소스 제한(크레딧과 무관한 절대 상한)을 계층 규칙으로 해석한다.
//
// 정준 모델(설계 문서): 하드 정책 = 1차(절대 벽, 모든 과금 모드 공통). 크레딧 정책 = 2차(크레딧 모드만).
// 해석은 바텀업 오버라이드: 개인 → 그룹 → 조직 → 전역. 각 레벨의 값이 지정(non-nil)돼 있으면
// 가장 가까운 것을 채택하고, 없으면 상위로 폴백한다. 전역(config.Quota)이 최종 폴백이라 결과는 항상 구체값.
package policy

// Limits는 한 레벨(개인/그룹/조직)의 하드 제한. nil = 그 레벨은 미지정(상위로 폴백).
type Limits struct {
	MaxGpu                *int
	MaxVramGB             *int
	MaxVolumeGiB          *int
	MaxConcurrentSessions *int
	MaxStoppedSessions    *int
	MaxEphemeralGiB       *int
}

// Resolved는 해석 완료된 구체 상한(0 = 무제한).
type Resolved struct {
	MaxGpu                int
	MaxVramGB             int
	MaxVolumeGiB          int
	MaxConcurrentSessions int
	MaxStoppedSessions    int
	MaxEphemeralGiB       int
}

// Level은 하드 제한을 설정할 계층(관리자 API 대상).
type Level string

const (
	LevelUser  Level = "user"
	LevelGroup Level = "group"
	LevelOrg   Level = "org"
)

// PolicyRow는 "정책" 화면의 한 줄 — 어떤 범위(scope)의 누구에게 어떤 상한이 걸려 있는지.
// 전 범위를 한 목록으로 보여주기 위해 users/groups/organizations 를 UNION 해서 만든다.
// PolicyRow는 정책 목록의 한 행이다.
// gorm:"column:" 태그 필수 — 기본 네이밍은 MaxVolumeGiB 를 max_volume_gi_b 로 바꿔 버려
// 실제 컬럼 max_volume_gib 와 어긋나고, 값이 조용히 nil 로 떨어진다(저장은 됐는데 안 보임).
type PolicyRow struct {
	Scope                 Level  `json:"scope" gorm:"column:scope"` // user | group | org
	ID                    int64  `json:"id" gorm:"column:id"`
	Name                  string `json:"name" gorm:"column:name"`
	MaxGpu                *int   `json:"maxGpu" gorm:"column:max_gpu"`
	MaxVramGB             *int   `json:"maxVramGb" gorm:"column:max_vram_gb"`
	MaxVolumeGiB          *int   `json:"maxVolumeGib" gorm:"column:max_volume_gib"`
	MaxConcurrentSessions *int   `json:"maxConcurrentSessions" gorm:"column:max_concurrent_sessions"`
	MaxStoppedSessions    *int   `json:"maxStoppedSessions" gorm:"column:max_stopped_sessions"`
	MaxEphemeralGiB       *int   `json:"maxEphemeralGib" gorm:"column:max_ephemeral_gib"`
}

// Repository는 각 계층의 제한값을 조회/설정한다.
type Repository interface {
	UserLimits(userID int64) (Limits, error)
	GroupLimits(groupID int64) (Limits, error)
	OrgLimits(orgID int64) (Limits, error)
	SetLimits(level Level, id int64, l Limits) error              // nil 필드 = NULL(그 레벨 미지정 → 상위 폴백)
	ListPolicies() ([]PolicyRow, error)                           // 상한이 하나라도 설정된 전 범위 목록
	ListPoliciesScoped(orgID, groupID int64) ([]PolicyRow, error) // 스코프 내(org/group)만
}

// Hierarchy는 사용자의 대표 그룹/조직 + 그룹의 조직을 해석한다(org.Service 가 구현).
type Hierarchy interface {
	GroupOfUser(userID int64) (int64, bool)
	OrgOfUser(userID int64) (int64, bool)
	OrgOfGroup(groupID int64) (int64, bool)
}

// ResolveGroupByID는 그룹 id 만으로 그 그룹의 유효 상한을 해석한다(전역←조직←그룹). 볼륨 팀 귀속 쿼터용.
func (r *Resolver) ResolveGroupByID(groupID int64) Resolved {
	orgID, _ := r.hier.OrgOfGroup(groupID)
	return r.ResolveGroup(orgID, groupID)
}

// Resolver는 계층을 걸어 하드 상한을 산출한다.
type Resolver struct {
	repo     Repository
	hier     Hierarchy
	globalFn func() Resolved // 전역 폴백(설치 env + 런타임 오버라이드) — 호출 시점 값을 읽는다
}

// Global은 현재 전역 상한을 반환한다(호출 시점 기준 — 런타임 편집이 바로 반영된다).
func (r *Resolver) Global() Resolved {
	if r.globalFn == nil {
		return Resolved{}
	}
	return r.globalFn()
}

// globalFn 은 호출할 때마다 현재 전역 상한을 돌려준다(런타임 편집 즉시 반영).
func NewResolver(repo Repository, hier Hierarchy, globalFn func() Resolved) *Resolver {
	return &Resolver{repo: repo, hier: hier, globalFn: globalFn}
}

// Resolve는 개인→그룹→조직→전역 순으로 각 키의 첫 non-nil 값을 채택해 구체 상한을 만든다.
func (r *Resolver) Resolve(userID int64) Resolved {
	// 상위→하위 순으로 레이어를 쌓고(전역이 바닥), 하위가 있으면 덮어쓴다 → 결과적으로 하위 우선.
	layers := []Limits{r.globalAsLimits()}
	if orgID, ok := r.hier.OrgOfUser(userID); ok {
		if l, err := r.repo.OrgLimits(orgID); err == nil {
			layers = append(layers, l)
		}
	}
	if groupID, ok := r.hier.GroupOfUser(userID); ok {
		if l, err := r.repo.GroupLimits(groupID); err == nil {
			layers = append(layers, l)
		}
	}
	if l, err := r.repo.UserLimits(userID); err == nil {
		layers = append(layers, l)
	}

	out := r.Global()
	for _, l := range layers {
		if l.MaxGpu != nil {
			out.MaxGpu = *l.MaxGpu
		}
		if l.MaxVramGB != nil {
			out.MaxVramGB = *l.MaxVramGB
		}
		if l.MaxVolumeGiB != nil {
			out.MaxVolumeGiB = *l.MaxVolumeGiB
		}
		if l.MaxConcurrentSessions != nil {
			out.MaxConcurrentSessions = *l.MaxConcurrentSessions
		}
		if l.MaxStoppedSessions != nil {
			out.MaxStoppedSessions = *l.MaxStoppedSessions
		}
		if l.MaxEphemeralGiB != nil {
			out.MaxEphemeralGiB = *l.MaxEphemeralGiB
		}
	}
	return out
}

// ResolveOrg는 조직 레벨 유효 상한(전역 ← 조직 오버레이). 그룹 상한의 부모 제한.
func (r *Resolver) ResolveOrg(orgID int64) Resolved {
	out := r.Global()
	if l, err := r.repo.OrgLimits(orgID); err == nil {
		applyLimits(&out, l)
	}
	return out
}

// ResolveGroup은 그룹 레벨 유효 상한(전역 ← 조직 ← 그룹). 사용자 상한의 부모 제한.
func (r *Resolver) ResolveGroup(orgID, groupID int64) Resolved {
	out := r.ResolveOrg(orgID)
	if l, err := r.repo.GroupLimits(groupID); err == nil {
		applyLimits(&out, l)
	}
	return out
}

// applyLimits는 지정된(non-nil) 필드만 out 에 덮어쓴다.
func applyLimits(out *Resolved, l Limits) {
	if l.MaxGpu != nil {
		out.MaxGpu = *l.MaxGpu
	}
	if l.MaxVramGB != nil {
		out.MaxVramGB = *l.MaxVramGB
	}
	if l.MaxVolumeGiB != nil {
		out.MaxVolumeGiB = *l.MaxVolumeGiB
	}
	if l.MaxConcurrentSessions != nil {
		out.MaxConcurrentSessions = *l.MaxConcurrentSessions
	}
	if l.MaxStoppedSessions != nil {
		out.MaxStoppedSessions = *l.MaxStoppedSessions
	}
	if l.MaxEphemeralGiB != nil {
		out.MaxEphemeralGiB = *l.MaxEphemeralGiB
	}
}

// ParentOfUser는 사용자 상한의 부모 유효 제한(전역←조직←그룹, 사용자 자신 제외).
func (r *Resolver) ParentOfUser(userID int64) Resolved {
	out := r.Global()
	if orgID, ok := r.hier.OrgOfUser(userID); ok {
		if l, err := r.repo.OrgLimits(orgID); err == nil {
			applyLimits(&out, l)
		}
	}
	if groupID, ok := r.hier.GroupOfUser(userID); ok {
		if l, err := r.repo.GroupLimits(groupID); err == nil {
			applyLimits(&out, l)
		}
	}
	return out
}

// HierOfUser는 사용자의 대표 조직/그룹 id(스코프 판별용).
func (r *Resolver) HierOfUser(userID int64) (orgID, groupID int64) {
	orgID, _ = r.hier.OrgOfUser(userID)
	groupID, _ = r.hier.GroupOfUser(userID)
	return
}

func (r *Resolver) globalAsLimits() Limits {
	g := r.Global()
	return Limits{MaxGpu: &g.MaxGpu, MaxVramGB: &g.MaxVramGB, MaxVolumeGiB: &g.MaxVolumeGiB, MaxConcurrentSessions: &g.MaxConcurrentSessions, MaxStoppedSessions: &g.MaxStoppedSessions, MaxEphemeralGiB: &g.MaxEphemeralGiB}
}
