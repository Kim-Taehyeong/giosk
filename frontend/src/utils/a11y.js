// 버튼이 아닌 요소(div/span)를 클릭 대상으로 쓸 때 키보드에서도 동작하게 하는 프로퍼티 묶음.
//
// 원칙: 새로 만드는 조작 요소는 <button> 을 쓴다. 이 헬퍼는 이미 카드/탭/칩 형태로
// 스타일이 잡혀 있어 <button> 으로 바꾸면 레이아웃이 흔들리는 기존 요소를 위한 보정이다.
//
//   <div className="selbox" {...clickable(() => pick(x))}>…</div>
//
// fn 이 없으면(비활성) 아무 속성도 주지 않아 포커스 순서에서도 빠진다.
export function clickable(fn, { role = 'button', label, pressed, disabled = false } = {}) {
  if (!fn || disabled) return { 'aria-disabled': disabled || undefined };
  return {
    role,
    tabIndex: 0,
    'aria-label': label,
    'aria-pressed': pressed,
    onClick: fn,
    onKeyDown: (e) => {
      if (e.key !== 'Enter' && e.key !== ' ') return;
      if (e.target !== e.currentTarget) return; // 내부 컨트롤의 키 입력은 그쪽 것
      e.preventDefault(); // Space 로 페이지가 스크롤되지 않게
      fn(e);
    },
  };
}

// 탭 묶음용이다. 선택 상태를 aria-pressed 로 알린다(role=tab 은 tablist 와 tabpanel 배선이 함께 필요해 과하다).
export const tabbable = (fn, active) => clickable(fn, { pressed: !!active });
