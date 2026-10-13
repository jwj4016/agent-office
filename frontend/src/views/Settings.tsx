import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {applyMotion, ErrorBox, REDUCED_MOTION} from '../ui';
import {Connections} from './Connections';
import {ExportButton} from './ExportButton';

export function Settings() {
    const status = useLoad(() => api.systemStatus(), []);
    const settings = useLoad(() => api.settings(), []);
    const services = useLoad(() => api.dashboard(true), []);
    const [error, setError] = useState<string | null>(null);
    const reduced = settings.data?.[REDUCED_MOTION] === 'true';
    return (
        <div className="stack" style={{maxWidth: 720}}>
            <h1>{t.nav.settings}</h1>
            <section className="card stack">
                <h2>화면</h2>
                <label className="check">
                    <input type="checkbox" checked={reduced} onChange={(e) => {
                        const on = e.target.checked;
                        api.setSetting(REDUCED_MOTION, String(on)).then(() => { applyMotion(on); settings.reload(); }, (err) => setError(String(err)));
                    }}/>
                    모션 줄이기 (사무실 캐릭터 동작을 멈추고 상태 표시만 남깁니다. OS 설정도 따릅니다)
                </label>
            </section>
            <Connections/>
            <section className="card stack" aria-label="서비스 내보내기">
                <h2>서비스 내보내기</h2>
                <p className="muted small">서비스마다 ZIP 하나로 저장합니다. API 키·로그인 정보·공급자 세션은 포함하지 않습니다.</p>
                {(services.data ?? []).map((s) => (
                    <div key={s.project.id} className="stack">
                        <strong>{s.project.name}{s.project.status === 'archived' ? ' (보관됨)' : ''}</strong>
                        <ExportButton projectId={s.project.id}/>
                    </div>
                ))}
                {services.data?.length === 0 ? <p className="muted small">아직 서비스가 없습니다.</p> : null}
            </section>
            <section className="card stack">
                <h2>정보</h2>
                {status.data ? (
                    <div className="small">
                        <div>버전 {status.data.version} · DB 스키마 v{status.data.schemaVersion}</div>
                        <div>저장 위치: <span className="mono">{status.data.dataDir}</span></div>
                        <div>API 키 보관: {status.data.secretsPersistent ? 'OS 비밀 저장소' : '이번 세션 메모리만 (앱 종료 시 사라짐)'}</div>
                        <ErrorBox error={status.data.error}/>
                    </div>
                ) : null}
            </section>
            <ErrorBox error={error}/>
        </div>
    );
}
