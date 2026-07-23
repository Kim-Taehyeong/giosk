package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// APIClient는 API 의 node-agent 엔드포인트에서 활성 임대를 가져온다(LeaseSource 구현).
type APIClient struct {
	baseURL string
	token   string
	node    string
	http    *http.Client
}

func NewAPIClient(baseURL, token, node string) *APIClient {
	return &APIClient{baseURL: baseURL, token: token, node: node, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *APIClient) ActiveLeases() ([]Lease, error) {
	u := fmt.Sprintf("%s/api/node-agent/leases?node=%s", c.baseURL, url.QueryEscape(c.node))
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent leases: status %d", resp.StatusCode)
	}
	var body struct {
		Items []Lease `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Items, nil
}
