// 알림 텍스트 현지화 — 신규 알림은 metric+파라미터로 저장되므로 프론트가 언어별로 렌더한다.
// 구 알림은 title(한국어)이 저장돼 있어 그대로 폴백 표시한다. alert.* 키는 consoleUser 네임스페이스.
// 세션 지표별 한국어 라벨(기본값 렌더용).
const SESSION_METRIC_LABEL = {
  session_gpu: 'GPU 사용률',
  session_cpu: 'CPU 사용률',
  session_vram: 'VRAM 사용률',
};

export function notifyText(n, t) {
  if (n?.title) return { title: n.title, body: n.body };
  const value = Math.round(n?.value ?? 0);
  const threshold = n?.threshold ?? 0;
  const target = n?.target || '';
  const ns = { ns: 'consoleUser' };
  // 세션 단위 알림 — 어느 세션인지 + 지표를 함께 보여준다.
  if (target && SESSION_METRIC_LABEL[n?.metric]) {
    const label = SESSION_METRIC_LABEL[n.metric];
    return {
      title: t(`alert.${n.metric}.title`, { ...ns, target, defaultValue: `세션 ${label} 임계 도달` }),
      body: t(`alert.${n.metric}.body`, { ...ns, value, threshold, target, defaultValue: `세션 ${target} 의 ${label}이 ${value}% 입니다(임계 ${threshold}%).` }),
    };
  }
  return {
    title: t(`alert.${n?.metric}.title`, { ...ns, defaultValue: n?.metric || '' }),
    body: t(`alert.${n?.metric}.body`, { ...ns, value, threshold, defaultValue: `${n?.metric || ''}: ${value} / ${threshold}` }),
  };
}
