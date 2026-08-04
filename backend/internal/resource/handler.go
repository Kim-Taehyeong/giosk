package resource

import (
	"errors"
	"strconv"
	"strings"

	"giosk/pkg/httpx"

	"github.com/gin-gonic/gin"
)

// Handler는 resource HTTP 핸들러(얇게).
type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ── 오퍼링 ──────────────────────────────
func (h *Handler) ListOfferings(c *gin.Context) {
	items, err := h.svc.ListOfferings(activeOnly(c))
	if err != nil {
		httpx.Internal(c, "오퍼링 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) SaveOffering(c *gin.Context) {
	var o Offering
	if err := c.ShouldBindJSON(&o); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	o.ID = idParam(c) // 0=생성, >0=수정
	if err := h.svc.SaveOffering(&o); err != nil {
		if errors.Is(err, ErrOfferingDup) {
			httpx.Err(c, 409, "offering_dup", "같은 규격의 오퍼링이 이미 있습니다: "+strings.TrimPrefix(err.Error(), ErrOfferingDup.Error()+": "))
			return
		}
		httpx.Internal(c, "오퍼링 저장 실패")
		return
	}
	httpx.OK(c, o)
}

func (h *Handler) DeleteOffering(c *gin.Context) {
	if err := h.svc.DeleteOffering(idParam(c)); err != nil {
		httpx.Internal(c, "오퍼링 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── 프리셋 ──────────────────────────────
func (h *Handler) ListPresets(c *gin.Context) {
	items, err := h.svc.ListPresets(activeOnly(c))
	if err != nil {
		httpx.Internal(c, "프리셋 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) SavePreset(c *gin.Context) {
	var p Preset
	if err := c.ShouldBindJSON(&p); err != nil {
		httpx.BadRequest(c, "본문 오류")
		return
	}
	p.ID = idParam(c)
	if err := h.svc.SavePreset(&p); err != nil {
		httpx.Internal(c, "프리셋 저장 실패")
		return
	}
	httpx.OK(c, p)
}

func (h *Handler) DeletePreset(c *gin.Context) {
	if err := h.svc.DeletePreset(idParam(c)); err != nil {
		httpx.Internal(c, "프리셋 삭제 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// ── GPU 타입(노드 라벨 수집) ───────────────
func (h *Handler) GpuTypes(c *gin.Context) {
	items, err := h.svc.GpuTypes(c.Request.Context())
	if err != nil {
		httpx.Internal(c, "GPU 타입 수집 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items, "cpuPricePerHour": h.svc.CpuPrice()})
}

// Availability는 GPU 가용 현황(세션 마법사·대시보드 공용 단일 소스).
func (h *Handler) Availability(c *gin.Context) {
	httpx.OK(c, h.svc.Availability(c.Request.Context()))
}

// ── 전용 GPU 단가(과금 모델) ───────────────
func (h *Handler) GpuPricing(c *gin.Context) {
	items, err := h.svc.GpuPricing()
	if err != nil {
		httpx.Internal(c, "단가 조회 실패")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) SetGpuPricing(c *gin.Context) {
	var p GpuPrice
	if err := c.ShouldBindJSON(&p); err != nil || p.GpuType == "" {
		httpx.BadRequest(c, "gpuType 필요")
		return
	}
	if err := h.svc.SetGpuPricing(p); err != nil {
		httpx.Internal(c, "단가 저장 실패")
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

func idParam(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

// activeOnly는 ?active=true 쿼리(사용자 카탈로그용).
func activeOnly(c *gin.Context) bool { return c.Query("active") == "true" }
