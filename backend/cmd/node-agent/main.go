// Command node-agent는 물리(hybrid) 노드에서 DaemonSet 으로 동작하는 에이전트다.
// API 에서 자기 노드의 활성 임대를 받아 계정/SSH/NFS 마운트를 reconcile 한다.
//
//	env: GIOSK_API_URL, GIOSK_AGENT_TOKEN, GIOSK_NODE_NAME(또는 NODE_NAME), GIOSK_AGENT_INTERVAL(초)
package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"giosk/internal/nodeagent"
)

func main() {
	apiURL := env("GIOSK_API_URL", "http://localhost:8080")
	token := os.Getenv("GIOSK_AGENT_TOKEN")
	node := env("GIOSK_NODE_NAME", os.Getenv("NODE_NAME"))
	interval := time.Duration(intEnv("GIOSK_AGENT_INTERVAL", 10)) * time.Second

	if node == "" || token == "" {
		log.Fatal("node-agent: GIOSK_NODE_NAME 과 GIOSK_AGENT_TOKEN 이 필요합니다")
	}

	src := nodeagent.NewAPIClient(apiURL, token, node)
	// GIOSK_AGENT_MODE=shell 이면 실제 호스트 계정과 마운트를 다룬다(운영). 기본은 log 라 개발이나 kind 에서 안전하다.
	var exec nodeagent.Executor
	if env("GIOSK_AGENT_MODE", "log") == "shell" {
		homeBase := env("GIOSK_PHYSICAL_HOME_HOSTPATH", "/home/giosk")
		nsenter := os.Getenv("GIOSK_AGENT_NSENTER") != "0" // 기본 on. privileged DaemonSet 이라 호스트 PID1 로 들어간다
		exec = nodeagent.NewShellExecutor(node, homeBase, nsenter).
			WithGatewayKey(os.Getenv("GIOSK_GATEWAY_SSH_PUBKEY")) // 게이트웨이 프록시 접속용 공개키 병기
		log.Printf("giosk node-agent: ShellExecutor (homeBase=%s, nsenter=%v)", homeBase, nsenter)
	} else {
		exec = nodeagent.NewLogExecutor(node)
	}
	log.Printf("giosk node-agent 시작 (node=%s, api=%s, interval=%s)", node, apiURL, interval)
	nodeagent.NewReconciler(src, exec).Run(interval)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}
