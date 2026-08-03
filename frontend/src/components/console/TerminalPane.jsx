import React, { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { wsURL } from '../../api/client';

// TerminalPane은 세션 웹터미널 — 브라우저 xterm ↔ API websocket(/instances/:id/terminal).
// 컨테이너 세션은 파드 exec, 물리 세션은 노드 SSH 로 백엔드가 브리지한다(사용자는 키/비번 불요).
//   프로토콜: C→S '0'+입력 / '1'+"cols,rows"(리사이즈), S→C 원시 출력 바이트.
export default function TerminalPane({ session, fill = false }) {
  const hostRef = useRef(null);

  useEffect(() => {
    if (!session?.id || !hostRef.current) return undefined;
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      theme: { background: '#1e1e1e', foreground: '#e6e6e6', cursor: '#e6e6e6' },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current);
    try { fit.fit(); } catch { /* 초기 사이즈 미확정 시 무시 */ }

    let alive = true;
    let opened = false;
    term.writeln('\x1b[90m연결 중…\x1b[0m');
    const ws = new WebSocket(wsURL(`/instances/${session.id}/terminal`));
    ws.binaryType = 'arraybuffer';
    const sendResize = () => { if (ws.readyState === 1) ws.send(`1${term.cols},${term.rows}`); };
    // 연결이 "CONNECTING"에서 멈춰 검은 화면만 나오는 것을 막는다 — 일정 시간 내 안 열리면 실패로 안내.
    const connectTimer = setTimeout(() => {
      if (alive && !opened) {
        term.writeln('\r\n\x1b[31m[접속 실패] 세션 서버에 연결하지 못했습니다.\x1b[0m');
        term.writeln('\x1b[90m세션이 실행(running) 중인지 확인하고, 잠시 후 다시 시도해 주세요.\x1b[0m');
        try { ws.close(); } catch { /* noop */ }
      }
    }, 10000);

    ws.onopen = () => { opened = true; clearTimeout(connectTimer); sendResize(); };
    ws.onmessage = (ev) => {
      term.write(typeof ev.data === 'string' ? ev.data : new Uint8Array(ev.data));
    };
    ws.onclose = () => {
      clearTimeout(connectTimer);
      if (!alive) return;
      term.writeln(opened ? '\r\n\x1b[90m[연결이 종료되었습니다]\x1b[0m' : '\r\n\x1b[31m[접속 실패] 연결이 즉시 종료되었습니다(인증 만료 또는 세션 미실행).\x1b[0m');
    };
    ws.onerror = () => { if (alive) term.writeln('\r\n\x1b[31m[연결 오류] 웹소켓 연결에 실패했습니다.\x1b[0m'); };

    const dData = term.onData((d) => { if (ws.readyState === 1) ws.send(`0${d}`); });
    const dResize = term.onResize(() => sendResize());
    const ro = new ResizeObserver(() => { try { fit.fit(); } catch { /* noop */ } });
    ro.observe(hostRef.current);
    term.focus();

    return () => {
      alive = false;
      dData.dispose();
      dResize.dispose();
      ro.disconnect();
      try { ws.close(); } catch { /* noop */ }
      term.dispose();
    };
  }, [session?.id]);

  return (
    <div className="tabpane active" style={fill ? { height: '100%' } : undefined}>
      <div ref={hostRef} style={{ width: '100%', height: fill ? '100%' : 400, background: '#1e1e1e', borderRadius: fill ? 0 : 8, padding: 8, overflow: 'hidden' }} />
    </div>
  );
}
