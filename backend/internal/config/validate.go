package config

import (
	"errors"
	"fmt"
)

// Validate는 설계 문서 §2/§3 의 설치 시점 불변식을 강제한다.
// 각 규칙은 작은 헬퍼로 분리해 한눈에 보이게 한다.
func (c *Config) Validate() error {
	checks := []func() error{
		c.checkEnums,
		c.checkVolumeStorage,
		c.checkDatasetNFS,
		c.checkPhysicalNodes,
		c.checkSharedHome,
		c.checkGateway,
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) checkEnums() error {
	if !oneOf(c.Deployment.Mode, DeploymentContainer, DeploymentHybrid) {
		return fmt.Errorf("deployment.mode invalid: %q", c.Deployment.Mode)
	}
	if !oneOf(c.Billing.Mode, BillingCredit, BillingDynamic, BillingFree) {
		return fmt.Errorf("billing.mode invalid: %q", c.Billing.Mode)
	}
	return nil
}

// 스토리지 단일화: 모든 볼륨은 NFS(RWX) 클래스로 프로비저닝된다. 따라서 nfsClass 는
// 반드시 지정돼야 하고, 노드 로컬 클래스(local-path 등)면 RWX·교차-ns 공유가 불가하므로 거부한다.
func (c *Config) checkVolumeStorage() error {
	sc := c.Storage.NFSClass
	if sc == "" {
		return errors.New("storage.nfsClass required: all volumes are provisioned NFS(RWX)")
	}
	if isNodeLocalClass(sc) {
		return fmt.Errorf("storage.nfsClass must be RWX/node-independent (NFS/CephFS/…), got node-local %q", sc)
	}
	return nil
}

// 데이터셋 사용 시 NFS server+path 필수. 데이터셋은 정규경로 <path>/dataset/<name> 에
// 저장되어 물리노드가 직접 마운트할 수 있어야 하므로, 쓰기 가능한 NFS export 지정이 반드시 필요하다.
func (c *Config) checkDatasetNFS() error {
	if !c.Storage.Datasets.Enabled {
		return nil
	}
	return requireNFSEndpoint("datasets", c.Storage.Datasets.NFS)
}

// 물리노드 활성 시 hybrid + 외부 NFS(server+path) 필수.
func (c *Config) checkPhysicalNodes() error {
	if !c.PhysicalNodes.Enabled {
		return nil
	}
	if !c.IsHybrid() {
		return errors.New("physicalNodes.enabled requires deployment.mode=hybrid")
	}
	return requireNFSEndpoint("physicalNodes", c.PhysicalNodes.NFS)
}

// 공유 영속 home(sharedHome) 활성 시 home 은 사용자의 여러 노드 세션이 공유하므로
// RWX·노드독립 스토리지클래스여야 한다. 노드 로컬 클래스(local-path 등)면 설치 자체를 거부한다
// (런타임에 PV 노드고정→스케줄 실패로 터지는 대신 설치시 차단). RWX 보장 못 하는 빈 값도 거부.
func (c *Config) checkSharedHome() error {
	if !c.Storage.SharedHome {
		return nil // 공유 home 미사용 → 영속 PVC 안 만들고 세션 emptyDir 로컬 임시만(스토리지 제약 없음)
	}
	sc := c.Storage.PersistenceClass
	if sc == "" {
		return errors.New("storage.persistence.sharedHome=true requires an RWX storage class (storage.persistence.storageClass)")
	}
	if isNodeLocalClass(sc) {
		return fmt.Errorf("storage.persistence.sharedHome=true requires RWX/node-independent storageClass, got node-local %q (use NFS/CephFS/… or set sharedHome=false)", sc)
	}
	return nil
}

// 접속 게이트웨이 설정 검증. 게이트웨이 SSH/토큰은 API 와 게이트웨이가 공유 비밀로 결합되므로,
// 컨테이너 sshd 사이드카를 켰으면(SessionSSHDImage) 반드시 공유 비밀(GatewaySecret)이 있어야 한다
// (없으면 토큰 발급 불가 → 접속 불능). 웹 게이트웨이 자체는 secret 없으면 조용히 직접접속 폴백.
func (c *Config) checkGateway() error {
	if c.K8s.SessionSSHDImage != "" && c.K8s.GatewaySecret == "" {
		return errors.New("k8s.sessionSSHDImage set but gatewaySecret empty: container SSH needs a shared gateway secret")
	}
	if c.K8s.GatewayScheme != "" && !oneOf(c.K8s.GatewayScheme, "http", "https") {
		return fmt.Errorf("k8s.gatewayScheme invalid: %q (http|https)", c.K8s.GatewayScheme)
	}
	return nil
}

// isNodeLocalClass는 RWX 불가(노드 고정)로 알려진 스토리지클래스 이름인지 본다(보수적 차단 목록).
func isNodeLocalClass(sc string) bool {
	switch sc {
	case "local-path", "local", "hostpath", "hostpath-provisioner":
		return true
	}
	return false
}

func requireNFSEndpoint(scope string, nfs NFS) error {
	if nfs.Server == "" || nfs.Path == "" {
		return fmt.Errorf("%s requires external NFS server and path", scope)
	}
	return nil
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
