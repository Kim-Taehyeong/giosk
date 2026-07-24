import React, { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { wsURL } from '../../api/client';

// TerminalPane은 세션 웹터미널 — 브라우저 xterm ↔ API websocket(/instances/:id/terminal).
// 컨테이너 세션은 파드 exec, 물리 세션은 노드 SSH 로 백엔드가 브리지한다(사용자는 키/비번 불요).
//   프로토콜: C→S '0'+입력 / '1'+"cols,rows"(리사이즈), S→C 원시 출력 바이트.
//
// 검은 화면 방지 장치 3가지(터미널 안에서만 알리면 렌더가 실패했을 때 아무것도 안 보인다):
//   ① 상태 바를 xterm 밖 일반 DOM 으로 그린다 — xterm 이 죽어도 상태·재시도가 보인다.
//   ② 크기가 0 인 동안에는 fit() 하지 않는다(새 창은 열린 직후 0x0 일 수 있고, 0 에서 fit 하면
//      cols/rows 가 NaN 이 되어 이후 출력이 화면에 그려지지 않는다).
//   ③ dispose 는 한 틱 뒤에 — xterm 이 예약해 둔 스크롤 동기화 콜백이 먼저 흐르게 해서
//      "Cannot read properties of undefined (reading 'dimensions')" 미처리 예외로 트리가 날아가는 걸 막는다.
export default function TerminalPane({ session, fill = false }) {
  const hostRef = useRef(null);
  const [status, setStatus] = useState('connecting'); // connecting | open | closed | error
  const [attempt, setAttempt] = useState(0);          // 재시도 트리거

  useEffect(() => {
    const host = hostRef.current;
    if (!session?.id || !host) return undefined;

    let alive = true;
    let opened = false;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      theme: { background: '#1e1e1e', foreground: '#e6e6e6', cursor: '#e6e6e6' },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);

    const safeFit = () => {
      if (!alive || !host.clientWidth || !host.clientHeight) return; // 0 크기에서 fit 금지(②)
      try { fit.fit(); } catch { /* 렌더러 준비 전 — 다음 호출에서 맞춰진다 */ }
    };
    safeFit();
    term.writeln('\x1b[90m연결 중…\x1b[0m');

    const ws = new WebSocket(wsURL(`/instances/${session.id}/terminal`));
    ws.binaryType = 'arraybuffer';
    // 서버(x/net/websocket)는 프레임을 []byte 로 받으므로 반드시 "바이너리" 프레임으로 보내야 한다.
    // ws.send(string) 은 텍스트 프레임이라 서버 Receive 가 거부→연결이 즉시 끊긴다(context canceled).
    // 프레임 = [프리픽스 1바이트('0'입력/'1'리사이즈)] + [UTF-8 payload].
    const enc = new TextEncoder();
    const sendFrame = (prefix, payload) => {
      if (ws.readyState !== 1) return;
      const body = enc.encode(payload);
      const frame = new Uint8Array(body.length + 1);
      frame[0] = prefix.charCodeAt(0);
      frame.set(body, 1);
      ws.send(frame);
    };
    const sendResize = () => sendFrame('1', `${term.cols},${term.rows}`);
    // 연결이 "CONNECTING"에서 멈춰 검은 화면만 나오는 것을 막는다 — 일정 시간 내 안 열리면 실패로 안내.
    const connectTimer = setTimeout(() => {
      if (alive && !opened) {
        setStatus('error');
        term.writeln('\r\n\x1b[31m[접속 실패] 세션 서버에 연결하지 못했습니다.\x1b[0m');
        term.writeln('\x1b[90m세션이 실행(running) 중인지 확인하고, 잠시 후 다시 시도해 주세요.\x1b[0m');
        try { ws.close(); } catch { /* noop */ }
      }
    }, 10000);

    ws.onopen = () => {
      opened = true;
      clearTimeout(connectTimer);
      if (!alive) return;
      setStatus('open');
      safeFit();
      sendResize();
    };
    ws.onmessage = (ev) => {
      if (!alive) return;
      term.write(typeof ev.data === 'string' ? ev.data : new Uint8Array(ev.data));
    };
    ws.onclose = (ev) => {
      clearTimeout(connectTimer);
      if (!alive) return;
      setStatus(opened ? 'closed' : 'error');
      term.writeln(opened
        ? `\r\n\x1b[90m[연결이 종료되었습니다]${ev?.reason ? ` ${ev.reason}` : ''}\x1b[0m`
        : `\r\n\x1b[31m[접속 실패] 연결이 즉시 종료되었습니다(인증 만료 또는 세션 미실행).${ev?.reason ? ` ${ev.reason}` : ''}\x1b[0m`);
    };
    ws.onerror = () => {
      if (!alive) return;
      setStatus('error');
      term.writeln('\r\n\x1b[31m[연결 오류] 웹소켓 연결에 실패했습니다.\x1b[0m');
    };

    const dData = term.onData((d) => sendFrame('0', d));
    const dResize = term.onResize(() => sendResize());
    const ro = new ResizeObserver(() => safeFit());
    ro.observe(host);
    window.addEventListener('resize', safeFit); // 새 창 크기 조절(ResizeObserver 가 못 잡는 경우 대비)
    term.focus();

    return () => {
      alive = false;
      window.removeEventListener('resize', safeFit);
      dData.dispose();
      dResize.dispose();
      ro.disconnect();
      clearTimeout(connectTimer);
      try { ws.close(); } catch { /* noop */ }
      // ③ 예약된 xterm 내부 콜백이 흐른 뒤에 파기(즉시 dispose 하면 미처리 예외가 난다).
      setTimeout(() => { try { term.dispose(); } catch { /* noop */ } }, 0);
    };
  }, [session?.id, attempt]);

  const tone = status === 'open' ? 'var(--free)' : status === 'connecting' ? 'var(--muted)' : 'var(--danger)';
  const label = status === 'open' ? '연결됨' : status === 'connecting' ? '연결 중…' : status === 'closed' ? '연결 종료' : '연결 실패';

  return (
    <div className="tabpane active" style={fill ? { height: '100%', display: 'flex', flexDirection: 'column' } : undefined}>
      {/* 상태 바(①) — xterm 렌더에 문제가 생겨도 이 줄은 항상 보인다. */}
      <div className="flex" style={{ alignItems: 'center', justifyContent: 'space-between', gap: 8, flex: '0 0 auto',
        padding: '6px 10px', background: '#141414', color: '#ddd', fontSize: 12, borderBottom: '1px solid #2a2a2a' }}>
        <span className="flex" style={{ alignItems: 'center', gap: 7 }}>
          <span style={{ width: 8, height: 8, borderRadius: 4, background: tone, flex: '0 0 auto' }} />
          <span style={{ fontWeight: 700 }}>{label}</span>
          <span style={{ opacity: 0.6 }}>{session?.id}</span>
        </span>
        {status !== 'open' && status !== 'connecting' && (
          <button type="button" className="btn sm" onClick={() => { setStatus('connecting'); setAttempt((n) => n + 1); }}>다시 연결</button>
        )}
      </div>
      <div ref={hostRef} style={{ width: '100%', flex: fill ? '1 1 auto' : undefined, height: fill ? undefined : 400,
        background: '#1e1e1e', borderRadius: fill ? 0 : 8, padding: 8, overflow: 'hidden' }} />
    </div>
  );
}
