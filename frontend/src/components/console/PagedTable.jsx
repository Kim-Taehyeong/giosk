import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import DataTable from './DataTable';

// 클라이언트 페이지네이션 래퍼다. 긴 목록(크레딧 내역이나 감사 로그 등)을 잘라서 보여준다.
// DataTable 와 동일한 props(rows/columns/rowKey/onRowClick)를 받고 pageSize 만 추가.
export default function PagedTable({ rows = [], pageSize = 10, ...rest }) {
  const { t } = useTranslation('consoleAdmin');
  const [page, setPage] = useState(1);
  const total = rows.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  // 행 수가 줄어 현재 페이지가 비면 마지막 페이지로 당긴다.
  useEffect(() => { if (page > pageCount) setPage(pageCount); }, [pageCount, page]);
  const start = (page - 1) * pageSize;
  const slice = rows.slice(start, start + pageSize);

  return (
    <>
      <DataTable rows={slice} {...rest} />
      {total > pageSize && (
        <div className="flex" style={{ justifyContent: 'space-between', alignItems: 'center', marginTop: 10 }}>
          <span className="muted" style={{ fontSize: 12.5 }}>
            {t('paged.range', { from: start + 1, to: Math.min(start + pageSize, total), total, defaultValue: `${start + 1}–${Math.min(start + pageSize, total)} / ${total}` })}
          </span>
          <span className="flex" style={{ gap: 6, alignItems: 'center' }}>
            <button className="btn sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}><ChevronLeft size={14} /></button>
            <span style={{ fontSize: 12.5, fontWeight: 600 }}>{page} / {pageCount}</span>
            <button className="btn sm" disabled={page >= pageCount} onClick={() => setPage((p) => Math.min(pageCount, p + 1))}><ChevronRight size={14} /></button>
          </span>
        </div>
      )}
    </>
  );
}
