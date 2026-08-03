package group

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("not found")

// Repository는 group 클러스터(groups/memberships/group_join_requests) 영속성 계약.
type Repository interface {
	ListAll() ([]Summary, error)
	Create(g *Group) error
	Update(id int64, fields map[string]any) error // 표시명·가입수락 등 수정
	Archive(id int64) error                       // 소프트 삭제(status='archived')
	CancelJoinRequest(userID, reqID int64) error  // 본인 대기중 가입신청 취소
	Find(id int64) (*Group, error)
	ActiveMemberCount(groupID int64) int // 활성 멤버 수(삭제 가드용)
	RoleInGroup(userID, groupID int64) (string, bool)

	ListMembers(groupID int64) ([]Member, error)
	UpsertMembership(groupID, userID int64, role, status string) error
	RemoveMember(groupID, userID int64) error
	MoveMember(fromGroupID, toGroupID, userID int64, role string) error // 원자적 그룹 이동
	SetMemberRole(groupID, userID int64, role string) error
	SetMemberBudget(groupID, userID int64, budget int) error
	Usage(groupID int64) ([]UsageRow, error)
	UsageTrend(groupID int64, days int) []UsageTrendPoint // 일자별 GPU 사용시간 추이
	UsageBySession(groupID int64) []SessionUsage          // 세션별 GPU 사용시간

	GetAcceptsJoin(groupID int64) (bool, error)
	SetAcceptsJoin(groupID int64, v bool) error

	ListJoinRequests(groupID int64, status string) ([]JoinRequest, error)
	ApproveJoin(reqID int64) error
	RejectJoin(reqID int64) error

	PrimaryMembership(userID int64) (*PrimaryMembership, error)
	ManagerMemberships(userID int64) ([]PrimaryMembership, error)
	ManagerRoleBadges(userIDs []int64) (map[int64][]RoleBadge, error)
	MyGroups(userID int64) ([]GroupRef, error)
	Directory(userID int64) ([]DirItem, error)
	MyJoinRequests(userID int64) ([]MyJoinRequest, error)
	CreateJoinRequest(userID, groupID int64) error
	ResolveUserID(account string) (int64, error)
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }
