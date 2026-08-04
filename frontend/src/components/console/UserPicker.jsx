import React, { useEffect, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { getUsers } from '../../api/console/misc';
import { getOrgs, getGroups } from '../../api/console/governance';
import Select from './Select';
import { nextDropdownId, announceOpen, onDropdownOpen } from './dropdownBus';

// UserPicker는 계정 선택기.
// 이름만으로는 동명이인을 구분할 수 없으므로 (1) 조직·그룹으로 후보를 좁히고
// (2) 후보를 [이름 / 아이디 / 이메일 / 소속] 열로 나눠 보여준다. 아이디는 유일하므로 최종 식별자.
// 선택하면 onChange(username) 를 부른다. 백엔드는 username 이나 email 을 해석한다.
//  onPick(user): 선택된 사용자 객체 전체(예: id 로 저장하는 정책 화면). 자유 입력 시 null.
//  filter(user): 후보 제한(예: 이미 멤버인 사용자 제외).
const MAX_SHOWN = 8;

export default function UserPicker({ value, onChange, placeholder, onPick, filter }) {
  const { t } = useTranslation('consoleAdmin');
  const [all, setAll] = useState([]);
  const [total, setTotal] = useState(0);
  const [q, setQ] = useState(value || '');
  const [open, setOpen] = useState(false);
  const [org, setOrg] = useState('all');
  const [group, setGroup] = useState('all');
  const [orgs, setOrgs] = useState([]);
  const [allGroups, setAllGroups] = useState([]);
  const boxRef = useRef(null);
  const idRef = useRef(null);
  if (idRef.current === null) idRef.current = nextDropdownId();

  // 내부의 조직·그룹 필터를 열면 이 제안 목록은 닫는다(둘이 겹쳐 뜨지 않게).
  useEffect(() => onDropdownOpen((openedId) => {
    if (openedId !== idRef.current) setOpen(false);
  }), []);

  // 후보는 서버에서 좁혀 받는다. 전체 목록을 내려받아 브라우저가 거르면 수천 명에서 못 버틴다.
  // 검색어/필터가 바뀔 때만, 300ms 디바운스로 요청한다.
  useEffect(() => {
    const id = setTimeout(() => {
      getUsers({ q: q.trim(), org: org === 'all' ? '' : org, group: group === 'all' ? '' : group, size: 30 })
        .then((d) => { setAll(d.items || []); setTotal(d.total || 0); })
        .catch(() => {});
    }, 300);
    return () => clearTimeout(id);
  }, [q, org, group]);
  useEffect(() => { setQ(value || ''); }, [value]);
  // 필터 후보다. 사용자 목록이 한 페이지뿐이라 거기서 못 뽑으므로 조직과 그룹 목록에서 가져온다.
  useEffect(() => {
    getOrgs().then((d) => setOrgs((d.items || []).map((o) => o.displayName))).catch(() => {});
    getGroups().then((d) => setAllGroups(d.items || [])).catch(() => {});
  }, []);
  useEffect(() => {
    const h = (e) => { if (boxRef.current && !boxRef.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, []);

  // 서버가 이미 q/org/group 으로 걸러 보냈다. 여기선 호출측 제약(filter: 이미 멤버인 사람 제외)만.
  const narrowed = (filter ? all.filter(filter) : all).filter((u) => u.status !== 'rejected');
  const matches = narrowed.slice(0, MAX_SHOWN);
  const exact = narrowed.some((u) => u.username === q); // 아이디가 정확히 일치 = 선택 완료

  // 후보가 더 있는지는 서버가 30건만 보냈으므로 total 로 판단한다.
  // filter(예: 멤버 제외)가 걸려 있으면 서버는 제외 대상을 모르니 남은 정확한 수를 셀 수 없다
  // 수 대신 검색으로 좁히라고 안내한다(이름이나 아이디를 치면 그 사람이 상위로 온다).
  const hasMore = total > all.length || narrowed.length > MAX_SHOWN;
  const moreExactCount = !filter && total > MAX_SHOWN ? total - MAX_SHOWN : null;

  // 좁힌 결과 안에서 이름이 겹치는 계정이다. 동명이인은 아이디와 소속을 강조해 오선택을 막는다.
  const dupNames = new Set(
    narrowed.map((u) => u.name).filter((n, i, a) => n && a.indexOf(n) !== i),
  );

  const pick = (u) => { setQ(u.username); onChange(u.username); onPick?.(u); setOpen(false); };
  // 제안 목록을 펼칠 때도 알린다. 열려 있던 다른 드롭다운이 닫히도록.
  const openList = () => { announceOpen(idRef.current); setOpen(true); };
  const reset = (patch) => {
    // 필터를 바꾸면 목록을 다시 펼쳐 좁혀진 후보를 즉시 보여준다.
    patch(); openList();
  };

  return (
    <div ref={boxRef} style={{ position: 'relative' }}>
      <div className="flex" style={{ gap: 6, marginBottom: 6 }}>
        <Select size="sm" width="50%" value={org} onChange={(v) => reset(() => { setOrg(v); setGroup('all'); })}
          options={[{ value: 'all', label: t('picker.allOrgs') }, ...orgs.map((o) => ({ value: o, label: o }))]} />
        <Select size="sm" width="50%" value={group} onChange={(v) => reset(() => setGroup(v))}
          options={[{ value: 'all', label: t('picker.allGroups') },
            ...allGroups.filter((g) => org === 'all' || g.orgName === org).map((g) => ({ value: g.displayName, label: g.displayName }))]} />
      </div>
      <input type="text" value={q} placeholder={placeholder || t('picker.searchPh')} autoComplete="off"
        onChange={(e) => {
          setQ(e.target.value); onChange(e.target.value); openList();
          // 자유 입력으로 되돌아가면 선택된 사용자를 해제한다(고른 id 가 남아 오염되지 않게).
          onPick?.(all.find((u) => u.username === e.target.value) || null);
        }}
        onFocus={openList}
        style={exact ? { borderColor: 'var(--free)' } : undefined} />
      {open && (
        <div style={{ position: 'absolute', zIndex: 50, left: 0, right: 0, top: '100%', marginTop: 4,
          background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 8,
          boxShadow: '0 8px 24px rgba(0,0,0,.18)', maxHeight: 300, overflowY: 'auto' }}>
          {narrowed.length === 0 && (
            <div className="muted" style={{ padding: '10px 12px', fontSize: 12.5 }}>
              {/* 서버엔 결과가 있는데(total>0) 화면이 비었으면 filter 로 이 페이지가 다 걸러진 것 →
                  "없음"이 아니라 좁히라고 안내한다(랭크 31+ 인 대상이 안 보이는 상황). */}
              {total > 0 ? t('picker.refine') : t('picker.noMatch')}
            </div>
          )}
          {matches.length > 0 && (
            <div className="muted" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1.2fr', gap: 8,
              padding: '6px 12px', fontSize: 10.5, fontWeight: 700, textTransform: 'uppercase',
              letterSpacing: '.03em', borderBottom: '1px solid var(--border)' }}>
              <span>{t('picker.colName')}</span><span>{t('picker.colAccount')}</span><span>{t('picker.colBelong')}</span>
            </div>
          )}
          {matches.map((u) => (
            <div key={u.id} onMouseDown={() => pick(u)}
              onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-2)'; }}
              onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
              style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1.2fr', gap: 8, alignItems: 'center',
                padding: '8px 12px', cursor: 'pointer', borderBottom: '1px solid var(--border)' }}>
              <span style={{ fontWeight: 600, fontSize: 13, minWidth: 0 }}>
                {u.name || u.username}
                {dupNames.has(u.name) && (
                  <span style={{ marginLeft: 5, fontSize: 10, fontWeight: 700, color: 'var(--warn)' }}>
                    {t('picker.dup')}</span>)}
              </span>
              <span className="mono" style={{ fontSize: 12, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                {u.username}
                {u.email && <div className="muted" style={{ fontSize: 10.5 }}>{u.email}</div>}
              </span>
              <span className="muted" style={{ fontSize: 11.5, minWidth: 0 }}>
                {u.group || <span style={{ opacity: 0.6 }}>{t('picker.noGroup')}</span>}
                {u.org && <div style={{ fontSize: 10.5, opacity: 0.75 }}>{u.org}</div>}
              </span>
            </div>
          ))}
          {hasMore && (
            <div className="muted" style={{ padding: '7px 12px', fontSize: 11.5 }}>
              {moreExactCount != null ? t('picker.more', { n: moreExactCount }) : t('picker.refine')}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
