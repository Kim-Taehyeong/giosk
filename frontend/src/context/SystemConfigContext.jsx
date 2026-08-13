import React, { createContext, useContext, useState, useCallback, useEffect, useRef } from 'react';
import { apiGet } from '../api/client';
import { putSystemConfig } from '../api/console/systemconfig';

// 전역 시스템 설정이다. 운영 모드, 과금, 유휴, 예약 정책의 단일 출처다.
// 백엔드 없이 localStorage 에 영속한다. 첫 실행 위저드(/setup)와 관리자 Settings 가 공유한다.
const SystemConfigContext = createContext(null);

const STORAGE_KEY = 'giosk_system_config';

// 기본 설정. 이후 단계(데이터셋·SSH 등) 키는 그 단계에서 확장한다.
export const DEFAULT_CONFIG = {
  setupComplete: false,
  deploymentMode: 'container', // 'container' | 'hybrid'
  // 브랜딩은 조직별 커스터마이즈다. name 은 워드마크, accent 는 로고 마크 색이다(비면 테마 기본색).
  branding: { name: 'Giosk', accent: '' },
  billing: {
    // 과금 모드: 크레딧 ↔ Dynamic 은 상호 배타(하나만 사용).
    mode: 'credit', // 'credit' | 'dynamic'
    credit: { pricing: 'static', surgeIncrement: 10 }, // pricing: 'static' | 'dynamic'
    dynamic: { maxLeaseHours: 8, cooldownHours: 2 },
  },
  idle: { timeoutMin: 30 },
  // 중단 세션 홈 회수 정책(운영 중 조정). 중단 세션도 노드 로컬 디스크를 계속 점유하므로,
  // 방치 stoppedTtlDays 일을 넘긴 중단 세션의 홈을 회수한다(사용자별 최신 1개는 면책).
  // stoppedTtlDays: 0 = 자동 회수 끔(중단 세션 개수 상한과 스토리지 과금만으로 억제).
  reclaim: { stoppedTtlDays: 14 },
  // 선착순(Dynamic) 임대 세션 연장 정책이다. 회당 최대 연장 시간과 최대 연장 횟수를 정한다.
  lease: { extensionHours: 4, maxExtensions: 2 },
  // 사용자 기능 on/off (운영 중 조정 가능). 끄면 해당 기능은 관리자만 수행.
  features: {
    signupRequest: true,   // 사용자 가입 신청 허용
    datasets: true,        // 데이터셋 기능 전체(메뉴, 캐싱, 선택). 끄면 NFS 나 중앙 스토리지 의존도가 낮아진다
    datasetRegister: true, // 사용자 데이터셋 등록 허용 (datasets 켜진 경우만 의미)
    workloadAlerts: true,  // 워크로드(가용성) 알림 신청 허용
    groupJoinRequest: true, // 타 그룹 가입 신청 허용
    creditRequest: true,   // 크레딧 충전 요청 허용 (크레딧 모드 전용)
  },
  // 전역 자원 할당량이다. 최고 관리자가 모든 조직을 통틀어 적용하는 플랫폼 상한이다.
  quota: { maxGpuCount: 64, maxVramGb: 512, maxConcurrentSessions: 50, volumeQuotaGb: 2000 },
};

const isObj = (v) => v && typeof v === 'object' && !Array.isArray(v);

// 중첩 patch 를 재귀 병합 (배열/스칼라는 교체).
function deepMerge(base, patch) {
  const out = { ...base };
  Object.keys(patch || {}).forEach((k) => {
    out[k] = isObj(patch[k]) && isObj(base?.[k]) ? deepMerge(base[k], patch[k]) : patch[k];
  });
  return out;
}

const load = () => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved ? deepMerge(DEFAULT_CONFIG, JSON.parse(saved)) : DEFAULT_CONFIG;
  } catch {
    return DEFAULT_CONFIG;
  }
};

export const SystemConfigProvider = ({ children }) => {
  const [config, setConfig] = useState(load);

  // 설치 시점 설정은 백엔드(/api/config, Helm 주입)가 권위 + 런타임 오버라이드(유휴·기능)를 합친 유효 설정.
  const refresh = useCallback(() => apiGet('/config')
    .then((remote) => { setConfig(deepMerge(DEFAULT_CONFIG, remote)); return remote; })
    .catch(() => { /* 백엔드 미가용 시 현재 유지(오프라인 미리보기) */ }), []);
  useEffect(() => { refresh(); }, [refresh]);

  const persist = useCallback((next) => {
    setConfig(next);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  }, []);

  // 부분 patch 를 낙관적 반영 + 디바운스(600ms) 후 백엔드(runtime_config)에 영속화.
  // 텍스트 입력처럼 연속 호출돼도 마지막 1회만 PUT(경쟁/난사 방지). 백엔드는 운영중 조정 항목
  // (branding·idle·features)만 저장하고 설치시 고정 항목은 무시한다.
  const latestRef = useRef(config);
  const saveTimer = useRef(null);
  const update = useCallback((patch) => {
    setConfig((prev) => {
      const next = deepMerge(prev, patch);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      latestRef.current = next;
      return next;
    });
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(async () => {
      const n = latestRef.current;
      try {
        await putSystemConfig({ branding: n.branding, idle: n.idle, features: n.features, storage: n.storage });
        await refresh();
      } catch { /* 백엔드 미가용 — 낙관적 로컬 상태 유지 */ }
    }, 600);
  }, [refresh]);

  const reset = useCallback(() => persist(DEFAULT_CONFIG), [persist]);

  return (
    <SystemConfigContext.Provider
      value={{ config, update, reset, refresh, isSetupComplete: !!config.setupComplete }}
    >
      {children}
    </SystemConfigContext.Provider>
  );
};

export const useSystemConfig = () => useContext(SystemConfigContext);
