package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"giosk/internal/k8s"
)

// ErrNotBuildable는 외부/기본(시드) 이미지처럼 Dockerfile 이 없어 재빌드할 수 없는 이미지.
var ErrNotBuildable = errors.New("not buildable")

// Builder는 이미지 빌드/프리페치 잡 실행 계약(*k8s.Client 구현).
type Builder interface {
	RunBuildJob(ctx context.Context, s k8s.BuildSpec) error
	BuildJobStatus(ctx context.Context, ns, jobName string) (string, error)
	DeleteBuildJob(ctx context.Context, ns, jobName string) error
	EnsurePrefetchPod(ctx context.Context, ns, name, node, imageRef string) error
	PrefetchStatus(ctx context.Context, ns, name string) string
	DeletePrefetchPod(ctx context.Context, ns, name string) error
	RunScanJob(ctx context.Context, ns, name, imageRef, registry string) error
	RunSignJob(ctx context.Context, ns, name, imageRef, keySecret, registry string) error
	EnsureCosignKey(ctx context.Context, ns, secret string) (bool, error)
	BuildLogs(ctx context.Context, ns, jobName string, tail int64) string
}

// Service는 image 빌드 파이프라인(가이드 입력→Dockerfile 생성→Kaniko Job→상태 리컨실).
type Service struct {
	repo      Repository
	builder   Builder
	registry  string // 푸시/풀 레지스트리(빈값=빌드 비활성)
	buildNS   string // 빌드 Job 네임스페이스
	cosignKey string // cosign 서명키 Secret 이름(빈값=서명 생략)
}

func NewService(repo Repository, builder Builder, registry, buildNS string) *Service {
	return &Service{repo: repo, builder: builder, registry: registry, buildNS: buildNS}
}

// WithCosign은 빌드 후 cosign 서명 단계를 활성화한다(키 Secret 이름).
func (s *Service) WithCosign(keySecret string) *Service { s.cosignKey = keySecret; return s }

// Enabled는 빌드 기능 가용 여부(레지스트리+빌더 주입 시).
func (s *Service) Enabled() bool { return s.registry != "" && s.builder != nil }

// Build는 가이드 입력으로 Dockerfile 을 만들고 이미지 행 + Kaniko Job 을 생성한다.
func (s *Service) Build(ctx context.Context, req BuildReq) error {
	if !s.Enabled() {
		return fmt.Errorf("이미지 빌드 비활성(레지스트리 미설정)")
	}
	img := &Image{
		Name:        s.registry + "/" + req.Name, // 세션 풀 ref 와 일치하도록 레지스트리 접두
		Tag:         "1",
		BaseImage:   req.Base,
		Description: strings.TrimSpace(req.Desc),
		Dockerfile:  genDockerfile(req.Base, req.Apt, req.Pip),
		Channels:    req.Channels,
		GPU:         req.GPU,
		ScanStatus: "building",
		Status:     StatusBuilding,
	}
	if err := s.repo.Create(img); err != nil {
		return err
	}
	return s.launch(ctx, img)
}

// Rebuild는 기존 이미지를 태그를 올려 다시 빌드한다(상태=building).
func (s *Service) Rebuild(ctx context.Context, id int64) error {
	if !s.Enabled() {
		return fmt.Errorf("이미지 빌드 비활성(레지스트리 미설정)")
	}
	img, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	// 외부/기본 이미지(예: codercom/code-server, jupyter)는 Dockerfile 이 없어 빌드·푸시 불가
	// (push 대상이 Docker Hub 의 원본이라 권한 없음). 가이드형으로 만든 이미지만 재빌드 가능.
	if strings.TrimSpace(img.Dockerfile) == "" {
		return ErrNotBuildable
	}
	img.Tag = bumpTag(img.Tag)
	img.ScanStatus, img.Status = "building", StatusBuilding
	if err := s.repo.Save(img); err != nil {
		return err
	}
	return s.launch(ctx, img)
}

