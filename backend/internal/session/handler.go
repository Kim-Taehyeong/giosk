package session

import (
	"context"
	"errors"
	"log"
	"strconv"

	"giosk/internal/auth"
	"giosk/internal/k8s"
	"giosk/internal/node"
	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 session HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(c *gin.Context) {
	var req CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	u := auth.CurrentUser(c)
	sess, err := h.svc.Create(c.Request.Context(), u.ID, u.Username, req)
	if err != nil {
		h.writeErr(c, err, "세션 생성 실패")
		return
	}
	httpx.Created(c, sess)
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context(), uid(c))
	if err != nil {
		httpx.Internal(c, "세션 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Connection(c *gin.Context) {
	conn, err := h.svc.Connection(c.Request.Context(), c.Param("id"), uid(c))
	if err != nil {
		h.writeErr(c, err, "연결정보 조회 실패")
		return
	}
	httpx.OK(c, conn)
}

// Access는 게이트웨이 단기 접속 토큰(웹 URL·SSH 명령)을 발급한다(열 때마다 새로).
func (h *Handler) Access(c *gin.Context) {
	info, err := h.svc.Access(c.Request.Context(), c.Param("id"), uid(c))
	if err != nil {
		h.writeErr(c, err, "접속 토큰 발급 실패")
		return
	}
	httpx.OK(c, info)
}

func (h *Handler) Logs(c *gin.Context) {
	logs, err := h.svc.Logs(c.Request.Context(), c.Param("id"), uid(c))
	if err != nil {
		// 파드 부재/스케줄 전/k8s 일시 미가용 등은 500 대신 graceful empty(뷰어가 안내). AdminLogs 와 동일.
		httpx.OK(c, gin.H{"logs": "", "error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"logs": logs})
}

// Usage는 세션 실사용 지표(CPU/RAM/GPU/VRAM). 측정 불가 항목은 null 로 내려간다.
func (h *Handler) Usage(c *gin.Context) {
	u, err := h.svc.Usage(c.Request.Context(), c.Param("id"), uid(c))
	if err != nil {
		h.writeErr(c, err, "사용량 조회 실패")
		return
	}
	httpx.OK(c, u)
}

// MyUsage는 본인 running 세션 전체의 실사용 지표를 id→Usage 맵으로 반환한다(목록 폴링용).
// 세션마다 호출하지 않도록 벌크로 둔다(/instances/:id/metrics 와 라우트가 겹치지 않게 경로 분리).
func (h *Handler) MyUsage(c *gin.Context) {
	items, err := h.svc.MyUsage(c.Request.Context(), uid(c))
	if err != nil {
		httpx.Internal(c, "사용량 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// History는 본인 세션의 사용률 이력(기본 24h) 시계열 + 요약을 반환한다(?hours 로 조정).
func (h *Handler) History(c *gin.Context) {
	pts, ins, err := h.svc.UserHistory(c.Request.Context(), c.Param("id"), uid(c), histHours(c))
	if err != nil {
		h.writeErr(c, err, "사용 이력 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"points": pts, "insights": ins})
}

// Audit는 세션 대상 감사 로그(로그 탭에 표시).
func (h *Handler) Audit(c *gin.Context) {
	items, err := h.svc.Audit(c.Param("id"), uid(c))
	if err != nil {
		h.writeErr(c, err, "감사 로그 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) Stop(c *gin.Context)  { h.act(c, h.svc.Stop, "정지 실패") }
func (h *Handler) Start(c *gin.Context) { h.act(c, h.svc.Start, "시작 실패") }

// Extend는 선착순(dynamic) 세션 임대를 연장한다.
func (h *Handler) Extend(c *gin.Context) {
	if err := h.svc.Extend(c.Request.Context(), c.Param("id"), uid(c)); err != nil {
		h.writeErr(c, err, "연장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), uid(c)); err != nil {
		h.writeErr(c, err, "삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// act는 (ctx, id, uid)→error 형태 동작의 공통 래퍼.
func (h *Handler) act(c *gin.Context, fn func(context.Context, string, int64) error, msg string) {
	if err := fn(c.Request.Context(), c.Param("id"), uid(c)); err != nil {
		h.writeErr(c, err, msg)
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func (h *Handler) writeErr(c *gin.Context, err error, msg string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(c, "세션을 찾을 수 없습니다")
	case errors.Is(err, ErrSessionLimit):
		httpx.Err(c, 409, "session_limit", "동시 실행 가능한 세션 수를 초과했습니다")
	case errors.Is(err, node.ErrNodeBusy):
		httpx.Err(c, 409, "node_busy", "이미 다른 사용자가 점유 중인 노드입니다")
	case errors.Is(err, ErrNodeRequired):
		httpx.Err(c, 400, "node_required", "대여할 노드를 선택하세요")
	case errors.Is(err, ErrLocalHomeForbidden):
		httpx.Err(c, 403, "local_home_forbidden", "대여한 적 없는 노드의 로컬 Home 은 사용할 수 없습니다")
	case errors.Is(err, ErrLocalHomeUnavailable):
		httpx.Err(c, 409, "local_home_unavailable", "선택한 로컬 Home 노드를 현재 사용할 수 없습니다")
	case errors.Is(err, ErrExtendUnavailable):
		httpx.Err(c, 409, "extend_unavailable", "임대를 연장할 수 없습니다")
	case errors.Is(err, ErrInsufficientCredit):
		httpx.Err(c, 402, "insufficient_credit", "크레딧이 부족합니다(최소 1시간 단가 이상 필요)")
	case errors.Is(err, ErrMustJoinTeam):
		httpx.Err(c, 400, "must_join_team", "세션을 만들려면 먼저 팀에 소속되어야 합니다")
	case errors.Is(err, ErrHardLimit):
		httpx.Err(c, 403, "hard_limit", "허용된 리소스 상한(GPU·VRAM)을 초과했습니다")
	case errors.Is(err, k8s.ErrNoCluster):
		httpx.Err(c, 503, "k8s_unavailable", "클러스터를 사용할 수 없습니다")
	default:
		// 알려지지 않은 에러(주로 프로비저닝: PVC/네임스페이스/스케줄)는 500 으로 나가되,
		// 원시 에러를 반드시 서버 로그에 남긴다 — 안 그러면 클라이언트엔 "실패"만 뜨고 원인이 사라진다.
		log.Printf("[session] %s: %v", msg, err)
		httpx.Internal(c, msg)
	}
}

// ── 관리자 세션 관제 ──────────────────────
func (h *Handler) AdminList(c *gin.Context) {
	items, err := h.svc.AdminList()
	if err != nil {
		httpx.Internal(c, "세션 관제 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// AdminUsage는 running 세션 전체의 실사용 지표를 id→Usage 맵으로 반환한다(관제 폴링용).
func (h *Handler) AdminUsage(c *gin.Context) {
	items, err := h.svc.AdminUsage(c.Request.Context())
	if err != nil {
		httpx.Internal(c, "사용량 관제 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) AdminTerminate(c *gin.Context) {
	if err := h.svc.AdminTerminate(c.Request.Context(), c.Param("id")); err != nil {
		h.writeErr(c, err, "강제 종료 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// AdminStop은 세션을 중단(stopped)한다 — 종료와 달리 재시작 가능.
func (h *Handler) AdminStop(c *gin.Context) {
	if err := h.svc.AdminStop(c.Request.Context(), c.Param("id")); err != nil {
		h.writeErr(c, err, "세션 중단 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// AdminDelete은 세션 레코드를 완전히 삭제한다.
func (h *Handler) AdminDelete(c *gin.Context) {
	if err := h.svc.AdminDelete(c.Request.Context(), c.Param("id")); err != nil {
		h.writeErr(c, err, "세션 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// AdminGet은 단일 세션 상세(관제).
func (h *Handler) AdminGet(c *gin.Context) {
	row, err := h.svc.AdminGet(c.Param("id"))
	if err != nil {
		h.writeErr(c, err, "세션 조회 실패")
		return
	}
	httpx.OK(c, row)
}

// AdminAudit은 세션 감사 로그(관제).
func (h *Handler) AdminAudit(c *gin.Context) {
	items, err := h.svc.AdminAudit(c.Param("id"))
	if err != nil {
		httpx.Internal(c, "세션 로그 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// AdminDescribe는 세션 파드의 describe 요약(상태·조건·이벤트)을 반환한다(관제).
func (h *Handler) AdminDescribe(c *gin.Context) {
	d, err := h.svc.AdminDescribe(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.OK(c, gin.H{"describe": nil, "error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"describe": d})
}

// AdminLogs는 세션 파드의 쿠버네티스 로그(tail)를 반환한다(관제).
func (h *Handler) AdminLogs(c *gin.Context) {
	logs, err := h.svc.AdminLogs(c.Request.Context(), c.Param("id"), 300)
	if err != nil {
		httpx.OK(c, gin.H{"logs": "", "error": err.Error()}) // 로그 미가용도 200(뷰어가 안내)
		return
	}
	httpx.OK(c, gin.H{"logs": logs})
}

// AdminHistory는 임의 세션의 사용률 이력(관제 세션 상세)을 반환한다.
func (h *Handler) AdminHistory(c *gin.Context) {
	pts, ins, err := h.svc.AdminHistory(c.Request.Context(), c.Param("id"), histHours(c))
	if err != nil {
		h.writeErr(c, err, "사용 이력 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"points": pts, "insights": ins})
}

// histHours는 ?hours 쿼리를 파싱한다(기본 24, 1~168 제한).
func histHours(c *gin.Context) int {
	h, _ := strconv.Atoi(c.Query("hours"))
	if h <= 0 {
		return 24
	}
	if h > 168 {
		return 168
	}
	return h
}

func uid(c *gin.Context) int64 { return auth.CurrentUser(c).ID }
