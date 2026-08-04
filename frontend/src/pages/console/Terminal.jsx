import React from 'react';
import { useParams } from 'react-router-dom';
import TerminalPane from '../../components/console/TerminalPane';

// 웹터미널 단독 페이지다. 세션 접속 모달의 웹으로 연결하기(새 창)가 window.open 으로 여는 전체화면 터미널이다.
export default function Terminal() {
  const { id } = useParams();
  return (
    <div style={{ height: '100vh', width: '100vw', background: '#1e1e1e', overflow: 'hidden' }}>
      <TerminalPane session={{ id }} fill />
    </div>
  );
}
