import { useEffect, useRef, useState } from 'react';

// usePoll은 fn 을 즉시 한 번 부르고 ms 간격으로 폴링해 최신 데이터를 준다(감시월 자동 새로고침).
// 언마운트/의존성 변경 시 정리. 실패는 무시(직전 값 유지).
// 값이 실제로 바뀌지 않으면 setData 를 건너뛴다. 매 폴링마다 리렌더하거나 차트를 다시 애니메이션하면
// 목록(가입요청 등)이 "깜박"이는 걸 막는다. onTick 은 폴링이 돌 때마다(변경 여부와 무관) 호출.
export default function usePoll(fn, ms = 15000, deps = [], onTick) {
  const [data, setData] = useState(null);
  const fnRef = useRef(fn);
  const tickRef = useRef(onTick);
  const lastRef = useRef('');
  fnRef.current = fn;
  tickRef.current = onTick;
  useEffect(() => {
    let stop = false;
    const run = () => fnRef.current().then((d) => {
      if (stop) return;
      const sig = (() => { try { return JSON.stringify(d); } catch { return null; } })();
      if (sig === null || sig !== lastRef.current) { lastRef.current = sig; setData(d); }
      if (tickRef.current) tickRef.current();
    }).catch(() => {});
    run();
    const id = setInterval(run, ms);
    return () => { stop = true; clearInterval(id); };
  }, [ms, ...deps]); // eslint-disable-line react-hooks/exhaustive-deps
  return data;
}
