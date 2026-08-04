package volume

import (
	"errors"
	"strconv"
	"strings"

	"giosk/internal/auth"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 volume HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) List(c *gin.Context) {
	res, err := h.svc.List(auth.CurrentUser(c).ID)
	if err != nil {
		httpx.Internal(c, "볼륨 조회 실패")
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name, sizeGib 필요")
		return
	}
	v, err := h.svc.Create(c.Request.Context(), auth.CurrentUser(c).ID, scopeGroup(c), req)
	if errors.Is(err, ErrQuotaExceeded) {
		httpx.Err(c, 409, "quota_exceeded", "볼륨 쿼터를 초과했습니다")
		return
	}
	if errors.Is(err, ErrInsufficientCredit) {
		httpx.Err(c, 402, "insufficient_credit", "크레딧이 부족합니다(스토리지 최소 비용 이상 필요)")
		return
	}
	if err != nil {
		httpx.Internal(c, "볼륨 생성 실패")
		return
	}
	httpx.Created(c, v)
}

func (h *Handler) Share(c *gin.Context) {
	var req ShareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	if err := h.svc.Share(idParam(c), req); err != nil {
		httpx.Internal(c, "공유 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	err := h.svc.Delete(c.Request.Context(), idParam(c), auth.CurrentUser(c).ID)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(c, "볼륨을 찾을 수 없습니다")
		return
	}
	if err != nil {
		httpx.Internal(c, "볼륨 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// AdminStorage는 관리자 스토리지 현황(노드 디스크/NFS/사용자별 할당).
func (h *Handler) AdminStorage(c *gin.Context) {
	httpx.OK(c, h.svc.AdminStorage(c.Request.Context()))
}

// AdminList는 전체 볼륨 목록(플랫폼). 스토리지 요약도 함께 반환한다.
func (h *Handler) AdminList(c *gin.Context) {
	items, err := h.svc.repo.ListAll(0, 0)
	if err != nil {
		httpx.Internal(c, "볼륨 조회 실패")
		return
	}
	if items == nil {
		items = []AdminVolume{}
	}
	httpx.OK(c, gin.H{"items": items, "storage": h.svc.AdminStorage(c.Request.Context())})
}

// ListScoped는 매니저 스코프 볼륨 목록(/console). 열람 전용(임포트는 플랫폼).
func (h *Handler) ListScoped(c *gin.Context) {
	orgID, groupID := scopeFn(c)
	items, err := h.svc.repo.ListAll(orgID, groupID)
	if err != nil {
		httpx.Internal(c, "볼륨 조회 실패")
		return
	}
	if items == nil {
		items = []AdminVolume{}
	}
	httpx.OK(c, gin.H{"items": items})
}

// scopeFn은 핸들러 주입식 스코프 해석(server 와이어링에서 세팅). 미설정이면 (0,0)=전체.
var scopeFn = func(c *gin.Context) (int64, int64) { return 0, 0 }

// SetScopeFn은 authz 스코프 해석기를 주입한다(순환참조 피하려 함수 주입).
func SetScopeFn(fn func(c *gin.Context) (int64, int64)) { scopeFn = fn }

// AdminImport는 기존 NFS 경로를 볼륨으로 매핑한다(도입 환경 어댑션).
func (h *Handler) AdminImport(c *gin.Context) {
	var req ImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "name, nfsServer, nfsPath 필요")
		return
	}
	v, err := h.svc.AdminImport(c.Request.Context(), req)
	if err != nil {
		httpx.BadRequest(c, err.Error())
		return
	}
	httpx.Created(c, v)
}

// RegisterUser는 인증 사용자용 볼륨 라우트.
func (h *Handler) Register(authed gin.IRouter) {
	authed.GET("/volumes", h.List)
	authed.POST("/volumes", h.Create)
	authed.POST("/volumes/:id/share", h.Share)
	authed.PUT("/volumes/:id/team", h.SetTeam)
	authed.DELETE("/volumes/:id", h.Delete)
}

// SetTeam은 볼륨의 귀속 팀을 바꾼다(소유자; 대상 팀 쿼터 확인).
func (h *Handler) SetTeam(c *gin.Context) {
	var req struct {
		GroupID int64 `json:"groupId"`
	}
	_ = c.ShouldBindJSON(&req)
	err := h.svc.SetVolumeTeam(auth.CurrentUser(c).ID, idParam(c), req.GroupID)
	if errors.Is(err, ErrQuotaExceeded) {
		httpx.Err(c, 409, "quota_exceeded", "대상 팀의 볼륨 쿼터를 초과합니다")
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(c, "볼륨을 찾을 수 없습니다")
		return
	}
	if err != nil {
		httpx.Internal(c, "귀속 팀 변경 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// RegisterAdmin은 플랫폼 관리자 스토리지 현황 라우트.
func (h *Handler) RegisterAdmin(admin gin.IRouter) {
	admin.GET("/storage", h.AdminStorage)
	admin.GET("/volumes", h.AdminList)
	admin.POST("/volumes", h.AdminImport)
}

// RegisterScoped는 매니저 볼륨 열람(/console)이다. 임포트는 플랫폼에 남긴다.
func (h *Handler) RegisterScoped(mgmt gin.IRouter) {
	mgmt.GET("/volumes", h.ListScoped)
}

func idParam(c *gin.Context) int64 { id, _ := strconv.ParseInt(c.Param("id"), 10, 64); return id }

// scopeGroup은 X-Console-Scope 헤더가 group:N 이면 N(활성 팀), 아니면 0(팀 미지정)을 반환한다.
func scopeGroup(c *gin.Context) int64 {
	if sel := c.GetHeader("X-Console-Scope"); strings.HasPrefix(sel, "group:") {
		id, _ := strconv.ParseInt(sel[len("group:"):], 10, 64)
		return id
	}
	return 0
}
