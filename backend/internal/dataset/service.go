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
			global[i].Progress, global[i].EtaSec, global[i].Downloaded = s.downloadProgress(ctx, global[i].ID, global[i].SizeBytes)
		}
		// 노드 로컬 캐시 복사 중이면 복사 진행률(%)도 채운다("진행중.." 대신 실측 %).
		for j := range global[i].Caches {
			if global[i].Caches[j].Status == "caching" {
				global[i].Caches[j].Progress = s.cacheProgress(ctx, global[i].ID, global[i].Caches[j].Node)
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

func (s *Service) downloadProgress(ctx context.Context, id int64, total int64) (pct, etaSec int, downloaded int64) {
	if s.prov == nil || total <= 0 {
		return 0, 0, 0
	}
	logs := s.prov.BuildLogs(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", id), 20)
	if logs == "" {
		return 0, 0, 0
	}
	// "PROGRESS <n>" 값들을 시간순으로 수집.
	var vals []int64
	toks := strings.Fields(logs)
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == "PROGRESS" {
			if n, err := strconv.ParseInt(toks[i+1], 10, 64); err == nil {
				vals = append(vals, n)
			}
		}
	}
	if len(vals) == 0 {
		return 0, 0, 0
	}
	downloaded = vals[len(vals)-1]
	pct = int(downloaded * 100 / total)
	if pct > 99 {
		pct = 99
	}
	if pct < 0 {
		pct = 0
	}
	// 속도 = (마지막-처음)/경과시간 → ETA = 남은바이트/속도. 표본 2개 이상일 때만.
	if len(vals) >= 2 {
		elapsed := float64(len(vals)-1) * progressIntervalSec
		rate := float64(downloaded-vals[0]) / elapsed // bytes/s
		if rate > 0 {
			etaSec = int(float64(total-downloaded) / rate)
			if etaSec < 0 {
				etaSec = 0
			}
		}
	}
	return pct, etaSec, downloaded
}

// cacheProgress는 노드 로컬 캐시 복사 Job(dc-<id>-<node>) 로그의 "PROGRESS <cur> <total>" 로
// 복사 진행률(%)을 낸다. 복사가 끝나 로컬 해제(EXTRACT) 단계면 97%로 표시(해제 진행은 측정 불가).
// 로그/총량 미가용이면 0. (다운로드 진행률 downloadProgress 와 형제 함수)
func (s *Service) cacheProgress(ctx context.Context, id int64, node string) int {
	if s.prov == nil {
		return 0
	}
	logs := s.prov.BuildLogs(ctx, s.namespace, fmt.Sprintf("dc-%d-%s", id, node), 20)
	if logs == "" {
		return 0
	}
	if strings.Contains(logs, "EXTRACT") { // 복사 완료 → 로컬 해제 중
		return 97
	}
	var cur, total int64
	toks := strings.Fields(logs)
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
		return 0
	}
	if cur > total {
		cur = total
	}
	pct := int(cur * 100 / total)
	if pct > 96 {
		pct = 96 // 복사 100% 도달해도 해제 전이므로 EXTRACT 전까진 96 상한
	}
	return pct
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

// ErrChunkOffset — 청크 offset 이 서버 현재 크기와 불일치(클라이언트가 반환된 offset 으로 재동기화해야 함).
var ErrChunkOffset = fmt.Errorf("chunk offset mismatch")

// validName은 데이터셋 이름이 NFS 경로로 안전한지 확인한다(경로 조작·구분자 차단).
func validName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("잘못된 데이터셋 이름")
	}
	return nil
}

// partPath는 업로드 중 아카이브의 NFS 경로(<mount>/dataset/<name>/<archive>)를 만든다.
func (s *Service) partPath(name, filename string) string {
	return filepath.Join(s.nfsMount, "dataset", name, safeArchiveName(filename))
}

// UploadInit은 청크 업로드를 시작(또는 재개)한다. 디렉터리/파트 파일을 준비하고 현재 크기(=재개 offset)를 반환한다.
// Cloudflare 100MB 리밋을 청크로 우회하고, 파트 파일 크기 자체가 재개 지점이라 새로고침 후에도 이어올릴 수 있다.
func (s *Service) UploadInit(name, filename string) (int64, error) {
	if !s.UploadEnabled() {
		return 0, ErrUploadDisabled
	}
	if err := validName(name); err != nil {
		return 0, err
	}
	if s.repo.NameTaken(name) {
		return 0, ErrNameTaken
	}
	dir := filepath.Join(s.nfsMount, "dataset", name)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return 0, fmt.Errorf("데이터셋 디렉터리 생성 실패: %w", err)
	}
	p := s.partPath(name, filename)
	if fi, err := os.Stat(p); err == nil {
		return fi.Size(), nil // 재개: 이미 올라온 만큼 이어서
	}
	f, err := os.Create(p)
	if err != nil {
		return 0, fmt.Errorf("업로드 파일 생성 실패: %w", err)
	}
	_ = f.Close()
	return 0, nil
}

