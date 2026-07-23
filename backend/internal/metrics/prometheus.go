// Package metrics는 Prometheus(HTTP API)에서 대시보드용 지표를 조회한다.
//
// URL 이 비었거나 클러스터에 Prometheus 가 없으면 "미가용"으로 간주하고
// (0, false) 를 반환한다 — 대시보드는 이를 0/빈 값으로 안전하게 표시한다.
// GPU 지표는 DCGM exporter(dcgm-exporter)가 노출하는 메트릭을 전제로 한다.
package metrics

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client는 Prometheus instant/range 쿼리 클라이언트.
type Client struct {
	base string
	http *http.Client
}

// New는 base URL(빈 값이면 미가용)로 클라이언트를 만든다.
func New(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: 5 * time.Second}}
}

// Enabled는 Prometheus 연동 가능 여부.
func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// Scalar는 instant 쿼리 결과의 첫 값을 반환한다. 미가용/오류면 (0,false).
func (c *Client) Scalar(ctx context.Context, query string) (float64, bool) {
	if !c.Enabled() {
		return 0, false
	}
	u := c.base + "/api/v1/query?query=" + url.QueryEscape(query)
	v, ok := c.fetchVector(ctx, u)
	if !ok || len(v) == 0 {
		return 0, false
	}
	return v[0].value, true
}

// VectorByLabel은 instant 쿼리 결과를 지정 라벨값→값 맵으로 반환한다(노드별 집계용).
func (c *Client) VectorByLabel(ctx context.Context, query, label string) (map[string]float64, bool) {
	if !c.Enabled() {
		return nil, false
	}
	u := c.base + "/api/v1/query?query=" + url.QueryEscape(query)
	return c.fetchVectorLabeled(ctx, u, label)
}

// Point는 range 쿼리의 (라벨, 시간순 값) 시리즈 1개를 단순화해 반환한다.
type Point struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Range는 [start,end] 구간을 step 간격으로 조회한 단일 시리즈를 반환한다.
func (c *Client) Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Point, bool) {
	if !c.Enabled() {
		return nil, false
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds())))
	pts, ok := c.fetchMatrix(ctx, c.base+"/api/v1/query_range?"+q.Encode())
	if !ok {
		return nil, false
	}
	return pts, true
}
