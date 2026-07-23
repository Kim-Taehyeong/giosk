// Package userdetail은 관리자 "사용자 360" 상세를 한 번에 모아 준다.
// 각 도메인(볼륨/세션/데이터셋/가입요청/지갑/사용량)의 기존 userID-키 서비스 메서드를
// 클로저로 주입받아(모듈 직접 import 없음 → 순환 참조 없음) 합쳐 반환한다.
package userdetail

import (
	"context"
	"strconv"

	"giosk/internal/authz"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Providers는 대상 사용자 id 로 각 도메인 데이터를 가져오는 주입 함수들.
// 개별 함수가 nil 이면 해당 섹션은 생략(응답에서 null).
type Providers struct {
	User     func(id int64) (any, error)                      // 기본 프로필(없으면 nil,nil → 404)
	Wallet   func(id int64) (any, error)                      // 지갑(잔액/내역/세션별 지출)
	Volumes  func(id int64) (any, error)                      // 소유/공유 볼륨
	Sessions func(ctx context.Context, id int64) (any, error) // 세션 목록
	Datasets func(ctx context.Context, id int64) (any, error) // 데이터셋/요청
	JoinReqs func(id int64) (any, error)                      // 그룹 가입 요청
	Usage    func(id int64) any                               // GPU 사용량(시간/소비) 롤업
	UserHier func(id int64) (orgID, groupID int64)            // 대상의 조직/그룹(스코프 검증용)
	Members  func(id int64) (any, error)                      // 이 사용자의 전체 소속(조직/팀/역할) — 다중 소속 평면 나열
}

type Handler struct{ p Providers }

func NewHandler(p Providers) *Handler { return &Handler{p: p} }

// RegisterAdmin은 플랫폼 관리자 사용자 상세 라우트.
func RegisterAdmin(admin gin.IRouter, h *Handler) {
	admin.GET("/users/:id/detail", h.Detail)
}

// RegisterScoped은 매니저(조직/그룹 관리자)용 스코프 검증 상세 라우트(/console).
func RegisterScoped(mgmt gin.IRouter, h *Handler) {
	mgmt.GET("/users/:id/detail", h.DetailScoped)
}

// Detail은 플랫폼 관리자용(스코프 무관).
func (h *Handler) Detail(c *gin.Context) { h.serve(c, false) }

// DetailScoped은 매니저용 — 대상 사용자가 내 스코프(조직/그룹)에 속할 때만.
func (h *Handler) DetailScoped(c *gin.Context) { h.serve(c, true) }

func (h *Handler) serve(c *gin.Context, scoped bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.BadRequest(c, "invalid user id")
		return
	}
	ctx := c.Request.Context()

	// 매니저는 자기 범위 밖 사용자 상세를 못 본다(조직관리자=자기 조직, 그룹관리자=자기 그룹).
	if scoped {
		s := authz.CurrentScope(c)
		if s.Level != "platform" && h.p.UserHier != nil {
			orgID, groupID := h.p.UserHier(id)
			if (s.Level == "org" && orgID != s.OrgID) || (s.Level == "group" && groupID != s.GroupID) {
				httpx.Forbidden(c, "권한 범위를 벗어난 사용자입니다")
				return
			}
		}
	}

	u, err := h.p.User(id)
	if err != nil {
		httpx.Internal(c, "사용자 조회 실패")
		return
	}
	if u == nil {
		httpx.NotFound(c, "사용자를 찾을 수 없습니다")
		return
	}

	out := gin.H{"user": u}
	// 섹션별 조회 실패는 치명적이지 않다 — 해당 섹션만 비우고 나머지는 채운다.
	if h.p.Wallet != nil {
		if v, e := h.p.Wallet(id); e == nil {
			out["wallet"] = v
		}
	}
	if h.p.Volumes != nil {
		if v, e := h.p.Volumes(id); e == nil {
			out["volumes"] = v
		}
	}
	if h.p.Sessions != nil {
		if v, e := h.p.Sessions(ctx, id); e == nil {
			out["sessions"] = v
		}
	}
	if h.p.Datasets != nil {
		if v, e := h.p.Datasets(ctx, id); e == nil {
			out["datasets"] = v
		}
	}
	if h.p.JoinReqs != nil {
		if v, e := h.p.JoinReqs(id); e == nil {
			out["joinRequests"] = v
		}
	}
	if h.p.Usage != nil {
		out["usage"] = h.p.Usage(id)
	}
	if h.p.Members != nil {
		if v, e := h.p.Members(id); e == nil {
			out["memberships"] = v
		}
	}
	httpx.OK(c, out)
}
