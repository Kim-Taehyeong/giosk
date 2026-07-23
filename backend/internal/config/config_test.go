package config

import "testing"

// fakeEnv는 map 기반 lookup으로 os 의존 없이 Load를 테스트한다.
func fakeEnv(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoad_Defaults(t *testing.T) {
	c, err := Load(fakeEnv(map[string]string{}))
	if err != nil {
		t.Fatalf("기본값 로드 실패: %v", err)
	}
	if c.Deployment.Mode != DeploymentContainer {
		t.Errorf("deployment.mode 기본값 = %q, want container", c.Deployment.Mode)
	}
	if !c.IsCredit() {
		t.Errorf("billing.mode 기본값은 credit 이어야 함")
	}
	if c.Storage.Datasets.Enabled {
		t.Errorf("datasets 기본값은 off 여야 함")
	}
}

func TestValidate_DatasetsRequireNFS(t *testing.T) {
	// 데이터셋 사용 + external NFS인데 server/path 누락 → 에러
	_, err := Load(fakeEnv(map[string]string{
		"GIOSK_DATASETS_ENABLED":  "true",
		"GIOSK_DATASETS_NFS_MODE": "external",
	}))
	if err == nil {
		t.Fatal("external NFS 정보 누락 시 에러가 나야 함")
	}
}

func TestValidate_DatasetsExternalNFS_OK(t *testing.T) {
	c, err := Load(fakeEnv(map[string]string{
		"GIOSK_DATASETS_ENABLED":    "true",
		"GIOSK_DATASETS_NFS_MODE":   "external",
		"GIOSK_DATASETS_NFS_SERVER": "10.0.0.5",
		"GIOSK_DATASETS_NFS_PATH":   "/export/datasets",
	}))
	if err != nil {
		t.Fatalf("정상 external NFS 인데 에러: %v", err)
	}
	if c.Storage.Datasets.NFS.Server != "10.0.0.5" {
		t.Errorf("nfs server 파싱 실패")
	}
}

func TestValidate_PhysicalNodesNeedHybridAndNFS(t *testing.T) {
	// 물리노드 on + container 모드 → 에러
	if _, err := Load(fakeEnv(map[string]string{
		"GIOSK_PHYSICAL_NODES_ENABLED": "true",
		"GIOSK_DEPLOYMENT_MODE":        "container",
	})); err == nil {
		t.Fatal("물리노드는 hybrid를 요구해야 함")
	}
	// 물리노드 on + hybrid + 외부 NFS 누락 → 에러
	if _, err := Load(fakeEnv(map[string]string{
		"GIOSK_PHYSICAL_NODES_ENABLED": "true",
		"GIOSK_DEPLOYMENT_MODE":        "hybrid",
	})); err == nil {
		t.Fatal("물리노드는 외부 NFS를 요구해야 함")
	}
	// 정상
	if _, err := Load(fakeEnv(map[string]string{
		"GIOSK_PHYSICAL_NODES_ENABLED": "true",
		"GIOSK_DEPLOYMENT_MODE":        "hybrid",
		"GIOSK_PHYSICAL_NFS_SERVER":    "10.0.0.6",
		"GIOSK_PHYSICAL_NFS_PATH":      "/export/phys",
	})); err != nil {
		t.Fatalf("정상 물리노드 설정인데 에러: %v", err)
	}
}

func TestValidate_BadEnum(t *testing.T) {
	if _, err := Load(fakeEnv(map[string]string{"GIOSK_BILLING_MODE": "bitcoin"})); err == nil {
		t.Fatal("잘못된 billing.mode는 에러여야 함")
	}
}

func TestDSN(t *testing.T) {
	d := Database{Host: "h", Port: 3306, Name: "giosk", User: "u", Pass: "p"}
	got := d.DSN()
	want := "u:p@tcp(h:3306)/giosk?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true"
	if got != want {
		t.Errorf("DSN=%q want %q", got, want)
	}
}
