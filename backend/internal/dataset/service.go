package dataset

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"giosk/internal/k8s"
	"giosk/internal/metrics"
)

// remoteSize는 URL 의 콘텐츠 크기(바이트)를 best-effort 로 측정한다(HEAD→실패 시 ranged GET).
// 측정 불가/오류면 0(표시는 "—"). 등록 응답 지연 방지를 위해 5초 타임아웃.
func remoteSize(url string) int64 {
	if url == "" {
		return 0
	}
	cl := &http.Client{Timeout: 5 * time.Second}
	if req, err := http.NewRequest(http.MethodHead, url, nil); err == nil {
		if resp, err := cl.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.ContentLength > 0 {
				return resp.ContentLength
			}
		}
	}
	// HEAD 미지원 서버 대비: 1바이트 범위 요청 → Content-Range 의 전체 크기 파싱.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := cl.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if cr := resp.Header.Get("Content-Range"); cr != "" { // 형식: "bytes 0-0/12345"
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			if n, err := strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// Provisioner는 데이터셋 RWX NFS PVC 프로비저닝 + URL 적재 계약(*k8s.Client 구현).
type Provisioner interface {
	CreatePVC(ctx context.Context, s k8s.PVCSpec) error
	RunDatasetFetch(ctx context.Context, ns, jobName, nfsServer, nfsBase, name, url string) error // mkdir <base>/dataset/<name> + curl
	RunDatasetExtract(ctx context.Context, ns, jobName, nfsServer, nfsBase, name string) error    // NFS 에 이미 올라온 아카이브 제자리 해제(업로드 경로)
	EnsureSharedNFSPVC(ctx context.Context, s k8s.SharedNFSSpec) error                            // 정규경로에 정적 NFS PVC 바인딩
	RunDatasetCache(ctx context.Context, ns, jobName, node, nfsServer, nfsDatasetPath, hostBase, name string) error
	RunDatasetUncache(ctx context.Context, ns, jobName, node, hostBase, name string) error
	BuildJobStatus(ctx context.Context, ns, name string) (string, error)
	BuildLogs(ctx context.Context, ns, jobName string, tail int64) string // 다운로드 진행률 파싱(PROGRESS 로그)
	DeleteBuildJob(ctx context.Context, ns, name string) error
}

// Service는 dataset 비즈니스 로직.
type Service struct {
	repo         Repository
	prov         Provisioner
	storageClass string // 데이터셋 PVC 스토리지클래스(폴백 동적 PVC용)
	namespace    string // 데이터셋 PVC 네임스페이스(예: <prefix>datasets)
	nfsServer    string // 데이터셋 정규 NFS 서버(설정 시 정규경로 모드)
	nfsBase      string // 데이터셋 정규 NFS 베이스(예: /export) — 데이터는 <base>/dataset/<name>
	cacheHost    string // 노드 로컬 캐시 루트(hostPath, 예: /dataset-cache)
	nfsMount     string // API 파드에 데이터셋 NFS 가 마운트된 로컬 경로(파일 업로드 직접 기록). 빈값=업로드 비활성.
	met          *metrics.Client // 다운로드 진행률(다운로드 Pod 수신 바이트) 조회용
}

// WithMetrics는 다운로드 진행률 산출용 Prometheus 클라이언트를 주입한다.
func (s *Service) WithMetrics(m *metrics.Client) *Service { s.met = m; return s }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// WithStorage는 데이터셋 저장 의존성을 주입한다. nfsServer/nfsBase 가 있으면 정규경로(/dataset/<name>) 모드.
func (s *Service) WithStorage(prov Provisioner, storageClass, namespace, nfsServer, nfsBase, cacheHost string) *Service {
	s.prov, s.storageClass, s.namespace = prov, storageClass, namespace
	s.nfsServer, s.nfsBase, s.cacheHost = nfsServer, nfsBase, cacheHost
	return s
}

