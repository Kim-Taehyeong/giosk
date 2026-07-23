package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"giosk/internal/metrics"

	"github.com/gin-gonic/gin"
)

// fakeProm은 PromQL 쿼리의 메트릭 이름으로 응답을 고르는 가짜 Prometheus.
// (metrics.Client 가 구체 타입이라 HTTP 로 세운다 — 디코드 경로까지 함께 검증된다.)
func fakeProm(t *testing.T, series map[string]map[string]float64) *metrics.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.Query().Get("query"))
		label := "pod"
		if strings.Contains(q, "podname") {
			label = "podname"
		}
		var result []map[string]any
		for metric, byPod := range series {
			if !strings.Contains(q, metric) {
				continue
			}
			for pod, v := range byPod {
				result = append(result, map[string]any{
					"metric": map[string]string{label: pod},
					"value":  []any{float64(0), formatFloat(v)},
				})
			}
			break
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": result},
		})
	}))
	t.Cleanup(srv.Close)
	return metrics.New(srv.URL)
}

func formatFloat(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestUsageOf_SourceBySharingMode(t *testing.T) {
	// FB_USED 는 "DCGM_FI_DEV_FB_USED" 와 "...FB_USED + ...FB_FREE" 두 쿼리에 모두 걸리므로
	// 맵 순회 순서에 의존하지 않도록 총량 쿼리는 이 테스트에서 확인하지 않는다.
	met := fakeProm(t, map[string]map[string]float64{
		"container_cpu_usage_seconds_total":    {"ses-excl": 2.5, "ses-hami": 1.0, "ses-slice": 0.5, "ses-cpu": 3.0},
		"container_memory_working_set_bytes":   {"ses-excl": 2 * 1024 * 1024 * 1024},
		"DCGM_FI_DEV_GPU_UTIL":                 {"ses-excl": 73},
		"Device_utilization_desc_of_container": {"ses-hami": 42},
		"vGPU_device_memory_usage_in_bytes":    {"ses-hami": 3 * 1024 * 1024 * 1024},
	})
	svc := &Service{met: met}

	rows := []Session{
		{InstanceID: "ses-excl", GpuMode: "exclusive", CPUCores: 8, MemGB: 32},
		{InstanceID: "ses-hami", GpuMode: "shared", VramMB: 8192},
		{InstanceID: "ses-slice", GpuMode: "timeslice"},
		{InstanceID: "ses-cpu", GpuMode: "cpu"},
		{InstanceID: "ses-phys", GpuMode: "exclusive", Env: "ssh"},
	}
	got := svc.usageOf(context.Background(), rows)

	// 전용 — DCGM 이 곧 세션 사용량.
	excl := got["ses-excl"]
	if excl.GpuSource != gpuSrcDCGM {
		t.Fatalf("exclusive source = %q, want dcgm", excl.GpuSource)
	}
	if excl.GpuUtil == nil || *excl.GpuUtil != 73 {
		t.Errorf("exclusive gpuUtil = %v, want 73", excl.GpuUtil)
	}
	if excl.CPUCores == nil || *excl.CPUCores != 2.5 {
		t.Errorf("exclusive cpuCores = %v, want 2.5", excl.CPUCores)
	}
	if excl.MemUsedMB == nil || *excl.MemUsedMB != 2048 {
		t.Errorf("exclusive memUsedMb = %v, want 2048 (bytes→MiB)", excl.MemUsedMB)
	}

	// 분할(HAMi) — DCGM 이 아니라 vGPUmonitor(podname 라벨)에서 온다.
	hami := got["ses-hami"]
	if hami.GpuSource != gpuSrcHAMi {
		t.Fatalf("shared source = %q, want hami", hami.GpuSource)
	}
	if hami.GpuUtil == nil || *hami.GpuUtil != 42 {
		t.Errorf("shared gpuUtil = %v, want 42", hami.GpuUtil)
	}
	if hami.VramUsedMB == nil || *hami.VramUsedMB != 3072 {
		t.Errorf("shared vramUsedMb = %v, want 3072 (bytes→MiB)", hami.VramUsedMB)
	}

	// 타임슬라이싱 — GPU 는 측정 불가지만 CPU/RAM 은 cgroup 기반이라 여전히 유효하다.
	slice := got["ses-slice"]
	if slice.GpuSource != gpuSrcNone || slice.GpuReason != gpuNoneTimeslice {
		t.Errorf("timeslice = (%q,%q), want (none,timeslice)", slice.GpuSource, slice.GpuReason)
	}
	if slice.GpuUtil != nil {
		t.Errorf("timeslice gpuUtil = %v, want nil — 카드 전체값을 세션값으로 내주면 안 된다", *slice.GpuUtil)
	}
	if slice.CPUCores == nil || *slice.CPUCores != 0.5 {
		t.Errorf("timeslice cpuCores = %v, want 0.5", slice.CPUCores)
	}

	// CPU 세션 / 물리(SSH) 세션.
	if c := got["ses-cpu"]; c.GpuReason != gpuNoneCPU || c.CPUCores == nil || *c.CPUCores != 3 {
		t.Errorf("cpu session = %+v", c)
	}
	if p := got["ses-phys"]; p.GpuReason != gpuNonePhysical || p.CPUCores != nil {
		t.Errorf("physical session = %+v, want reason=physical + Pod 지표 없음", p)
	}
}

// Prometheus 미연동이면 전 세션이 "측정 불가"여야 한다 — 0 으로 채워 "안 쓰는 중"처럼 보이면 안 된다.
func TestUsageOf_NoPrometheus(t *testing.T) {
	svc := &Service{met: metrics.New("")}
	got := svc.usageOf(context.Background(), []Session{{InstanceID: "ses-a", GpuMode: "exclusive"}})
	u := got["ses-a"]
	if u.GpuSource != gpuSrcNone || u.GpuReason != gpuNoneUnavailable {
		t.Errorf("= (%q,%q), want (none,unavailable)", u.GpuSource, u.GpuReason)
	}
	if u.CPUCores != nil || u.GpuUtil != nil {
		t.Errorf("측정 불가인데 값이 채워졌다: %+v", u)
	}
}

// gin 은 같은 자리에 정적 세그먼트와 :id 를 함께 등록하면 패닉한다 — 등록 시점에 잡는다.
func TestRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(&Service{})
	RegisterUser(r.Group("/api"), h)
	RegisterAdmin(r.Group("/api/admin"), h)

	want := map[string]bool{
		"GET /api/metrics/sessions":       false,
		"GET /api/instances/:id/metrics":  false,
		"GET /api/admin/sessions/metrics": false,
	}
	for _, ri := range r.Routes() {
		if _, ok := want[ri.Method+" "+ri.Path]; ok {
			want[ri.Method+" "+ri.Path] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("라우트 미등록: %s", route)
		}
	}
}
