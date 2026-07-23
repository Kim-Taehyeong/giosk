import React, { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, Upload } from 'lucide-react';
import { useToast } from './Toast';
import { downloadCsv, parseCsvText } from '../../utils/csv';

// 엑셀(CSV) 양식 다운로드 + 일괄 업로드 버튼 묶음 — 사용자/그룹/조직 등에서 공용.
//  header: 양식 헤더 셀 배열, samples: 예시 행들, onRows: 파싱된 데이터 행(헤더 제외) 콜백.
export default function BulkImport({ filename, header, samples = [], onRows }) {
  const { t } = useTranslation('consoleAdmin');
  const { toast } = useToast();
  const ref = useRef(null);

  const download = () => downloadCsv(filename, [header, ...samples]);

  const onFile = async (e) => {
    const f = e.target.files?.[0];
    e.target.value = ''; // 같은 파일 재선택 허용
    if (!f) return;
    const all = parseCsvText(await f.text());
    // 첫 행 첫 셀이 헤더와 같으면 헤더로 보고 건너뜀.
    const data = all.length && all[0][0] === header[0] ? all.slice(1) : all;
    if (data.length === 0) { toast(t('bulk.empty')); return; }
    onRows(data);
  };

  return (
    <span className="flex gap">
      <button className="btn" onClick={download}><Download size={15} /> {t('bulk.download')}</button>
      <button className="btn" onClick={() => ref.current?.click()}><Upload size={15} /> {t('bulk.upload')}</button>
      <input ref={ref} type="file" accept=".csv,text/csv" onChange={onFile} style={{ display: 'none' }} />
    </span>
  );
}