// WithUploadMount는 파일 업로드용 로컬 NFS 마운트 경로를 주입한다(설정 시 zip/tar 직접 업로드 가능).
func (s *Service) WithUploadMount(path string) *Service { s.nfsMount = path; return s }

// UploadEnabled는 파일 직접 업로드 가능 여부(정규경로 모드 + NFS 로컬 마운트 존재).
func (s *Service) UploadEnabled() bool { return s.canonical() && s.nfsMount != "" }

// CacheHostPath는 노드 로컬 캐시 루트(세션 hostPath 마운트 산정용). 빈값=캐시 비활성.
func (s *Service) CacheHostPath() string { return s.cacheHost }

// DatasetCachePlacement는 세션 마운트 판정용으로 datasetID별 (cached 노드 목록, 노드로컬 경로)를 반환한다.
// 캐시 비활성(cacheHost 빈값)이면 빈 맵 → 세션은 전부 NFS 마운트. session.DatasetCacheReader 구현.
func (s *Service) DatasetCachePlacement(ids []int64) (map[int64][]string, map[int64]string) {
	if s.cacheHost == "" || len(ids) == 0 {
		return nil, nil
	}
	nodes := map[int64][]string{}
	hostPaths := map[int64]string{}
	for _, id := range ids {
		d, err := s.repo.Get(id)
		if err != nil {
			continue
		}
		nodes[id] = s.repo.CachedNodesOf(id)
		hostPaths[id] = s.cacheHost + "/" + d.Name
	}
	return nodes, hostPaths
}

// canonical은 정규 NFS 경로 모드 여부(서버+베이스 설정 시).
func (s *Service) canonical() bool { return s.prov != nil && s.nfsServer != "" && s.nfsBase != "" }

// pvcSizeGi는 PVC 용량(GiB) 산정 — 명시 GB 없으면 측정 바이트에서 올림 + 여유 1Gi.
func pvcSizeGi(d *Dataset) int {
	if d.SizeGB >= 1 {
		return d.SizeGB
	}
	return int(d.SizeBytes>>30) + 1
}

// ensureStorage는 (폴백) 동적 RWX NFS PVC(ds-<id>)를 멱등 생성한다. 정규경로 모드에선 미사용.
func (s *Service) ensureStorage(ctx context.Context, d *Dataset) {
	if s.prov == nil || d.PVCName != "" {
		return
	}
	name := fmt.Sprintf("ds-%d", d.ID)
	err := s.prov.CreatePVC(ctx, k8s.PVCSpec{
		Namespace: s.namespace, Name: name, SizeGiB: pvcSizeGi(d),
		StorageClass: s.storageClass, AccessMode: "RWX",
	})
	if err != nil {
		log.Printf("[dataset] PVC 생성 실패 ds %d: %v", d.ID, err)
		return
	}
	_ = s.repo.SetPVC(d.ID, name, s.namespace)
}

