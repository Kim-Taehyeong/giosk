package auth

import "github.com/gin-gonic/gin"

// ProfileScope는 사용자가 가진 관리 스코프 하나(컨텍스트 전환기용). 멀티롤 지원.
type ProfileScope struct {
	Level     string `json:"level"` // org | group
	OrgID     int64  `json:"orgId"`
	GroupID   int64  `json:"groupId,omitempty"`
	OrgName   string `json:"orgName,omitempty"`
	GroupName string `json:"groupName,omitempty"`
}

// ProfileExtra는 콘솔 라우팅에 필요한 멤버십 컨텍스트(프론트 consoleRoles 기준).
// MembershipRole, ConsoleLevel, OrgID, GroupID 는 기본(최우선) 스코프이며 하위 호환을 위해 남긴다.
// Scopes 는 사용자가 동시에 가진 모든 관리 스코프(멀티롤 전환기).
type ProfileExtra struct {
	MembershipRole string         `json:"membershipRole,omitempty"` // org_admin | group_admin | member
	ConsoleLevel   string         `json:"consoleLevel,omitempty"`   // platform | org | group | user
	OrgID          *int64         `json:"orgId,omitempty"`
	GroupID        *int64         `json:"groupId,omitempty"`
	OrgName        string         `json:"orgName,omitempty"`
	GroupName      string         `json:"groupName,omitempty"`
	Scopes         []ProfileScope `json:"scopes,omitempty"`
}

// Enricher는 사용자 멤버십 컨텍스트를 조회한다(group.Service 가 구현, main 에서 주입).
type Enricher interface {
	Profile(userID int64) ProfileExtra
}

// userView는 로그인/me 응답용 사용자 객체(프론트 user 와 호환).
func userView(u *User, e ProfileExtra) gin.H {
	v := gin.H{
		"id": u.ID, "username": u.Username, "email": u.Email,
		"firstName": u.FirstName, "lastName": u.LastName,
		"role": u.Role, "status": u.Status,
	}
	// SSH 공개키는 비밀이 아니다. 내정보와 접속 화면이 등록 여부를 판단하고 그대로 보여준다.
	addIf(v, "sshPublicKey", u.SSHPublicKey)
	if u.Role == RoleAdmin {
		v["consoleLevel"] = "platform"
	} else if e.ConsoleLevel != "" {
		v["consoleLevel"] = e.ConsoleLevel
	}
	addIf(v, "membershipRole", e.MembershipRole)
	addIf(v, "orgName", e.OrgName)
	addIf(v, "groupName", e.GroupName)
	if e.OrgID != nil {
		v["orgId"] = *e.OrgID
	}
	if e.GroupID != nil {
		v["groupId"] = *e.GroupID
	}
	if len(e.Scopes) > 0 {
		v["scopes"] = e.Scopes
	}
	return v
}

func addIf(v gin.H, key, val string) {
	if val != "" {
		v[key] = val
	}
}
