package store

import "gorm.io/gorm"

// SeedDefaults는 자원 카탈로그(오퍼링/프리셋/이미지)가 비어 있으면 기본값을 넣는다(멱등).
// 세션 마법사가 즉시 동작하도록. 이미지는 테스트에서 실제로 Running 되는 nginx 사용.
func SeedDefaults(db *gorm.DB) error {
	for _, step := range []func(*gorm.DB) error{seedOfferings, seedPresets, seedImages} {
		if err := step(db); err != nil {
			return err
		}
	}
	return nil
}

// seedOfferings는 GPU 무관한 기본 오퍼링만 넣는다.
// 공유(fractional) GPU 오퍼링은 gpu_type 이 클러스터 노드 라벨(nvidia.com/gpu.product)과
// 정확히 일치해야 가용성 조회·스케줄이 된다. 그 값은 설치 시점에 알 수 없으므로(클러스터마다 다름)
// 하드코딩하지 않는다. 관리자가 자원 관리 화면에서 실제 gpu-types 드롭다운으로 만든다.
// (예전엔 rtx4090 이나 a100 을 하드코딩해 실 클러스터 라벨과 어긋났고, 그래서 공유 모드가 항상 "가용 없음"이었다.)
func seedOfferings(db *gorm.DB) error {
	if count(db, "offerings") > 0 {
		return nil
	}
	return db.Exec(`INSERT INTO offerings (name, gpu_type, vram_mb, core_percent, mode, price_per_hour, is_active) VALUES
		('cpu-data','',0,0,'exclusive',0,1)`).Error
}

func seedPresets(db *gorm.DB) error {
	if count(db, "presets") > 0 {
		return nil
	}
	return db.Exec(`INSERT INTO presets (name, display_name, icon, vram_mb, core_percent, sort_order, is_active) VALUES
		('light','가벼운 작업','🌱',2048,10,1,1),
		('standard','표준','💻',8192,50,2,1),
		('large','대형 학습','🚀',24576,100,3,1),
		('cpu','데이터 준비(CPU)','⬇',0,0,4,1)`).Error
}

// seedImages는 테스트에서 실제로 기동되는 이미지(nginx:alpine)다. 운영에서는 실 세션 이미지로 교체한다.
func seedImages(db *gorm.DB) error {
	if count(db, "images") > 0 {
		return nil
	}
	// 기본 이미지: code-server(VSCode/SSH) + Jupyter. GPU·CPU 세션 공용. 중복 행 방지.
	// 접속 채널은 channels JSON 이 결정(vscode=8080/PASSWORD, jupyter=8888/JUPYTER_TOKEN, ssh).
	return db.Exec(`INSERT INTO images (name, tag, base_image, description, channels, tags, gpu, scan_status, signed, status) VALUES
		('codercom/code-server','latest','codercom/code-server:latest','VSCode/code-server 개발 환경(GPU·CPU 공용, 랜덤 비밀번호)', '{"vscode":true,"ssh":true}', '["GPU","CPU"]', 1, 'pass', 1, 'active'),
		('quay.io/jupyter/base-notebook','latest','quay.io/jupyter/base-notebook:latest','Jupyter Notebook/Lab 환경(GPU·CPU 공용, 랜덤 토큰)', '{"jupyter":true}', '["GPU","CPU"]', 1, 'pass', 1, 'active')`).Error
}

func count(db *gorm.DB, table string) int64 {
	var n int64
	db.Raw("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n
}