// RunReconciler는 적재중(loading) 데이터셋의 다운로드 Job 을 폴링해 완료/실패를 반영한다.
// 완료 시 정규경로(<base>/dataset/<name>)에 정적 NFS PVC(ds-<id>)를 바인딩하고 ready 로 전이한다.
func (s *Service) RunReconciler(ctx context.Context, interval time.Duration) {
	if !s.canonical() {
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	log.Printf("[dataset-reconciler] started (nfs=%s:%s/dataset)", s.nfsServer, s.nfsBase)
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

// parseExtracted는 해제 잡 로그에서 마지막 "EXTRACTED <bytes>"(실제 콘텐츠 크기)를 뽑는다.
func parseExtracted(logs string) int64 {
	v, _ := lastMarker(strings.Fields(logs), "EXTRACTED")
	return v
}

// parseHash는 해제 잡 로그에서 마지막 "HASH <값>" 을 뽑는다(없으면 "").
func parseHash(logs string) string {
	toks := strings.Fields(logs)
	hash := ""
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == "HASH" && toks[i+1] != "" {
			hash = toks[i+1]
		}
	}
	return hash
}

func (s *Service) reconcileOnce(ctx context.Context) {
	for _, d := range s.repo.ListLoading() {
		job := fmt.Sprintf("dl-ds-%d", d.ID)
		st, err := s.prov.BuildJobStatus(ctx, s.namespace, job)
		if err != nil || st == k8s.BuildRunning {
			continue
		}
		if st == k8s.BuildFailed {
			_ = s.repo.SetLoadStatus(d.ID, "failed") // Job 보존(로그 조회)
			log.Printf("[dataset] ds %d 적재 실패", d.ID)
			continue
		}
		// 성공 → 정규경로에 정적 NFS PVC 바인딩 + ready.
		pvc := fmt.Sprintf("ds-%d", d.ID)
		err = s.prov.EnsureSharedNFSPVC(ctx, k8s.SharedNFSSpec{
			Namespace: s.namespace, Name: pvc,
			NFSServer: s.nfsServer, NFSPath: s.nfsBase + "/dataset/" + d.Name, SizeGiB: pvcSizeGi(&d),
		})
		if err != nil {
			log.Printf("[dataset] ds %d PVC 바인딩 실패: %v", d.ID, err)
			continue
		}
		_ = s.repo.SetPVC(d.ID, pvc, s.namespace)
		// 해제 잡 로그에서 HASH(아카이브 sha256 16자) + EXTRACTED(실제 콘텐츠 크기)를 읽어 저장/교정. Job 삭제 전에.
		logs := s.prov.BuildLogs(ctx, s.namespace, job, 80)
		if d.Hash == "" {
			if h := parseHash(logs); h != "" {
				_ = s.repo.SetHash(d.ID, h)
			}
		}
		if sz := parseExtracted(logs); sz > 0 {
			_ = s.repo.SetSize(d.ID, sz) // 아카이브 추정 → 실제 해제 크기로 교정(용량 뻥튀기 해소)
		}
		_ = s.repo.SetLoadStatus(d.ID, "ready")
		_ = s.prov.DeleteBuildJob(ctx, s.namespace, job)
		log.Printf("[dataset] ds %d 적재 완료 → /dataset/%s", d.ID, d.Name)
	}
	// 노드 로컬 캐시 복사 Job 폴링 → cached/failed 전이.
	for _, c := range s.repo.ListCaching() {
		job := fmt.Sprintf("dc-%d-%s", c.DatasetID, c.Node)
		st, err := s.prov.BuildJobStatus(ctx, s.namespace, job)
		if err != nil || st == k8s.BuildRunning {
			continue
		}
		if st == k8s.BuildFailed {
			_ = s.repo.CacheUpsert(c.DatasetID, c.Node, "failed")
			log.Printf("[dataset] ds %d 노드 %s 캐시 실패", c.DatasetID, c.Node)
			continue
		}
		_ = s.repo.CacheUpsert(c.DatasetID, c.Node, "cached")
		_ = s.prov.DeleteBuildJob(ctx, s.namespace, job)
		log.Printf("[dataset] ds %d 노드 %s 로컬 캐시 완료", c.DatasetID, c.Node)
	}
}

// List는 전역 레지스트리 + 내 데이터셋/신청을 반환한다.
func (s *Service) List(ctx context.Context, userID int64) (*ListRes, error) {
	global, err := s.repo.GlobalActive()
	if err != nil {
		return nil, err
	}
	for i := range global { // 다운로드 중인 데이터셋은 진행률(%)·ETA·받은 용량 채움
		if global[i].LoadStatus == "loading" {
			global[i].Phase, global[i].Progress, global[i].EtaSec, global[i].Downloaded = s.loadProgress(ctx, global[i].ID, global[i].SizeBytes)
		}
		// 노드 로컬 캐시 복사 중이면 단계(복사/해제)+진행률(%)을 채운다.
		for j := range global[i].Caches {
			if global[i].Caches[j].Status == "caching" {
				global[i].Caches[j].Phase, global[i].Caches[j].Progress = s.cacheProgress(ctx, global[i].ID, global[i].Caches[j].Node)
			}
		}
	}
	mine, err := s.repo.Mine(userID)
	if err != nil {
		return nil, err
	}
	return &ListRes{Global: global, Mine: mine}, nil
}

// downloadProgress는 다운로드 Job 로그의 "PROGRESS <bytes>" 시계열(2초 간격)로 진행률·ETA·받은 용량을 산출한다.
// Prometheus 가 아니라 실제 기록된 파일 크기 기반이라 정밀하다. 로그/용량 미가용이면 0.
const progressIntervalSec = 2 // 다운로드 Job 의 PROGRESS 출력 간격(RunDatasetFetch 의 sleep 과 일치)

// lastMarker는 로그에서 "<key> <int>" 마지막 값을 찾는다(없으면 ok=false).
func lastMarker(toks []string, key string) (int64, bool) {
	var v int64
	found := false
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == key {
			if n, err := strconv.ParseInt(toks[i+1], 10, 64); err == nil {
				v, found = n, true
			}
		}
	}
	return v, found
}

