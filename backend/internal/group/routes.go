package group

import "github.com/gin-gonic/gin"

// RegisterAdmin은 플랫폼 관리자 그룹 라우트(admin 그룹에 등록).
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/groups", h.ListAll)
	admin.POST("/groups", h.Create)
	admin.PUT("/groups/:id", h.Update)    // 표시명·가입수락 수정
	admin.DELETE("/groups/:id", h.Delete) // 소프트 삭제
	// 그룹 이동은 대상 그룹까지 지정하므로 플랫폼 관리자 전용(그룹 관리자는 자기 그룹 밖을 모른다).
	admin.PUT("/groups/:id/members/:userId/move", h.MoveMember)
}

// RegisterUser는 인증 사용자용(소속/디렉터리/가입신청). authed 그룹에 등록.
func RegisterUser(authed gin.IRouter, h *Handler) {
	authed.GET("/me/groups", h.MyGroups)
	authed.GET("/me/membership-context", h.MembershipContext)
	authed.GET("/me/share-targets", h.ShareTargets)
	authed.GET("/me/group-directory", h.Directory)
	authed.POST("/me/join-requests", h.CreateJoinRequest)
	authed.GET("/me/join-requests", h.MyJoinRequests)
	authed.DELETE("/me/join-requests/:reqId", h.CancelJoinRequest) // 본인 가입신청 취소
}

// RegisterGroupScoped은 그룹 관리자(project_admin) 라우트.
// projAdmin 미들웨어는 호출측(server)에서 GroupRole 로 주입.
func RegisterGroupScoped(authed gin.IRouter, h *Handler, projAdmin gin.HandlerFunc) {
	g := authed.Group("/groups/:id", projAdmin)
	g.GET("/members", h.Members)
	g.POST("/members", h.AddMember)
	g.PUT("/members/:userId/role", h.SetMemberRole)
	g.PUT("/members/:userId/budget", h.SetMemberBudget)
	g.DELETE("/members/:userId", h.RemoveMember)
	g.GET("/usage", h.Usage)
	g.GET("/join-policy", h.GetJoinPolicy)
	g.PUT("/join-policy", h.SetJoinPolicy)
	g.GET("/join-requests", h.JoinRequests)
	g.POST("/join-requests/:reqId/approve", h.ApproveJoin)
	g.POST("/join-requests/:reqId/reject", h.RejectJoin)
}
