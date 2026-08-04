// 크레딧 숫자 포맷이다. 천단위 콤마(1,000,000)를 쓰며 잔액, 소비, 풀, 단가 등 모든 크레딧 표기에 쓴다.
export const c = (n) => Number(n || 0).toLocaleString();
// cU: 값 뒤에 " C" 단위까지 붙인다(예: "1,000,000 C").
export const cU = (n) => `${c(n)} C`;