// lastPair는 로그에서 "<key> <int> <int>" 마지막 쌍을 찾는다(해제 진행률 EXPROGRESS <ex> <total> 용).
func lastPair(toks []string, key string) (a, b int64, ok bool) {
	for i := 0; i+2 < len(toks); i++ {
		if toks[i] != key {
			continue
		}
		x, e1 := strconv.ParseInt(toks[i+1], 10, 64)
		y, e2 := strconv.ParseInt(toks[i+2], 10, 64)
		if e1 == nil && e2 == nil {
			a, b, ok = x, y, true
		}
	}
	return
}

func capPct(v int) int {
	if v > 99 {
		return 99
	}
	if v < 0 {
		return 0
	}
	return v
}

// loadProgress는 적재 Job 로그로 현재 단계(download|extract)와 그 단계의 진행률(%)을 산출한다.
//   다운로드: "PROGRESS <bytes>" / SizeBytes. 해제: "EXTOTAL"·"EXPROGRESS <bytes>". "EXTRACT DONE"=거의 완료.
func (s *Service) loadProgress(ctx context.Context, id int64, total int64) (phase string, pct, etaSec int, downloaded int64) {
	if s.prov == nil {
		return "", 0, 0, 0
	}
	logs := s.prov.BuildLogs(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", id), 40)
	if logs == "" {
		return "download", 0, 0, 0
	}
	toks := strings.Fields(logs)
	// 해제 단계 — EXPROGRESS <ex> <total> 로 판정(로그 tail 밖으로 EXTOTAL 이 밀려나도 견고).
	if strings.Contains(logs, "EXTRACT DONE") {
		return "extract", 99, 0, 0
	}
	if ex, total, ok := lastPair(toks, "EXPROGRESS"); ok {
		if total > 0 {
			return "extract", capPct(int(ex * 100 / total)), 0, 0
		}
		return "extract", 0, 0, 0
	}
	if strings.Contains(logs, "EXTRACT") || strings.Contains(logs, "EXTOTAL") { // 해제 시작(진행표본 전)
		return "extract", 0, 0, 0
	}
	// 다운로드 단계.
	if total <= 0 {
		return "download", 0, 0, 0
	}
	var vals []int64
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == "PROGRESS" {
			if n, err := strconv.ParseInt(toks[i+1], 10, 64); err == nil {
				vals = append(vals, n)
			}
		}
	}
	if len(vals) == 0 {
		return "download", 0, 0, 0
	}
	downloaded = vals[len(vals)-1]
	pct = capPct(int(downloaded * 100 / total))
	if len(vals) >= 2 {
		elapsed := float64(len(vals)-1) * progressIntervalSec
		if rate := float64(downloaded-vals[0]) / elapsed; rate > 0 {
			if etaSec = int(float64(total-downloaded) / rate); etaSec < 0 {
				etaSec = 0
			}
		}
	}
	return "download", pct, etaSec, downloaded
}

