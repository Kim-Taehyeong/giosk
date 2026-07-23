// 알림 텍스트 현지화 — 신규 알림은 metric+파라미터로 저장되므로 프론트가 언어별로 렌더한다.
// 구 알림은 title(한국어)이 저장돼 있어 그대로 폴백 표시한다. alert.* 키는 consoleUser 네임스페이스.
export function notifyText(n, t) {
  if (n?.title) return { title: n.title, body: n.body };
  const value = Math.round(n?.value ?? 0);
  const threshold = n?.threshold ?? 0;
  const ns = { ns: 'consoleUser' };
  return {
    title: t(`alert.${n?.metric}.title`, { ...ns, defaultValue: n?.metric || '' }),
    body: t(`alert.${n?.metric}.body`, { ...ns, value, threshold, defaultValue: `${n?.metric || ''}: ${value} / ${threshold}` }),
  };
}
