import React from 'react';

// 간단 테이블 래퍼.
// columns: [{ key, header, render?(row), className? }]
// rows: 데이터 배열. emptyText: 빈 상태 문구.
// expandedRow?(row,i): 값이 있으면 해당 행 바로 아래 전체너비 확장 행으로 렌더(아코디언).
// onRowClick?(row): 행 전체를 클릭 대상으로. "관리" 버튼을 따로 두지 않고 행을 눌러 상세로 간다.
//   행 안의 버튼/입력은 클릭이 행으로 번지지 않게 자동으로 막는다(아래 stopPropagation).
export default function DataTable({ columns, rows, emptyText = '데이터가 없습니다', rowKey, expandedRow, onRowClick, className = '' }) {
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
                {emptyText}
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