// cacheProgress는 노드 로컬 캐시 Job(dc-<id>-<node>) 로그로 단계(copy|extract)+진행률(%)을 산출한다.
//   복사: "PROGRESS <cur> <total>". 해제: "EXTOTAL"·"EXPROGRESS <bytes>". "EXTRACT DONE"=거의 완료.
func (s *Service) cacheProgress(ctx context.Context, id int64, node string) (phase string, pct int) {
	if s.prov == nil {
		return "", 0
	}
	logs := s.prov.BuildLogs(ctx, s.namespace, fmt.Sprintf("dc-%d-%s", id, node), 40)
	if logs == "" {
		return "copy", 0
	}
	toks := strings.Fields(logs)
	if strings.Contains(logs, "EXTRACT DONE") {
		return "extract", 99
	}
	if ex, total, ok := lastPair(toks, "EXPROGRESS"); ok { // 해제 단계
		if total > 0 {
			return "extract", capPct(int(ex * 100 / total))
		}
		return "extract", 0
	}
	if strings.Contains(logs, "EXTRACT") || strings.Contains(logs, "EXTOTAL") {
		return "extract", 0
	}
	// 복사 단계 — "PROGRESS <cur> <total>".
	var cur, total int64
	for i := 0; i+2 < len(toks); i++ {
		if toks[i] != "PROGRESS" {
			continue
		}
		c, e1 := strconv.ParseInt(toks[i+1], 10, 64)
		tt, e2 := strconv.ParseInt(toks[i+2], 10, 64)
		if e1 == nil && e2 == nil {
			cur, total = c, tt
		}
	}
	if total <= 0 {
		return "copy", 0
	}
	if cur > total {
		cur = total
	}
	return "copy", capPct(int(cur * 100 / total))
}

// Register는 업로드-등록 신청을 접수한다(승인 대기).
func (s *Service) Register(userID int64, req RegisterReq) error {
	scope := req.TargetScope
	if scope == "" {
		scope = ScopePersonal
	}
	if s.repo.NameTaken(req.Name) { // 정규경로 /dataset/<name> 충돌 방지 — 이름 유일
		return ErrNameTaken
	}
	// 등록은 즉시 끝내고(용량 측정으로 막지 않음), URL 용량은 백그라운드로 측정해 채운다(프론트는 "측정 중" 표시).
	r := &Request{
		Name: req.Name, RequesterUserID: userID, SizeClass: req.SizeClass,
		SizeGB: req.SizeGB, SizeBytes: 0, Hash: req.Hash, SourceURL: req.SourceURL, TargetScope: scope, Status: StatusPending,
	}
	if err := s.repo.CreateRequest(r); err != nil {
		return err
	}
	if r.SourceURL != "" {
		go func(id int64, url string) {
			if b := remoteSize(url); b > 0 {
				_ = s.repo.SetRequestSize(id, b)
			}
		}(r.ID, r.SourceURL)
	}
	return nil
}

// ErrUploadDisabled — NFS 마운트 미설정 등으로 파일 업로드 불가.
var ErrUploadDisabled = fmt.Errorf("upload disabled")


// validName은 데이터셋 이름이 NFS 경로로 안전한지 확인한다(경로 조작·구분자 차단).
func validName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("잘못된 데이터셋 이름")
	}
	return nil
}


// ── 등록 방식 ①: NFS 인박스 (관리자가 SCP로 복사한 파일을 등록) ──

// InboxDir은 관리자가 SCP로 데이터셋 아카이브를 올려두는 NFS 인박스 로컬 경로.
func (s *Service) inboxDir() string { return filepath.Join(s.nfsMount, "dataset-inbox") }

