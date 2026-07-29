// Package dayspine은 "최근 N일" 시계열의 날짜 축(스파인)을 만든다.
//
// 일자별 GROUP BY 쿼리는 행이 존재하는 날짜만 돌려준다 — 값이 0인 날은 결과에서 통째로 빠진다.
// 그 결과를 그대로 차트에 넘기면 "최근 14일"이 아니라 "값이 있었던 날들"이 되고,
// 카테고리 축 라인 차트에선 12일 떨어진 두 점이 바로 옆에 붙어 그려져 x축이 거짓말을 한다.
// 잔디 히트맵은 더 나쁘다 — 칸 인덱스로 날짜를 역산하므로 빠진 날이 하나만 있어도
// 모든 칸의 날짜와 요일 정렬이 밀린다.
//
// 그래서 조회 결과를 스파인에 얹어 빈 날을 0으로 메운 뒤 프론트로 보낸다.
package dayspine

import "time"

// Layout은 스파인 키 형식(정렬 가능한 ISO 날짜). 표시용 포맷은 호출부에서 만든다 —
// "%m/%d" 같은 짧은 포맷을 키로 쓰면 연말(12/31→01/01)에 정렬이 뒤집힌다.
const Layout = "2006-01-02"

// Keys는 anchor(포함)로 끝나는 days개 날짜 키를 오래된→최근 순으로 만든다.
//
// anchor 는 반드시 DB 의 오늘(CURDATE())이어야 한다. Go 의 시계로 만들면 DB 서버
// 타임존이 다를 때 스파인과 쿼리 WHERE 절의 기준일이 하루 어긋나, 축 전체가 밀린다.
func Keys(anchor time.Time, days int) []string {
	if days < 1 {
		return nil
	}
	out := make([]string, 0, days)
	start := anchor.AddDate(0, 0, -(days - 1))
	for i := 0; i < days; i++ {
		out = append(out, start.AddDate(0, 0, i).Format(Layout))
	}
	return out
}
