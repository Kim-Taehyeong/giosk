// Command api는 Giosk API 서버 진입점이다. 얇게 유지: 설정→DB→마이그레이션→와이어링→실행.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"giosk/internal/alert"
	"giosk/internal/alertlog"
	"giosk/internal/announcement"
	"giosk/internal/audit"
	"giosk/internal/auth"
	"giosk/internal/billing"
	"giosk/internal/config"
	"giosk/internal/dashboard"
	"giosk/internal/dataset"
	"giosk/internal/group"
	"giosk/internal/image"
	"giosk/internal/k8s"
	"giosk/internal/metrics"
	"giosk/internal/node"
	"giosk/internal/notify"
	"giosk/internal/org"
	"giosk/internal/policy"
	"giosk/internal/resource"
	"giosk/internal/server"
	"giosk/internal/session"
	"giosk/internal/store"
	"giosk/internal/systemconfig"
	"giosk/internal/topup"
	"giosk/internal/userdetail"
	"giosk/internal/usernotify"
	"giosk/internal/users"
	"giosk/internal/volume"
	"giosk/internal/wallet"
)

const sessionTTL = 7 * 24 * time.Hour

func main() {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := store.Open(cfg.Database)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := store.SeedDefaults(db); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("db connected & migrated & seeded (%s)", cfg.Database.Name)

	// 도메인 조립(수동 DI). 도메인이 늘면 여기서만 추가.
	authSvc := auth.NewService(auth.NewRepository(db), sessionTTL)
	if err := authSvc.EnsureAdmin(cfg.Admin.Username, cfg.Admin.Password); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	groupSvc := group.NewService(group.NewRepository(db))
	authSvc.WithEnricher(groupSvc) // /me·로그인 응답에 콘솔 라우팅 컨텍스트 주입

	// K8s 클라이언트(미가용이면 nil — gpu-types 등은 빈 결과 반환).
	kc, _ := k8s.New(cfg.K8s.Kubeconfig, cfg.K8s.GpuTypeLabel)
	kc.WithPhysicalLabel(cfg.PhysicalNodes.Label) // 물리노드 식별 라벨(멀티 인스턴스 격리)
	kc.WithCudaLabel(cfg.K8s.CudaLabel)           // 노드 CUDA(드라이버) 버전 라벨(GFD)
	if kc.Available() {
		log.Printf("k8s connected (gpu-type label=%s)", cfg.K8s.GpuTypeLabel)
	} else {
		log.Printf("k8s unavailable — cluster 기능은 빈 결과/503")
	}

	resourceSvc := resource.NewService(resource.NewRepository(db), kc, resource.NewSessionCounter(db))
	sessionSvc := session.NewService(session.NewRepository(db), kc,
		cfg.K8s.NamespacePrefix, cfg.K8s.GatewayDomain, cfg.Storage.PersistenceClass)
	authSvc.WithKeySync(sessionSvc) // SSH 공개키 등록/교체 → 실행 중 컨테이너 세션 authorized_keys 즉시 반영

	orgRepo := org.NewRepository(db)
	orgSvc := org.NewService(orgRepo).WithBootstrapper(groupSvc)              // 조직 생성 시 기본 그룹+org_admin 배정
	walletSvc := wallet.NewService(wallet.NewRepository(db)).WithPool(orgSvc) // 할당 시 조직 풀에서 차감(enforce)
	groupSvc.WithWallet(walletSvc)                                            // 팀 생성 시 초기 크레딧/리필 세팅(크레딧 모드)
	topupSvc := topup.NewService(topup.NewRepository(db), crediter{wallet: walletSvc, orgRepo: orgRepo})
	met := metrics.New(cfg.PrometheusURL)
	// 스토리지 단일화: 볼륨은 NFS(RWX) 클래스로 프로비저닝 → 교차-ns 공유·물리노드 직접 마운트 일관.
	volumeSvc := volume.NewService(volume.NewRepository(db), kc,
		cfg.Storage.NFSClass, cfg.K8s.NamespacePrefix, cfg.Storage.VolumeQuotaGB)
	volumeSvc.WithMetrics(met)
	volumeSvc.WithLocalHome(cfg.PhysicalNodes.Enabled) // 물리 활성 시 로컬 Home 특수 볼륨 노출
	nodeSvc := node.NewService(node.NewRepository(db), kc, met,
		cfg.PhysicalNodes.NFS.Server, cfg.PhysicalNodes.NFS.Path).
		WithScratch(cfg.Storage.ScratchHostPath).                                            // 물리노드 스크래치 루트(노드별 활성은 DB)
		WithUIDBase(cfg.PhysicalNodes.UIDBase).                                              // 전역 안정 UID 베이스(재사용 방지)
		WithLocalHomeHost(cfg.Storage.PhysicalHomeHost).                                     // 물리 SSH 로컬 home 루트(/home/giosk)
		WithFreeMode(cfg.IsFree()).                                                          // 자유 모드: 임대 영속·계정 재사용·동시접속
		WithDevicePluginConfig(cfg.K8s.DevicePluginConfigNS, cfg.K8s.DevicePluginConfigName). // 타임셰어링 웹 설정 → device plugin 즉시 반영
		WithPhysicalLabel(cfg.PhysicalNodes.Label)                                           // 물리 임대 토글이 이 라벨을 k8s 노드에 적용
	cfgStore := systemconfig.NewStore(db) // 런타임 설정(유휴·기능 토글·전역 상한) 저장소
	platIntervalFn := func() int { return cfgStore.IntOr(systemconfig.KeyRechargeIntervalDays, 30) }
	orgSvc.WithPlatformInterval(platIntervalFn)    // 조직 리필 주기 캡(플랫폼 기본)
	walletSvc.WithPlatformInterval(platIntervalFn) // 팀/개인 리필 주기 캡 최상단

	// 하드 정책(크레딧 무관 절대 상한) — 계층 해석: 개인→그룹→조직→전역.
	// 전역은 설치 기본(config.Quota) 위에 런타임 오버라이드를 얹어 호출 시점마다 읽는다
	// (정책 탭에서 바꾸면 재시작 없이 즉시 반영).
	policyRepo := policy.NewRepository(db)
	globalQuota := func() policy.Resolved {
		return policy.Resolved{
			MaxGpu:                cfgStore.IntOr(systemconfig.KeyQuotaMaxGpu, cfg.Quota.MaxGpuCount),
			MaxVramGB:             cfgStore.IntOr(systemconfig.KeyQuotaMaxVramGB, cfg.Quota.MaxVramGB),
			MaxVolumeGiB:          cfgStore.IntOr(systemconfig.KeyQuotaMaxVolGiB, cfg.Quota.VolumeQuotaGB),
			MaxConcurrentSessions: cfgStore.IntOr(systemconfig.KeyQuotaMaxSessions, cfg.Quota.MaxConcurrentSessions),
			MaxEphemeralGiB:       cfg.Quota.MaxEphemeralGiB, // 전역 폴백(런타임 오버라이드 키 없음 — 설치값 사용)
		}
	}
	limitResolver := policy.NewResolver(policyRepo, orgSvc, globalQuota)
	volumeSvc.WithLimits(limitResolver)
	dashboardSvc := dashboard.NewService(dashboard.NewRepository(db), met, kc, cfg)
	dashboardSvc.WithLimits(limitResolver) // 동시세션 KPI = 정책값(정책 일원화)
	auditRepo := audit.NewRepository(db)
	sessionSvc.WithAudit(auditRepo)                                                           // 세션 로그 탭 = 감사 로그
	sessionSvc.WithMetrics(met)                                                               // 유휴 판정용
	sessionSvc.WithLimits(limitResolver)                                                      // 동시세션·GPU·VRAM 상한 = 정책(quota) 일원화
	sessionSvc.WithLeaser(nodeSvc)                                                            // 물리(SSH) 세션 임대
	sessionSvc.WithExpose(cfg.K8s.SessionExpose)                                              // 웹 노출 모드
	sessionSvc.WithGateway(cfg.K8s.GatewaySecret, cfg.K8s.GatewayScheme, cfg.K8s.GatewayHost, // 접속 게이트웨이(단기 토큰)
		cfg.K8s.GatewaySSHPort, cfg.K8s.SessionSSHDImage, cfg.K8s.SessionSSHDPubKey)
	sessionSvc.WithGatewayProxyJump(cfg.K8s.GatewayJump)                                             // 외부 접속용 -J 점프 호스트(빈값=미표시)
	sessionSvc.WithGatewaySSHKey([]byte(cfg.K8s.GatewaySSHKey))                                      // 물리 세션 웹터미널 SSH 관리키(빈값=물리 웹터미널 비활성)
	sessionSvc.WithScratch(cfg.Storage.ScratchEnabled, cfg.Storage.ScratchHostPath)                  // 노드로컬 스크래치
	sessionSvc.WithLocalHome(cfg.PhysicalNodes.Enabled, cfg.Storage.PhysicalHomeHost)                // 로컬 Home 특수 볼륨(hostPath+노드핀)
	sessionSvc.WithUIDBase(cfg.PhysicalNodes.UIDBase)                                                // 컨테이너 안정 UID(물리 SSH 와 동일) → NFS 권한 일관
	sessionSvc.WithSharedHome(cfg.Storage.SharedHome)                                                // 영속 home(~/nfs) 사용 여부(설치시 고정)
	sessionSvc.WithSurge(cfg.Billing.Credit.Pricing == "dynamic", cfg.Billing.Credit.SurgeIncrement, // 동적/서지 가격
		func(ctx context.Context, gt string) (int, int) {
			for _, t := range resourceSvc.Availability(ctx).ByType {
				if t.GpuType == gt {
					return t.Free, t.Total
				}
			}
			return 0, 0
		})
	imageRepo := image.NewRepository(db)
	imageSvc := image.NewService(imageRepo, kc, cfg.K8s.Registry, cfg.K8s.NamespacePrefix+"build"). // 이미지 빌드(Kaniko)
													WithCosign(cfg.K8s.CosignKeySecret) // 빌드 후 trivy 스캔 + (키 있으면) cosign 서명
	go imageSvc.RunReconciler(context.Background(), 15*time.Second) // 빌드 Job 상태 → DB
	go sessionSvc.RunIdleReaper(context.Background(), func() int {  // 유휴 타임아웃은 운영 중 변경 가능 → 라이브 read
		return cfgStore.IntOr(systemconfig.KeyIdleTimeoutMin, cfg.IdleTimeoutMin)
	})
	go sessionSvc.RunPhaseReconciler(context.Background(), 30*time.Second)                     // 라이브 phase → DB(대시보드/관제 최신화)
	go dashboardSvc.RunSampler(context.Background(), time.Minute, sessionSvc.CountIdleRunning) // 인프라 메트릭 스냅샷(감시 시계열)
	if cfg.IsCredit() {
		sessionSvc.WithCharger(walletSvc) // 크레딧 소비 회계
		go sessionSvc.RunBiller(context.Background(), time.Minute)
		volumeSvc.WithCharger(walletSvc, func() int { // 스토리지 GiB·월 과금(런타임 단가 라이브 read)
			return cfgStore.IntOr(systemconfig.KeyStoragePriceGiBMonth, cfg.Storage.PricePerGiBMonth)
		})
		go volumeSvc.RunStorageBiller(context.Background(), time.Minute)
		// 크레딧 정기 재충전(런타임 설정 라이브 read) — 매시 도래 지갑 리필.
		go walletSvc.RunCreditRecharge(context.Background(), time.Hour, func() wallet.RechargeCfg {
			return wallet.RechargeCfg{
				Enabled:      cfgStore.Get(systemconfig.KeyRechargeEnabled) == "true",
				IntervalDays: cfgStore.IntOr(systemconfig.KeyRechargeIntervalDays, 30),
				Carryover:    cfgStore.Get(systemconfig.KeyRechargeCarryover) == "true",
			}
		})
	}
	if cfg.Billing.Mode == config.BillingDynamic {
		sessionSvc.WithDynamicLease(cfg.Billing.Dynamic.MaxLeaseHours, cfg.Billing.Dynamic.ExtensionHours, cfg.Billing.Dynamic.MaxExtensions) // 선착순 임대(연장·만료)
		go sessionSvc.RunLeaseReaper(context.Background(), time.Minute)                                                                       // 임대 만료 자동 정지
	}

	// 알림 엔진 — 관리자 규칙(gpu_util/gpu_temp/node_down)을 평가해 위반 시 웹훅·이메일 발송.
	notifyRepo := notify.NewRepository(db)
	var mailer notify.Mailer
	if cfg.SMTPEnabled() {
		mailer = &notify.SMTPMailer{Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, From: cfg.SMTP.From}
	}
	nodesDown := func(ctx context.Context) (int, bool) {
		nodes, err := kc.ListNodes(ctx)
		if err != nil {
			return 0, false
		}
		down := 0
		for _, n := range nodes {
			if !n.Ready {
				down++
			}
		}
		return down, true
	}
	alertStore := alertlog.New(db) // 발화 경고 이력(감시월 통합 피드) — notify 적재, dashboard 조회
	dashboardSvc.WithAlertStore(alertStore)

	// 사용자 인앱 알림 수신함 + 사용자 규칙 평가기(userMetric).
	// credit_balance(잔액)는 DB(user_wallets)로 바로 평가한다. 나머지 사용자 지표는
	// 실측(Prometheus/PVC 매핑)이 필요해 여기선 미지원(ok=false → 발화 안 함) — 오탐을 만들지 않는다.
	inboxStore := usernotify.New(db)
	userMetric := func(ctx context.Context, metric string) (map[int64]float64, bool) {
		switch metric {
		case "credit_balance":
			// per-membership 지갑(유저×팀)이라 유저당 행이 여러 개다. 유저 단위로 합산해야
			// 한 유저의 빈 팀(0) 행이 다른 팀 잔액을 덮어써 잘못된 "0" 알림을 반복 발화하지 않는다.
			// (팀별 저잔액은 세션 과금이 그 팀 지갑을 소진하면 RunBiller 가 직접 중단시킨다.)
			var rows []struct {
				UserID  int64
				Balance int
			}
			if err := db.Raw(`SELECT user_id, COALESCE(SUM(balance),0) AS balance FROM user_wallets GROUP BY user_id`).Scan(&rows).Error; err != nil {
				return nil, false
			}
			out := make(map[int64]float64, len(rows))
			for _, r := range rows {
				out[r.UserID] = float64(r.Balance)
			}
			return out, true
		default:
			return nil, false
		}
	}
	// credit_balance 를 팀별로 평가 — 크레딧이 팀 지갑에 귀속되므로 "어느 팀" 잔액이 낮은지 발화한다.
	// Active=그 팀에 활성 세션이 있거나 소비 이력이 있으면 true(안 쓰는 빈 팀은 알림 억제 → 스팸 방지).
	creditTeams := func(ctx context.Context) map[int64][]notify.TeamBalance {
		var rows []struct {
			UserID  int64
			GroupID int64
			Name    string
			Balance int
			Active  bool
		}
		err := db.Raw(`
			SELECT uw.user_id, uw.group_id, g.display_name AS name, uw.balance,
			  (EXISTS(SELECT 1 FROM sessions s WHERE s.user_id=uw.user_id AND s.group_id=uw.group_id AND s.phase IN ('provisioning','running'))
			   OR EXISTS(SELECT 1 FROM memberships m WHERE m.user_id=uw.user_id AND m.group_id=uw.group_id AND m.consumed>0)) AS active
			FROM user_wallets uw JOIN `+"`groups`"+` g ON g.id=uw.group_id
			WHERE uw.group_id>0 AND EXISTS(SELECT 1 FROM memberships m WHERE m.user_id=uw.user_id AND m.group_id=uw.group_id AND m.status='active')`).Scan(&rows).Error
		if err != nil {
			return nil
		}
		out := map[int64][]notify.TeamBalance{}
		for _, r := range rows {
			out[r.UserID] = append(out[r.UserID], notify.TeamBalance{GroupID: r.GroupID, Name: r.Name, Balance: r.Balance, Active: r.Active})
		}
		return out
	}
	go notify.NewEngine(notifyRepo, met, nodesDown, mailer).
		WithRecorder(alertStore).
		WithUserAlerts(inboxStore, userMetric).
		WithSessionMetric(sessionSvc.SessionMetric). // 세션 단위 알림(session_gpu/cpu/vram)
		WithCreditTeams(creditTeams).                // 팀별 크레딧 알림
		Run(context.Background(), time.Minute)

	// 데이터셋 — 정규 NFS 경로(<base>/dataset/<name>) 적재 + 리컨실러(다운로드 완료 시 PVC 바인딩).
	datasetSvc := dataset.NewService(dataset.NewRepository(db)).
		WithStorage(kc, cfg.Storage.NFSClass, cfg.K8s.NamespacePrefix+"datasets",
					cfg.Storage.Datasets.NFS.Server, cfg.Storage.Datasets.NFS.Path, cfg.Storage.DatasetCacheHost).
		WithUploadMount(cfg.Storage.Datasets.LocalMount). // zip/tar 직접 업로드(설정 시 API 가 NFS 에 직접 기록)
		WithMetrics(met) // 다운로드 진행률(%)
	go datasetSvc.RunReconciler(context.Background(), 10*time.Second)
	sessionSvc.WithDatasetCache(datasetSvc) // 세션이 캐시된 노드선 데이터셋을 hostPath 로 마운트
	// 세션 생성 UI: 노드별 "캐시된 데이터셋" 표시(dataset_node_cache). 노드-선호 선택용.
	// node.Cached(물리노드 뷰) + resource.byNode.cached(세션 가용노드 = 컨테이너 GPU 노드) 둘 다 채운다.
	nodeSvc.WithCachedDatasets(func() map[string][]node.CachedDataset {
		out := map[string][]node.CachedDataset{}
		for _, c := range datasetSvc.CachedByNode() {
			out[c.Node] = append(out[c.Node], node.CachedDataset{
				Name: c.Name, SizeClass: c.SizeClass, SizeGb: c.SizeGB, Owner: c.Owner, Desc: c.Desc,
			})
		}
		return out
	})
	resourceSvc.WithCachedDatasets(func() map[string][]resource.CachedDS {
		out := map[string][]resource.CachedDS{}
		for _, c := range datasetSvc.CachedByNode() {
			out[c.Node] = append(out[c.Node], resource.CachedDS{Name: c.Name, SizeClass: c.SizeClass, SizeGb: c.SizeGB})
		}
		return out
	})

	// 사용자 360 상세 — 각 도메인의 기존 userID-키 서비스를 클로저로 모아 한 응답으로.
	usersRepo := users.NewRepository(db)
	billingRepo := billing.NewRepository(db)
	userDetailH := userdetail.NewHandler(userdetail.Providers{
		User: func(id int64) (any, error) {
			u, e := usersRepo.ByID(id)
			if u == nil {
				return nil, e
			}
			return u, e
		},
		Wallet:   func(id int64) (any, error) { return walletSvc.MyWallet(id, 0) },
		Volumes:  func(id int64) (any, error) { return volumeSvc.List(id) },
		Sessions: func(ctx context.Context, id int64) (any, error) { return sessionSvc.List(ctx, id, 0) },
		Datasets: func(ctx context.Context, id int64) (any, error) { return datasetSvc.List(ctx, id) },
		JoinReqs: func(id int64) (any, error) { return groupSvc.MyJoinRequests(id) },
		Usage: func(id int64) any { // billing.ByUser 전체에서 대상 사용자 1행(누적 GPU시간·소비)
			for _, r := range billingRepo.ByUser() {
				if r.ID == id {
					return r
				}
			}
			return nil
		},
		UserHier: limitResolver.HierOfUser, // 매니저 스코프 검증(대상의 조직/그룹)
		Members: func(id int64) (any, error) { // 전체 소속(조직/팀/역할) — 다중 소속 평면 나열
			type mrow struct {
				Role      string `json:"role"`
				GroupID   int64  `json:"groupId"`
				GroupName string `json:"groupName"`
				OrgID     int64  `json:"orgId"`
				OrgName   string `json:"orgName"`
			}
			var rows []mrow
			err := db.Raw("SELECT m.role, g.id AS group_id, g.display_name AS group_name, o.id AS org_id, o.display_name AS org_name "+
				"FROM memberships m JOIN `groups` g ON g.id=m.group_id JOIN organizations o ON o.id=g.org_id "+
				"WHERE m.user_id=? AND m.status='active' ORDER BY o.display_name, g.display_name", id).Scan(&rows).Error
			return rows, err
		},
	})

	deps := server.Deps{
		Auth:            auth.NewHandler(authSvc),
		Org:             org.NewHandler(orgSvc),
		Group:           group.NewHandler(groupSvc),
		Resource:        resource.NewHandler(resourceSvc),
		Image:           image.NewHandler(imageRepo, imageSvc),
		Session:         session.NewHandler(sessionSvc),
		Wallet:          wallet.NewHandler(walletSvc),
		Topup:           topup.NewHandler(topupSvc),
		Volume:          volume.NewHandler(volumeSvc),
		Dataset:         dataset.NewHandler(datasetSvc),
		Node:            node.NewHandler(nodeSvc),
		Announcement:    announcement.NewHandler(announcement.NewRepository(db), groupSvc, orgRepo),
		Users:           users.NewHandler(usersRepo),
		UserDetail:      userDetailH,
		SystemConfig:    systemconfig.NewHandler(cfg, cfgStore),
		Dashboard:       dashboard.NewHandler(dashboardSvc),
		Audit:           audit.NewHandler(auditRepo),
		AuditRepo:       auditRepo,
		Billing:         billing.NewHandler(billing.NewRepository(db)),
		Alert:           alert.NewHandler(alert.NewRepository(db)),
		Notify:          notify.NewHandler(notifyRepo),
		UserNotify:      usernotify.NewHandler(inboxStore),
		DatasetsEnabled: cfg.Storage.Datasets.Enabled, // off 면 데이터셋 라우트 미등록(404)
		Policy: policy.NewHandler(policyRepo, limitResolver).
			WithOrgOfGroup(orgRepo). // 스코프 정책 편집 인가(그룹→조직)
			// 전역 상한 런타임 편집 — 런타임 저장소에 영속(설치 env 는 기본값으로 남는다).
			WithGlobalSetter(func(g policy.Resolved) error {
				for k, v := range map[string]int{
					systemconfig.KeyQuotaMaxGpu:      g.MaxGpu,
					systemconfig.KeyQuotaMaxVramGB:   g.MaxVramGB,
					systemconfig.KeyQuotaMaxVolGiB:   g.MaxVolumeGiB,
					systemconfig.KeyQuotaMaxSessions: g.MaxConcurrentSessions,
				} {
					if err := cfgStore.Set(k, strconv.Itoa(v)); err != nil {
						return err
					}
				}
				return nil
			}),
		OrgReader:   orgRepo, // authz.OrgReader 구현(IsOrgAdmin)
		AgentToken:  cfg.PhysicalNodes.AgentToken,
		GroupReader: groupSvc, // authz.MembershipReader 구현
		ScopeReader: groupSvc, // authz.ScopeReader 구현(PrimaryScope)
		OrgOfGroup:  orgRepo,  // authz.OrgOfGroupReader 구현
	}

	r := server.New(deps)
	log.Printf("Giosk API listening on %s (deployment=%s, billing=%s)",
		cfg.HTTPAddr, cfg.Deployment.Mode, cfg.Billing.Mode)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