// InboxTarget은 관리자에게 안내할 SCP 대상(<nfsServer>:<nfsBase>/dataset-inbox/).
func (s *Service) InboxTarget() string {
	if s.nfsServer == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s/dataset-inbox/", s.nfsServer, strings.TrimRight(s.nfsBase, "/"))
}

// InboxFile — 인박스에 올라온 아카이브 1개.
type InboxFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// InboxList는 인박스 디렉터리의 아카이브 목록을 반환한다(관리자가 등록 대상으로 선택).
func (s *Service) InboxList() ([]InboxFile, error) {
	if !s.UploadEnabled() {
		return nil, ErrUploadDisabled
	}
	dir := s.inboxDir()
	_ = os.MkdirAll(dir, 0o777) // 최초 조회 시 생성해 안내 경로가 실재하게
	_ = os.Chmod(dir, 0o777)    // umask 로 0755 가 되면 SCP 하는 외부 계정이 못 쓴다 → 강제로 world-writable
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []InboxFile{}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, InboxFile{Name: e.Name(), Bytes: fi.Size()})
	}
	return out, nil
}

// RegisterNFS는 인박스의 파일을 데이터셋으로 등록한다 — dataset/<name>/ 로 이동 후 해제 Job.
func (s *Service) RegisterNFS(ctx context.Context, userID int64, name, scope, ownerName, filename, sizeClass string) error {
	if !s.UploadEnabled() {
		return ErrUploadDisabled
	}
	if err := validName(name); err != nil {
		return err
	}
	if s.repo.NameTaken(name) {
		return ErrNameTaken
	}
	src := filepath.Join(s.inboxDir(), safeArchiveName(filename))
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("인박스에 파일이 없습니다: %w", err)
	}
	dir := filepath.Join(s.nfsMount, "dataset", name)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	dst := filepath.Join(dir, safeArchiveName(filename))
	if err := os.Rename(src, dst); err != nil {
		// 교차 디바이스 등으로 rename 실패하면 복사 폴백.
		if cerr := copyFile(src, dst); cerr != nil {
			return fmt.Errorf("인박스 파일 이동 실패: %w", err)
		}
		_ = os.Remove(src)
	}
	return s.createLoadingAndExtract(ctx, userID, name, scope, ownerName, sizeClass, fi.Size())
}

// RegisterURL은 관리자가 URL(wget)로 데이터셋을 직접 등록한다(요청/승인 절차 없이).
func (s *Service) RegisterURL(ctx context.Context, userID int64, name, scope, ownerName, url, sizeClass string) error {
	if !s.canonical() {
		return ErrUploadDisabled
	}
	if err := validName(name); err != nil {
		return err
	}
	if s.repo.NameTaken(name) {
		return ErrNameTaken
	}
	if scope == "" {
		scope = ScopeGlobal
	}
	status := StatusActive
	if scope == ScopePersonal {
		status = StatusPrivate
	}
	d := &Dataset{Name: name, Scope: scope, Owner: ownerName, OwnerUserID: &userID, SourceURL: url, SizeClass: sizeClass, SizeBytes: remoteSize(url), Status: status, LoadStatus: "loading"}
	if err := s.repo.CreateDataset(d); err != nil {
		return err
	}
	if err := s.prov.RunDatasetFetch(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", d.ID), s.nfsServer, s.nfsBase, d.Name, url); err != nil {
		log.Printf("[dataset] URL 적재 Job 실패 ds %d: %v", d.ID, err)
		_ = s.repo.SetLoadStatus(d.ID, "failed")
	}
	return nil
}