// UploadChunk는 offset 위치에 청크를 이어붙인다. offset 이 서버 현재 크기와 다르면 ErrChunkOffset 과
// 함께 실제 크기를 돌려줘 클라이언트가 그 지점부터 재전송하게 한다. 성공 시 새 크기를 반환한다.
func (s *Service) UploadChunk(name, filename string, offset int64, r io.Reader) (int64, error) {
	if !s.UploadEnabled() {
		return 0, ErrUploadDisabled
	}
	if err := validName(name); err != nil {
		return 0, err
	}
	p := s.partPath(name, filename)
	f, err := os.OpenFile(p, os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if offset != fi.Size() {
		return fi.Size(), ErrChunkOffset // 클라이언트가 반환된 크기부터 재전송
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fi.Size(), err
	}
	n, err := io.Copy(f, r)
	if err != nil {
		return offset + n, err
	}
	return offset + n, nil
}

// UploadStatus는 현재까지 올라온 바이트(파트 파일 크기)를 반환한다(재개 지점 조회). 없으면 0.
func (s *Service) UploadStatus(name, filename string) int64 {
	if err := validName(name); err != nil {
		return 0
	}
	fi, err := os.Stat(s.partPath(name, filename))
	if err != nil {
		return 0
	}
	return fi.Size()
}

// UploadFinish는 청크 전송이 끝난 파트 파일을 데이터셋으로 확정한다 — 크기 검증 후 레코드 생성(loading) +
// 해제 Job(dl-ds-<id>). 이후 리컨실러가 PVC 바인딩+ready.
func (s *Service) UploadFinish(ctx context.Context, userID int64, name, scope, ownerName, filename string, size int64) error {
	if !s.UploadEnabled() {
		return ErrUploadDisabled
	}
	if err := validName(name); err != nil {
		return err
	}
	if s.repo.NameTaken(name) {
		return ErrNameTaken
	}
	p := s.partPath(name, filename)
	fi, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("업로드 파일이 없습니다: %w", err)
	}
	if size > 0 && fi.Size() != size {
		return fmt.Errorf("불완전한 업로드입니다(%d/%d 바이트)", fi.Size(), size)
	}
	if scope == "" {
		scope = ScopeGlobal
	}
	status := StatusActive
	if scope == ScopePersonal {
		status = StatusPrivate
	}
	d := &Dataset{
		Name: name, Scope: scope, Owner: ownerName, OwnerUserID: &userID,
		SizeBytes: fi.Size() * 3, Status: status, LoadStatus: "loading",
	}
	if err := s.repo.CreateDataset(d); err != nil {
		return err
	}
	if err := s.prov.RunDatasetExtract(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", d.ID), s.nfsServer, s.nfsBase, d.Name); err != nil {
		log.Printf("[dataset] 업로드 해제 Job 실패 ds %d: %v", d.ID, err)
		_ = s.repo.SetLoadStatus(d.ID, "failed")
	}
	return nil
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

// Upload는 최고관리자가 zip/tar 아카이브(또는 단일 파일)를 직접 업로드해 데이터셋으로 등록한다.
// API 가 NFS(<mount>/dataset/<name>)에 스트리밍 기록 → 해제 Job(dl-ds-<id>) → 리컨실러가 PVC 바인딩+ready.
// 업로드는 승인 절차 없이 즉시 정규경로 데이터셋을 만든다(최고관리자 전용 라우트에서만 호출).
func (s *Service) Upload(ctx context.Context, userID int64, name, scope, ownerName, filename string, size int64, src io.Reader) error {
	if !s.UploadEnabled() {
		return fmt.Errorf("파일 업로드가 비활성화되어 있습니다(데이터셋 NFS 마운트 필요)")
	}
	if scope == "" {
		scope = ScopeGlobal
	}
	if s.repo.NameTaken(name) {
		return ErrNameTaken
	}
	// 1) NFS 에 스트리밍 기록: <mount>/dataset/<name>/<archive>
	dir := filepath.Join(s.nfsMount, "dataset", name)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return fmt.Errorf("데이터셋 디렉터리 생성 실패: %w", err)
	}
	arc := safeArchiveName(filename)
	dst := filepath.Join(dir, arc)
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("업로드 파일 생성 실패: %w", err)
	}
	written, werr := io.Copy(f, src)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("업로드 저장 실패: %w", werr)
	}
	if cerr != nil {
		return fmt.Errorf("업로드 파일 마감 실패: %w", cerr)
	}
	// 2) 데이터셋 레코드(정규경로, loading). PVC 크기는 아카이브의 3배로 넉넉히 잡되(NFS 정적 PVC 는 미강제),
	//    상태 정확성보다 여유가 낫다.
	status := StatusActive
	if scope == ScopePersonal {
		status = StatusPrivate
	}
	d := &Dataset{
		Name: name, Scope: scope, Owner: ownerName, OwnerUserID: &userID,
		SizeBytes: written * 3, Status: status, LoadStatus: "loading",
	}
	if err := s.repo.CreateDataset(d); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// 3) 해제 Job(다운로드 없이 제자리 해제). 리컨실러가 dl-ds-<id> 완료를 보고 PVC 바인딩+ready.
	if err := s.prov.RunDatasetExtract(ctx, s.namespace, fmt.Sprintf("dl-ds-%d", d.ID), s.nfsServer, s.nfsBase, d.Name); err != nil {
		log.Printf("[dataset] 업로드 해제 Job 실패 ds %d: %v", d.ID, err)
		_ = s.repo.SetLoadStatus(d.ID, "failed")
	}
	return nil
}

func (s *Service) Delete(id int64) error { return s.repo.Delete(id) }

// UpdateDescription은 데이터셋 설명을 수정한다(관리자).
func (s *Service) UpdateDescription(id int64, desc string) error { return s.repo.SetDescription(id, desc) }

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
