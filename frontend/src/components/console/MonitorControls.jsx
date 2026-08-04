import React, { useEffect, useState } from 'react';
import { RefreshCw, Maximize2, Minimize2 } from 'lucide-react';
import Select from './Select';

// 사전 정의된 폴링 주기(감시월). 값=ms.
const OPTS = [
  { value: 5000, label: '5초' }, { value: 10000, label: '10초' }, { value: 30000, label: '30초' },
  { value: 60000, label: '1분' }, { value: 120000, label: '2분' }, { value: 300000, label: '5분' },
];

// 감시월 컨트롤. 폴링 주기(테마 드롭다운)와 전체화면 토글(우측 상단)이다. containerRef 가 전체화면 대상 요소다.
export default function MonitorControls({ intervalMs, setIntervalMs, containerRef }) {
  const [fs, setFs] = useState(false);
  const [spin, setSpin] = useState(false);
  useEffect(() => {
    const onFs = () => setFs(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onFs);
    return () => document.removeEventListener('fullscreenchange', onFs);
  }, []);
  // 폴링 주기에 맞춰 새로고침 아이콘을 한 번 돌려 "갱신 중" 신호를 준다(깜박임 대신 은은한 효과).
  useEffect(() => {
    const id = setInterval(() => { setSpin(true); setTimeout(() => setSpin(false), 650); }, intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  const toggleFs = () => {
    const el = containerRef?.current;
    if (!document.fullscreenElement) el?.requestFullscreen?.().catch(() => {});
    else document.exitFullscreen?.();
  };
  return (
    <span className="flex gap" style={{ alignItems: 'center', gap: 8 }}>
      <RefreshCw size={13} className="muted" style={{ animation: spin ? 'giosk-spin 0.65s linear' : undefined }} />
      <Select size="sm" width={96} value={intervalMs} onChange={(v) => setIntervalMs(Number(v))} options={OPTS} />
      <button type="button" className="btn sm" onClick={toggleFs} title="fullscreen">
        {fs ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
      </button>
    </span>
  );
}