// createLoadingAndExtract는 NFS 에 이미 놓인 아카이브를 loading 데이터셋으로 만들고 해제 Job 을 띄운다.
func (s *Service) createLoadingAndExtract(ctx context.Context, userID int64, name, scope, ownerName, sizeClass string, bytes int64) error {
	if scope == "" {
		scope = ScopeGlobal
	}
	status := StatusActive
	if scope == ScopePersonal {
		status = StatusPrivate
	}
	// 초기 용량 = 아카이브 크기(정직한 하한). 해제 완료 시 리컨실러가 실제 해제 콘텐츠 크기로 교정한다.
	d := &Dataset{Name: name, Scope: scope, Owner: ownerName, OwnerUserID: &userID, SizeClass: sizeClass, SizeBytes: bytes, Status: status, LoadStatus: "loading"}
	if err := s.repo.CreateDataset(d); err != nil {
		return err
	}
	if err := s.prov.RunDatasetExtract(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", d.ID), s.nfsServer, s.nfsBase, d.Name); err != nil {
		log.Printf("[dataset] 해제 Job 실패 ds %d: %v", d.ID, err)
		_ = s.repo.SetLoadStatus(d.ID, "failed")
	}
	return nil
}

// copyFile은 rename 폴백용 단순 복사.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// safeArchiveName은 업로드 파일명을 안전한 기본명으로 정리한다(경로 조작 방지).
// 허용 확장자(zip/tar/tar.gz/tgz)만; 그 외는 단일 파일로 그대로 저장(해제 잡이 스킵).
func safeArchiveName(fn string) string {
	base := filepath.Base(strings.ReplaceAll(fn, "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return "upload.bin"
	}
	return base
}

// Delete는 데이터셋을 삭제하며 노드 로컬 캐시(hostPath)와 NFS 데이터도 함께 정리한다.
// CachedByNode는 노드별 캐시 완료 데이터셋을 반환한다(세션 생성 UI 노드 선호 표시용).
func (s *Service) CachedByNode() []NodeCachedDataset { return s.repo.CachedDatasetsByNode() }

func (s *Service) Delete(ctx context.Context, id int64) error {
	d, err := s.repo.Get(id)
	if err == nil {
		// 처리 중이면 적재/해제 Job 을 먼저 죽인다(고아 Job 방지).
		if s.prov != nil {
			_ = s.prov.DeleteBuildJob(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", id))
		}
		// 캐시된/진행중 노드의 로컬 디렉터리 정리 + 캐시 Job 정리(best-effort).
		if s.prov != nil && s.cacheHost != "" {
			for _, node := range s.repo.CachedNodesOf(id) {
				_ = s.prov.DeleteBuildJob(ctx, s.namespace, fmt.Sprintf("dc-%d-%s", id, node)) // 복사 중이면 중단
				_ = s.prov.RunDatasetUncache(ctx, s.namespace, fmt.Sprintf("rm-dc-%d-%s", id, node), node, s.cacheHost, d.Name)
			}
		}
		_ = s.repo.CacheDeleteAll(id)
		// NFS 정규경로 데이터(dataset/<name>) 정리.
		if s.nfsMount != "" && d.Name != "" {
			_ = os.RemoveAll(filepath.Join(s.nfsMount, "dataset", d.Name))
		}
	}
	return s.repo.Delete(id)
}

// UpdateDescription은 데이터셋 설명을 수정한다(관리자).
func (s *Service) UpdateDescription(id int64, desc string) error { return s.repo.SetDescription(id, desc) }

// UpdateMeta는 설명 + 크기 클래스를 함께 저장한다(관리자 상세 편집). sizeClass 빈값이면 클래스 미변경.
func (s *Service) UpdateMeta(id int64, desc, sizeClass string) error {
	if err := s.repo.SetDescription(id, desc); err != nil {
		return err
	}
	if sizeClass != "" {
		return s.repo.SetSizeClass(id, sizeClass)
	}
	return nil
}

func (s *Service) PendingRequests() ([]Request, error) { return s.repo.ListPendingRequests() }

// Approve는 신청을 승인하고 데이터셋 레지스트리에 등록한 뒤 RWX NFS PVC 를 프로비저닝한다.
func (s *Service) Approve(ctx context.Context, reqID, reviewer int64, ownerName string) error {
	req, err := s.repo.GetRequest(reqID)
	if err != nil {
		return err
	}
	status, owner := StatusActive, ownerName
	if req.TargetScope == ScopePersonal {
		status = StatusPrivate
	}
	bytes := req.SizeBytes // 등록 시 백그라운드 측정값. 아직 0이면 승인 시점에 동기 측정(폴백).
	if bytes == 0 && req.SourceURL != "" {
		bytes = remoteSize(req.SourceURL)
	}
	d := &Dataset{
		Name: req.Name, Scope: req.TargetScope, Owner: owner, OwnerUserID: &req.RequesterUserID,
		SizeClass: req.SizeClass, SizeGB: req.SizeGB, SizeBytes: bytes, Hash: req.Hash,
		SourceURL: req.SourceURL, Status: status, LoadStatus: "ready",
	}
	if s.canonical() {
		// 정규경로 모드: NFS <base>/dataset/<name> 로 먼저 적재(loading) → 리컨실러가 완료 시 PVC 바인딩+ready.
		d.LoadStatus = "loading"
		if err := s.repo.CreateDataset(d); err != nil {
			return err
		}
		if err := s.prov.RunDatasetFetch(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", d.ID), s.nfsServer, s.nfsBase, d.Name, d.SourceURL); err != nil {
			log.Printf("[dataset] 적재 Job 실패 ds %d: %v", d.ID, err)
			_ = s.repo.SetLoadStatus(d.ID, "failed")
		}
		return s.repo.MarkRequest(reqID, reviewer, "approved")
	}
	// 폴백(정규 NFS 미설정): 동적 빈 PVC 만 생성.
	if err := s.repo.CreateDataset(d); err != nil {
		return err
	}
	s.ensureStorage(ctx, d)
	return s.repo.MarkRequest(reqID, reviewer, "approved")
}

func (s *Service) Reject(reqID, reviewer int64) error {
	return s.repo.MarkRequest(reqID, reviewer, "rejected")
}

// ToggleCache는 노드 로컬 캐시를 토글한다.
//   - 이미 행 있으면 해제: 행 삭제 + 노드 로컬 디렉터리 정리 Job(best-effort).
//   - 없으면 캐시 시작: status=caching 기록 + NFS→노드 로컬(hostPath) 복사 Job. 리컨실러가 cached/failed 로 전이.
func (s *Service) ToggleCache(ctx context.Context, datasetID int64, node string) error {
	d, err := s.repo.Get(datasetID)
	if err != nil {
		return err
	}
	jobBase := fmt.Sprintf("dc-%d-%s", datasetID, node)
	// 등록중(loading) 데이터셋은 캐시할 수 없다 — NFS 해제가 끝나야 노드로 복사 가능.
	// 단 이미 캐시 행이 있으면(해제/취소) 통과시킨다.
	if _, exists := s.repo.CacheStatus(datasetID, node); !exists && d.LoadStatus != "ready" {
		return fmt.Errorf("등록이 완료된 뒤에 캐시할 수 있습니다(현재 %s)", d.LoadStatus)
	}
	if _, exists := s.repo.CacheStatus(datasetID, node); exists {
		_ = s.repo.CacheDelete(datasetID, node)
		if s.prov != nil && s.cacheHost != "" {
			_ = s.prov.RunDatasetUncache(ctx, s.namespace, "rm-"+jobBase, node, s.cacheHost, d.Name)
		}
		return nil
	}
	if s.prov == nil || s.cacheHost == "" || !s.canonical() {
		return fmt.Errorf("노드 로컬 캐시 비활성(NFS/캐시 경로 미설정)")
	}
	if err := s.repo.CacheUpsert(datasetID, node, "caching"); err != nil {
		return err
	}
	return s.prov.RunDatasetCache(ctx, s.namespace, jobBase, node, s.nfsServer, s.nfsBase+"/dataset/"+d.Name, s.cacheHost, d.Name)
}
