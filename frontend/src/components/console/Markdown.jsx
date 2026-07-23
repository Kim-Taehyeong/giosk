import React from 'react';
import { ImageIcon } from 'lucide-react';

// 의미 없는 자리표시 이미지(데모용)에 쓰는 그라데이션 팔레트.
const FIG_GRADS = [
  'linear-gradient(135deg,#6366f1,#8b5cf6)',
  'linear-gradient(135deg,#0ea5e9,#22d3ee)',
  'linear-gradient(135deg,#10b981,#34d399)',
  'linear-gradient(135deg,#f59e0b,#fb7185)',
  'linear-gradient(135deg,#ec4899,#8b5cf6)',
];

function Figure({ alt, src, idx }) {
  const placeholder = !src || src === 'placeholder' || src.startsWith('ph');
  return (
    <figure style={{ margin: '16px 0' }}>
      {placeholder ? (
        <div style={{
          height: 220, borderRadius: 12, background: FIG_GRADS[idx % FIG_GRADS.length],
          display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'rgba(255,255,255,.85)',
        }}>
          <ImageIcon size={40} strokeWidth={1.5} />
        </div>
      ) : (
        <img src={src} alt={alt} style={{ width: '100%', borderRadius: 12, display: 'block' }} />
      )}
      {alt && <figcaption className="muted" style={{ fontSize: 12, textAlign: 'center', marginTop: 8 }}>{alt}</figcaption>}
    </figure>
  );
}

// 가벼운 Markdown 렌더러 (제목/목록/코드/굵게/인라인코드/이미지). 정적 가이드 문서용.
function inline(text, keyBase) {
  // **bold** 와 `code` 처리
  const parts = [];
  const regex = /(\*\*[^*]+\*\*|`[^`]+`)/g;
  let last = 0; let m; let i = 0;
  while ((m = regex.exec(text)) !== null) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    const tok = m[0];
    if (tok.startsWith('**')) parts.push(<strong key={`${keyBase}-${i++}`}>{tok.slice(2, -2)}</strong>);
    else parts.push(<code key={`${keyBase}-${i++}`} className="mono" style={{ background: 'var(--surface-2)', padding: '1px 5px', borderRadius: 4 }}>{tok.slice(1, -1)}</code>);
    last = m.index + tok.length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return parts;
}

export default function Markdown({ children }) {
  const lines = (children || '').split('\n');
  const out = [];
  let list = null; let code = null; let key = 0; let fig = 0; let table = null;

  const flushList = () => { if (list) { out.push(<ul key={`ul-${key++}`} style={{ margin: '6px 0 10px', paddingLeft: 20 }}>{list}</ul>); list = null; } };
  const flushTable = () => {
    if (!table) return;
    const rows = table.filter((r) => !/^[\s|:-]+$/.test(r)); // 구분선 행 제거
    const cells = rows.map((r) => r.replace(/^\||\|$/g, '').split('|').map((c) => c.trim()));
    out.push(
      <table key={`tb-${key++}`} style={{ width: '100%', borderCollapse: 'collapse', margin: '12px 0', fontSize: 13 }}>
        <tbody>
          {cells.map((row, ri) => (
            <tr key={ri}>
              {row.map((c, ci) => (ri === 0
                ? <th key={ci} style={{ textAlign: 'left', padding: '8px 10px', borderBottom: '2px solid var(--border)', fontWeight: 700 }}>{inline(c, `${key}-${ri}-${ci}`)}</th>
                : <td key={ci} style={{ padding: '8px 10px', borderBottom: '1px solid var(--border)' }}>{inline(c, `${key}-${ri}-${ci}`)}</td>))}
            </tr>
          ))}
        </tbody>
      </table>,
    );
    table = null;
  };

  const flush = () => { flushList(); flushTable(); };

  lines.forEach((raw) => {
    const line = raw.replace(/\s+$/, '');
    if (line.startsWith('```')) {
      if (code === null) { flush(); code = []; }
      else { out.push(<pre key={`pre-${key++}`} style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', borderRadius: 8, padding: 12, fontSize: 12.5, overflowX: 'auto' }} className="mono">{code.join('\n')}</pre>); code = null; }
      return;
    }
    if (code !== null) { code.push(raw); return; }
    if (line.startsWith('|') && line.endsWith('|')) { flushList(); if (!table) table = []; table.push(line); return; }
    const img = line.match(/^!\[([^\]]*)\]\(([^)]*)\)$/);
    if (img) { flush(); out.push(<Figure key={key++} alt={img[1]} src={img[2]} idx={fig++} />); return; }
    if (line.startsWith('### ')) { flush(); out.push(<h4 key={key++} style={{ fontSize: 14.5, fontWeight: 700, margin: '12px 0 4px' }}>{inline(line.slice(4), key)}</h4>); return; }
    if (line.startsWith('## ')) { flush(); out.push(<h3 key={key++} style={{ fontSize: 16, fontWeight: 800, margin: '14px 0 6px' }}>{inline(line.slice(3), key)}</h3>); return; }
    if (line.startsWith('# ')) { flush(); out.push(<h2 key={key++} style={{ fontSize: 19, fontWeight: 800, margin: '6px 0 8px' }}>{inline(line.slice(2), key)}</h2>); return; }
    if (line.startsWith('- ')) { flushTable(); if (!list) list = []; list.push(<li key={`li-${key++}`} style={{ margin: '3px 0', fontSize: 13.5 }}>{inline(line.slice(2), key)}</li>); return; }
    if (line.trim() === '') { flush(); return; }
    flush();
    out.push(<p key={key++} style={{ margin: '6px 0', fontSize: 13.5, lineHeight: 1.6 }}>{inline(line, key)}</p>);
  });
  flush();
  return <div>{out}</div>;
}
