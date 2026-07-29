import React from 'react';
import { useTranslation } from 'react-i18next';

// 간단 테이블 래퍼.
// columns: [{ key, header, render?(row), className? }]
// rows: 데이터 배열. emptyText: 빈 상태 문구(미지정 시 공용 번역).
// expandedRow?(row,i): 값이 있으면 해당 행 바로 아래 전체너비 확장 행으로 렌더(아코디언).
// onRowClick?(row): 행 전체를 클릭 대상으로. "관리" 버튼을 따로 두지 않고 행을 눌러 상세로 간다.
//   행 안의 버튼/입력은 클릭이 행으로 번지지 않게 자동으로 막는다(아래 stopPropagation).
//   클릭 행은 키보드로도 열 수 있어야 한다(tabIndex + Enter/Space). <tr> 에 role=button 을 씌우면
//   표 구조(행/열 위치) 안내가 사라지므로 행 시맨틱은 유지하고 조작만 추가한다.
// rowLabel?(row,i): 스크린리더가 읽을 행 이름. 없으면 첫 열의 원시값으로 대체한다.
export default function DataTable({ columns, rows, emptyText, rowKey, expandedRow, onRowClick, rowLabel, className = '' }) {
  const { t } = useTranslation('common');
  const empty = emptyText ?? t('table.empty', { defaultValue: '데이터가 없습니다' });

  // 행 이름: 명시 rowLabel > 첫 열의 원시값(문자열/숫자) > 없음.
  const nameOf = (row, i) => {
    if (rowLabel) return rowLabel(row, i);
    const v = row?.[columns[0]?.key];
    return typeof v === 'string' || typeof v === 'number' ? String(v) : undefined;
  };

  return (
    <div className={className} style={{ overflowX: 'auto' }}>
      <table>
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.key}>{c.header}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {(!rows || rows.length === 0) ? (
            <tr>
              <td colSpan={columns.length} style={{ textAlign: 'center', color: 'var(--muted)' }}>
                {empty}
              </td>
            </tr>
          ) : (
            rows.map((row, i) => {
              const ex = expandedRow ? expandedRow(row, i) : null;
              return (
                <React.Fragment key={rowKey ? rowKey(row, i) : i}>
                  <tr
                    onClick={onRowClick ? (e) => {
                      // 행 안의 조작 요소(버튼/링크/입력)를 누른 거면 상세로 넘어가지 않는다.
                      if (e.target.closest('button, a, input, select, textarea, [role=dialog]')) return;
                      onRowClick(row, i);
                    } : undefined}
                    onKeyDown={onRowClick ? (e) => {
                      if (e.key !== 'Enter' && e.key !== ' ') return;
                      // 행 안의 컨트롤에 포커스가 있으면 그 컨트롤의 키 입력이다(행을 열지 않는다).
                      if (e.target !== e.currentTarget) return;
                      e.preventDefault(); // Space 로 페이지가 스크롤되지 않게
                      onRowClick(row, i);
                    } : undefined}
                    tabIndex={onRowClick ? 0 : undefined}
                    aria-label={onRowClick ? nameOf(row, i) : undefined}
                    className={onRowClick ? 'row-link' : undefined}
                    style={onRowClick ? { cursor: 'pointer' } : undefined}
                  >
                    {columns.map((c) => (
                      <td key={c.key} className={c.className}>
                        {c.render ? c.render(row, i) : row[c.key]}
                      </td>
                    ))}
                  </tr>
                  {ex && (
                    <tr>
                      <td colSpan={columns.length} style={{ background: 'var(--surface)' }}><div className="row-detail">{ex}</div></td>
                    </tr>
                  )}
                </React.Fragment>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}
