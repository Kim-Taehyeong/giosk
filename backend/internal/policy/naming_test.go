package policy

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// GORM 기본 네이밍은 MaxVolumeGiB 를 max_volume_gi_b 로 바꾼다(실제 컬럼은 max_volume_gib).
// 태그가 빠지면 Scan 이 조용히 nil 을 넣어 "저장은 됐는데 목록에 안 보이는" 버그가 된다.
// 이 테스트는 PolicyRow 의 각 필드가 실제 컬럼명으로 매핑되는지 고정한다.
func TestPolicyRowColumnNames(t *testing.T) {
	s, err := schema.Parse(&PolicyRow{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema parse: %v", err)
	}
	want := []string{"scope", "id", "name", "max_gpu", "max_vram_gb", "max_volume_gib", "max_concurrent_sessions", "max_stopped_sessions"}
	for _, col := range want {
		if s.LookUpField(col) == nil {
			t.Errorf("컬럼 %q 에 매핑되는 필드가 없다. gorm:\"column:%s\" 태그를 확인하라", col, col)
		}
	}
	// 기본 네이밍이 만들어내는 잘못된 이름이 남아 있지 않은지.
	for _, bad := range []string{"max_volume_gi_b", "max_vram_g_b"} {
		if f := s.LookUpField(bad); f != nil && f.DBName == bad {
			t.Errorf("필드가 잘못된 컬럼 %q 로 매핑됐다", bad)
		}
	}
}
