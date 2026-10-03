import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {ErrorBox} from '../ui';

const REDUCED_MOTION = 'ui.reducedMotion';

export function Settings() {
    const status = useLoad(() => api.systemStatus(), []);
    const settings = useLoad(() => api.settings(), []);
    const conns = useLoad(() => api.connections(), []);
    const [error, setError] = useState<string | null>(null);
    const reduced = settings.data?.[REDUCED_MOTION] === 'true';
    return (
        <div className="stack" style={{maxWidth: 720}}>
            <h1>{t.nav.settings}</h1>
            <section className="card stack">
                <h2>화면</h2>
                <label className="check">
                    <input type="checkbox" checked={reduced} onChange={(e) => api.setSetting(REDUCED_MOTION, String(e.target.checked))
                        .then(settings.reload, (err) => setError(String(err)))}/>
                    모션 줄이기
                </label>
            </section>
            <section className="card stack">
                <h2>AI 연결</h2>
                <p className="muted small">실제 Codex·Claude 연결 설정과 연결 시험은 다음 단계(M2)에서 제공합니다.</p>
                {(conns.data ?? []).map((c) => (
                    <div key={c.id} className="row between">
                        <span>{c.name} <span className="muted small">({c.provider})</span></span>
                        <span className={`badge ${c.usable ? 'ok' : 'bad'}`}>{c.note}</span>
                    </div>
                ))}
                {(conns.data ?? []).length === 0 ? <p className="muted small">연결이 없습니다.</p> : null}
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
