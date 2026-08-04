package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// promResp는 Prometheus API 응답(공통 envelope).
type promResp struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

type sample struct{ value float64 }

// fetchVector는 instant(vector) 결과를 디코드한다.
func (c *Client) fetchVector(ctx context.Context, u string) ([]sample, bool) {
	resp, ok := c.get(ctx, u)
	if !ok {
		return nil, false
	}
	out := make([]sample, 0, len(resp.Data.Result))
	for _, raw := range resp.Data.Result {
		var r struct {
			Value [2]any `json:"value"`
		}
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		if v, ok := scalarOf(r.Value[1]); ok {
			out = append(out, sample{value: v})
		}
	}
	return out, true
}

// fetchVectorLabeled는 instant(vector) 결과를 metric[label] 기준 값 맵으로 디코드한다.
func (c *Client) fetchVectorLabeled(ctx context.Context, u, label string) (map[string]float64, bool) {
	resp, ok := c.get(ctx, u)
	if !ok {
		return nil, false
	}
	out := map[string]float64{}
	for _, raw := range resp.Data.Result {
		var r struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		}
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		key := r.Metric[label]
		if key == "" {
			continue
		}
		if v, ok := scalarOf(r.Value[1]); ok {
			out[key] = v
		}
	}
	return out, true
}

// fetchMatrix는 range(matrix) 결과의 첫 시리즈를 시간순 Point 로 디코드한다.
func (c *Client) fetchMatrix(ctx context.Context, u string) ([]Point, bool) {
	resp, ok := c.get(ctx, u)
	if !ok || len(resp.Data.Result) == 0 {
		return nil, ok
	}
	var r struct {
		Values [][2]any `json:"values"`
	}
	if json.Unmarshal(resp.Data.Result[0], &r) != nil {
		return nil, false
	}
	pts := make([]Point, 0, len(r.Values))
	for _, pair := range r.Values {
		t, _ := scalarOf(pair[0])
		v, ok := scalarOf(pair[1])
		if ok {
			pts = append(pts, Point{T: t, V: v})
		}
	}
	return pts, true
}

func (c *Client) get(ctx context.Context, u string) (*promResp, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, false
	}
	var pr promResp
	if json.NewDecoder(res.Body).Decode(&pr) != nil || pr.Status != "success" {
		return nil, false
	}
	return &pr, true
}

// scalarOf는 Prometheus 값을 파싱한다. 샘플 값은 문자열("0.5")이지만 range/instant 의
// 타임스탬프(pair[0])는 JSON 숫자(float)로 오므로 둘 다 받아야 한다. 문자열만 받으면
// matrix 타임스탬프가 전부 0 으로 파싱돼 range 결과가 한 점(t=0)으로 붕괴한다(노드·세션 이력 1점 버그).
func scalarOf(v any) (float64, bool) {
	switch x := v.(type) {
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case float64:
		return x, true
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
