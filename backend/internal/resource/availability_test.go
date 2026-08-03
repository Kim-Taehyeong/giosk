package resource

import "testing"

// 노드 고정 세션(홈이 노드 로컬)은 "클러스터 어딘가"가 아니라 "그 노드"에 자리가 있어야 뜬다.
func TestCanPlaceOnNode_그_노드_기준으로만_판정한다(t *testing.T) {
	av := Availability{ByNode: []NodeAvail{
		{Node: "gpu-1", GpuType: "RTX4090", GpuTotal: 2, GpuFree: 1},
		{Node: "gpu-2", GpuType: "A100", GpuTotal: 4, GpuFree: 4},
	}}
	none := Reservation{GpuByType: map[string]int{}, SharedByType: map[string]ShareUse{}}

	// 다른 모델을 요청하면 거절 — 클러스터에 A100 이 남아 있어도 이 세션은 gpu-1 에서만 뜬다.
	if canPlaceOnNode(av, PlaceReq{Node: "gpu-1", GpuMode: "exclusive", GpuType: "A100", GpuCount: 1}, none) {
		t.Fatal("고정 노드에 없는 GPU 모델을 승인했다")
	}
	// 그 노드의 여유 안이면 통과, 넘으면 거절.
	if !canPlaceOnNode(av, PlaceReq{Node: "gpu-1", GpuMode: "exclusive", GpuType: "RTX4090", GpuCount: 1}, none) {
		t.Fatal("여유 1장인 노드에 1장 요청이 거절됐다")
	}
	if canPlaceOnNode(av, PlaceReq{Node: "gpu-1", GpuMode: "exclusive", GpuType: "RTX4090", GpuCount: 2}, none) {
		t.Fatal("여유 1장인 노드에 2장 요청을 승인했다(파드는 노드에 걸칠 수 없다)")
	}
	// 인벤토리에 없는 노드는 막지 않는다(조회 공백으로 사용자를 잠그지 않는다).
	if !canPlaceOnNode(av, PlaceReq{Node: "unknown", GpuMode: "exclusive", GpuType: "RTX4090", GpuCount: 1}, none) {
		t.Fatal("모르는 노드에서 거절됐다 — 관문은 확실할 때만 막아야 한다")
	}
}

// 아직 노드가 안 붙은(스케줄 대기) 세션의 예약분을 빼야, 동시에 들어온 두 번째 요청이
// 같은 자리를 다시 승인받아 Pending 으로 매달리지 않는다.
func TestCanPlaceOnNode_예약분을_빼고_판정한다(t *testing.T) {
	av := Availability{ByNode: []NodeAvail{{Node: "gpu-1", GpuType: "RTX4090", GpuTotal: 2, GpuFree: 1}}}
	held := Reservation{GpuByType: map[string]int{"RTX4090": 1}, SharedByType: map[string]ShareUse{}}
	if canPlaceOnNode(av, PlaceReq{Node: "gpu-1", GpuMode: "exclusive", GpuType: "RTX4090", GpuCount: 1}, held) {
		t.Fatal("남은 1장을 이미 예약한 세션이 있는데 또 승인했다")
	}
}

// 분할(HAMi)은 VRAM·코어·슬롯 셋 다 남아야 들어간다. 예약분도 같은 축에서 빠져야 한다.
func TestFitsShared_예약분까지_반영해_세_축을_모두_본다(t *testing.T) {
	n := NodeAvail{
		Node: "gpu-1", GpuType: "RTX4090", Fractional: true,
		FracVramFreeMB: 8192, FracCoresFree: 60, FracSlotsFree: 2,
	}
	req := PlaceReq{GpuMode: "shared", GpuType: "RTX4090", VramMB: 4096, CorePercent: 50}
	if !fitsShared(n, req, ShareUse{}) {
		t.Fatal("여유가 충분한데 거절됐다")
	}
	// VRAM 은 남지만 코어가 예약분에 먹혀 부족.
	if fitsShared(n, req, ShareUse{Cores: 20}) {
		t.Fatal("코어 여유가 부족한데 승인했다")
	}
	// 슬롯(동시 분할 태스크 수)이 예약분으로 소진.
	if fitsShared(n, req, ShareUse{Count: 2}) {
		t.Fatal("슬롯이 남지 않았는데 승인했다")
	}
	// VRAM 이 예약분으로 소진.
	if fitsShared(n, req, ShareUse{VramMB: 6144}) {
		t.Fatal("VRAM 여유가 부족한데 승인했다")
	}
}