// RegisterExternal은 외부 이미지를 빌드 없이 등록한다(항상 가용 — 레지스트리 불요).
// Name 에 원본 ref 를 접두 없이 저장하므로 세션 Pod 이 그 origin 에서 직접 pull 한다.
// Dockerfile="" 라 재빌드는 자동 차단(Rebuild 의 기존 가드). 상태는 바로 active(빌드/스캔 파이프라인 없음, seed 선례).
func (s *Service) RegisterExternal(req ExternalReq) error {
	name, tag := splitRef(strings.TrimSpace(req.Ref))
	if name == "" {
		return fmt.Errorf("이미지 ref 가 비어 있습니다")
	}
	// 커스텀 웹 포트가 있으면 채널명을 k8s Service 포트명 규칙(소문자 영숫자·'-', ≤15)으로 정규화한다.
	// 규칙을 어기면 세션 Service 생성이 통째로 깨지므로 여기서 막는다.
	if norm, err := normalizeChannelsWebName(req.Channels); err != nil {
		return err
	} else {
		req.Channels = norm
	}
	img := &Image{
		Name:        name, // 레지스트리 접두 없음 — 원본에서 직접 pull
		Tag:         tag,
		BaseImage:   req.Ref, // 표시용(빌드 이미지의 base 자리와 동일 위치)
		Description: strings.TrimSpace(req.Desc),
		Dockerfile:  "", // 외부 → 재빌드 불가
		Channels:    req.Channels,
		GPU:         req.GPU,
		ScanStatus:  "pass", // 외부는 스캔 파이프라인 없음(seed 와 동일)
		Status:      StatusActive,
	}
	return s.repo.Create(img)
}

// normalizeChannelsWebName은 channels JSON 안 web.name 을 k8s 포트명 규칙으로 정규화한다.
// name 미지정이면 "web". 포트가 유효하지 않으면(≤0) 에러. 그 외 채널 필드는 보존.
func normalizeChannelsWebName(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, nil // 파싱 불가면 그대로(하위 계층이 무시)
	}
	web, ok := m["web"].(map[string]any)
	if !ok {
		return raw, nil // 커스텀 포트 없음
	}
	port, _ := web["port"].(float64)
	if int(port) <= 0 {
		return nil, fmt.Errorf("커스텀 웹 포트가 올바르지 않습니다")
	}
	name, _ := web["name"].(string)
	name = sanitizePortName(name)
	if name == "" {
		name = "web"
	}
	web["name"] = name
	m["web"] = web
	out, err := json.Marshal(m)
	if err != nil {
		return raw, nil
	}
	return out, nil
}

// sanitizePortName은 k8s Service 포트명 규칙(소문자 영숫자·'-', 시작/끝 영숫자, ≤15)에 맞게 다듬는다.
func sanitizePortName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			b.WriteRune(ch)
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 15 {
		out = strings.Trim(out[:15], "-")
	}
	return out
}

// splitRef는 이미지 ref 를 name/tag 로 나눈다. 레지스트리 host:port 의 콜론과 구분하려고
// 마지막 "/" 뒤에서만 태그 콜론을 찾는다(예: registry:5000/img:1 → name=registry:5000/img, tag=1).
// 태그가 없으면 latest.
func splitRef(ref string) (name, tag string) {
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon > slash { // 콜론이 마지막 경로 세그먼트 안 → 태그
		return ref[:colon], ref[colon+1:]
	}
	return ref, "latest"
}

func (s *Service) launch(ctx context.Context, img *Image) error {
	return s.builder.RunBuildJob(ctx, k8s.BuildSpec{
		Namespace:  s.buildNS,
		JobName:    fmt.Sprintf("build-%d", img.ID),
		ImageRef:   img.Name + ":" + img.Tag, // Name 에 레지스트리 접두 포함
		Dockerfile: img.Dockerfile,
		Registry:   s.registry,
	})
}

