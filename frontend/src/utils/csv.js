// CSV(엑셀 호환) 다운로드와 파싱 유틸이다. 외부 라이브러리 없이 동작하며 UTF-8 BOM 으로 한글 깨짐을 막는다.

function escapeCell(v) {
  const s = String(v ?? '');
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function downloadCsv(filename, rows2d) {
  const csv = rows2d.map((r) => r.map(escapeCell).join(',')).join('\r\n');
  const blob = new Blob(['﻿', csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url; a.download = filename; a.click();
  URL.revokeObjectURL(url);
}

// 한 줄 파싱(따옴표/이스케이프 처리).
function parseLine(line) {
  const out = []; let cur = ''; let q = false;
  for (let i = 0; i < line.length; i += 1) {
    const c = line[i];
    if (q) {
      if (c === '"' && line[i + 1] === '"') { cur += '"'; i += 1; } else if (c === '"') q = false; else cur += c;
    } else if (c === '"') q = true;
    else if (c === ',') { out.push(cur); cur = ''; } else cur += c;
  }
  out.push(cur);
  return out.map((x) => x.trim());
}

// 텍스트를 행 배열로 바꾼다(빈 줄 제거). 헤더 처리는 호출측에서 한다.
export function parseCsvText(text) {
  return String(text).replace(/^﻿/, '').split(/\r?\n/).filter((l) => l.trim()).map(parseLine);
}
