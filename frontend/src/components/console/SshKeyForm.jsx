import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyRound, Download } from 'lucide-react';
import { apiPut, apiPost } from '../../api/client';
import { useAuth } from '../../context/AuthContext';
import { useToast } from './Toast';

// downloadText는 서버가 1회만 내려주는 개인키를 파일로 저장시킨다(브라우저 Blob 다운로드).
function downloadText(filename, text) {
  const url = URL.createObjectURL(new Blob([text], { type: 'application/x-pem-file' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// SshKeyForm은 SSH 공개키 등록/생성 UI(내정보·접속 모달 공용).
// 저장은 백엔드(users.ssh_public_key)에 하며, 실행 중인 컨테이너 세션에도 즉시 반영된다.
// 키 생성은 서버가 키쌍을 만들어 개인키를 1회만 내려준다(서버는 보관하지 않는 EC2 키페어 방식).
export default function SshKeyForm({ onSaved }) {
  const { t } = useTranslation('consoleUser');
  const { user, refreshUser } = useAuth();
  const { toast } = useToast();
  const [key, setKey] = useState(user?.sshPublicKey || '');
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    try {
      await apiPut('/auth/me/ssh-key', { publicKey: key });
      await refreshUser();
      toast(t('account.sshSaved'));
      onSaved?.();
    } catch (e) {
      toast(e?.code === 'bad_public_key'
        ? t('account.sshBad', { defaultValue: '공개키 형식이 올바르지 않습니다. "ssh-ed25519 AAAA..." 한 줄을 붙여넣으세요.' })
        : (e?.message || t('account.sshFail', { defaultValue: 'SSH 공개키 저장에 실패했습니다.' })));
    }
    setBusy(false);
  };

  const generate = async () => {
    setBusy(true);
    try {
      const r = await apiPost('/auth/me/ssh-key/generate', {});
      downloadText(r.filename || 'giosk-key.pem', r.privateKey);
      const me = await refreshUser();
      setKey(me?.sshPublicKey || '');
      toast(t('account.sshGenerated', { defaultValue: '키를 생성했습니다. 내려받은 개인키 파일을 안전하게 보관하세요(다시 받을 수 없습니다).' }));
      onSaved?.();
    } catch (e) {
      toast(e?.message || t('account.sshGenFail', { defaultValue: 'SSH 키 생성에 실패했습니다.' }));
    }
    setBusy(false);
  };

  return (
    <div>
      <textarea value={key} onChange={(e) => setKey(e.target.value)} placeholder="ssh-ed25519 AAAA..." rows={3} />
      <div className="legend">{t('account.sshHint')}</div>
      <div className="flex gap mt">
        <button type="button" className="btn primary" disabled={busy} onClick={save}>
          <KeyRound size={14} /> {t('account.save')}
        </button>
        <button type="button" className="btn" disabled={busy} onClick={generate}>
          <Download size={14} /> {t('account.sshGenerate', { defaultValue: '키 생성 & 다운로드' })}
        </button>
      </div>
      <div className="legend mt">
        {t('account.sshGenerateHint', { defaultValue: '키가 없으면 "키 생성 & 다운로드"를 누르세요. 개인키 파일(.pem)이 한 번만 내려받아지고 공개키는 자동 등록됩니다. 접속: ssh -i 내려받은키.pem ...' })}
      </div>
    </div>
  );
}