// RunReconciler는 building 이미지의 빌드 Job + 노드 캐시 프리페치 상태를 주기적으로 반영한다.
func (s *Service) RunReconciler(ctx context.Context, interval time.Duration) {
	if s.builder == nil { // 빌드(레지스트리)·캐시 모두 클러스터 필요
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.reconcileOnce(ctx)
		}
	}
}

// CacheOnNode는 이미지를 특정 노드에 프리페치(미리 풀)하도록 한다.
func (s *Service) CacheOnNode(ctx context.Context, imageID int64, node string) error {
	if s.builder == nil {
		return fmt.Errorf("클러스터 미연동")
	}
	img, err := s.repo.Get(imageID)
	if err != nil {
		return err
	}
	ref := img.Name + ":" + img.Tag
	if err := s.builder.EnsurePrefetchPod(ctx, s.buildNS, prefetchName(imageID, node), node, ref); err != nil {
		return err
	}
	return s.repo.CacheSet(imageID, node, "pulling")
}

// UncacheNode는 노드 캐시 기록/프리페치 Pod 를 제거한다(노드 레이어는 containerd GC 까지 잔존).
func (s *Service) UncacheNode(ctx context.Context, imageID int64, node string) error {
	if s.builder != nil {
		_ = s.builder.DeletePrefetchPod(ctx, s.buildNS, prefetchName(imageID, node))
	}
	return s.repo.CacheDelete(imageID, node)
}

// CacheList는 이미지의 노드별 캐시 상태를 반환한다.
func (s *Service) CacheList(imageID int64) []NodeCache { return s.repo.CacheByImage(imageID) }

// ImageLogs는 빌드/스캔 Job 파드 로그를 반환한다(실패 진단용). scan→build 순으로 시도.
func (s *Service) ImageLogs(ctx context.Context, imageID int64) string {
	if s.builder == nil {
		return ""
	}
	for _, job := range []string{scanJob(imageID), fmt.Sprintf("build-%d", imageID)} {
		if l := s.builder.BuildLogs(ctx, s.buildNS, job, 300); strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}

// prefetchName은 프리페치 Pod 이름(노드명 정규화).
func prefetchName(imageID int64, node string) string {
	safe := make([]rune, 0, len(node))
	for _, r := range node {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			safe = append(safe, r)
		} else {
			safe = append(safe, '-')
		}
	}
	return fmt.Sprintf("prefetch-%d-%s", imageID, string(safe))
}

