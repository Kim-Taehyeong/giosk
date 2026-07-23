// 드롭다운 배타 제어 — 화면에 열린 드롭다운은 항상 하나뿐이게 한다.
//
// 각 드롭다운이 "내 바깥을 눌렀나"만 보면, 서로 중첩된 경우(예: UserPicker 안의 조직/그룹 필터)
// 안쪽을 눌러도 바깥 목록은 자기 내부 클릭으로 보고 닫지 않아 둘이 동시에 열린다.
// 그래서 위치가 아니라 "누가 마지막으로 열었는지"로 판단한다.
let seq = 0;
const listeners = new Set();

// 드롭다운마다 한 번 호출해 고유 id 를 받는다.
export const nextDropdownId = () => { seq += 1; return seq; };

// 열릴 때 호출 — 자기 id 를 알린다. 나머지는 스스로 닫는다.
export const announceOpen = (id) => { listeners.forEach((fn) => fn(id)); };

// 다른 드롭다운이 열리면 fn(openedId) 이 불린다. 구독 해제 함수를 반환.
export const onDropdownOpen = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};