func (s *Service) reconcileOnce(ctx context.Context) {
	s.reconcileCache(ctx)
	building, err := s.repo.ListBuilding()
	if err != nil {
		return
	}
	for _, img := range building {
		ref := img.Name + ":" + img.Tag
		switch img.ScanStatus {

		case "scanning": // trivy 스캔 단계
			st, err := s.builder.BuildJobStatus(ctx, s.buildNS, scanJob(img.ID))
			if err != nil || st == k8s.BuildRunning {
				continue
			}
			if st == k8s.BuildFailed { // HIGH/CRITICAL 취약점 → 등록하되 high 플래그(미서명). Job 은 로그 조회 위해 보존.
				_ = s.repo.SetBuildResult(img.ID, StatusDraft, "high")
				log.Printf("[image] scan %s → HIGH", img.Name)
				continue
			}
			_ = s.builder.DeleteBuildJob(ctx, s.buildNS, scanJob(img.ID)) // 통과 시에만 정리
			if s.cosignKey != "" {                                        // 스캔 통과 + 서명키 설정 → 서명 준비 단계(키 보장 후 서명)
				_ = s.repo.SetScanStatus(img.ID, "signpending")
			} else {
				_ = s.repo.SetBuildResult(img.ID, StatusDraft, "pass")
				log.Printf("[image] scan %s → pass (서명 생략)", img.Name)
			}

		case "signpending": // 서명키 보장(없으면 in-cluster 자동 생성) → 준비되면 서명 Job 기동
			ready, err := s.builder.EnsureCosignKey(ctx, s.buildNS, s.cosignKey)
			if err != nil {
				log.Printf("[image] cosign 키 보장 실패 %s: %v", s.cosignKey, err)
				continue
			}
			if !ready { // 키 생성 Job 진행 중 — 다음 주기 재시도
				continue
			}
			_ = s.builder.RunSignJob(ctx, s.buildNS, signJob(img.ID), ref, s.cosignKey, s.registry)
			_ = s.repo.SetScanStatus(img.ID, "signing")

		case "signing": // cosign 서명 단계(스캔 pass 전제)
			st, err := s.builder.BuildJobStatus(ctx, s.buildNS, signJob(img.ID))
			if err != nil || st == k8s.BuildRunning {
				continue
			}
			_ = s.builder.DeleteBuildJob(ctx, s.buildNS, signJob(img.ID))
			if st == k8s.BuildSucceeded {
				_ = s.repo.SetSigned(img.ID, true)
			}
			_ = s.repo.SetBuildResult(img.ID, StatusDraft, "pass")
			log.Printf("[image] sign %s 완료(%s) → draft", img.Name, st)

		default: // 빌드 단계(scan_status=="building")
			st, err := s.builder.BuildJobStatus(ctx, s.buildNS, fmt.Sprintf("build-%d", img.ID))
			if err != nil || st == k8s.BuildRunning {
				continue
			}
			if st == k8s.BuildFailed { // 빌드 실패 → Job 보존(로그 조회용)
				_ = s.repo.SetBuildResult(img.ID, StatusFailed, "fail")
				log.Printf("[image] build %s 실패", img.Name)
				continue
			}
			_ = s.builder.DeleteBuildJob(ctx, s.buildNS, fmt.Sprintf("build-%d", img.ID)) // 성공 시에만 정리
			_ = s.builder.RunScanJob(ctx, s.buildNS, scanJob(img.ID), ref, s.registry)    // 빌드 성공 → trivy 스캔
			_ = s.repo.SetScanStatus(img.ID, "scanning")
			log.Printf("[image] build %s 완료 → 스캔", img.Name)
		}
	}
}

func scanJob(id int64) string { return fmt.Sprintf("scan-%d", id) }
func signJob(id int64) string { return fmt.Sprintf("sign-%d", id) }

// reconcileCache는 pulling 상태 노드 캐시의 프리페치 Pod 상태를 반영한다(완료/실패 시 Pod 정리).
func (s *Service) reconcileCache(ctx context.Context) {
	for _, c := range s.repo.CachePulling() {
		name := prefetchName(c.ImageID, c.Node)
		st := s.builder.PrefetchStatus(ctx, s.buildNS, name)
		if st == "pulling" {
			continue
		}
		_ = s.repo.CacheSet(c.ImageID, c.Node, st)
		_ = s.builder.DeletePrefetchPod(ctx, s.buildNS, name)
	}
}

// genDockerfile은 베이스 + apt/pip 패키지로 Dockerfile 을 생성한다(로컬 COPY 없음 → ConfigMap 컨텍스트로 충분).
func genDockerfile(base, apt, pip string) string {
	var b strings.Builder
	b.WriteString("FROM " + base + "\n")
	if a := pkgs(apt); len(a) > 0 {
		b.WriteString("RUN apt-get update && apt-get install -y --no-install-recommends " +
			strings.Join(a, " ") + " && rm -rf /var/lib/apt/lists/*\n")
	}
	if p := pkgs(pip); len(p) > 0 {
		b.WriteString("RUN pip install --no-cache-dir " + strings.Join(p, " ") + "\n")
	}
	return b.String()
}

// pkgs는 공백/콤마/줄바꿈으로 구분된 패키지 문자열을 토큰 슬라이스로 만든다.
func pkgs(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == '\r' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// bumpTag는 정수 태그를 1 증가시킨다(비정수면 1).
func bumpTag(tag string) string {
	n, err := strconv.Atoi(tag)
	if err != nil {
		return "1"
	}
	return strconv.Itoa(n + 1)
}
