import {Database, Info, Monitor, Moon, Package, Sun} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import {applyMotion, applyTheme, ErrorBox, Icon, Loading, PageHeader, REDUCED_MOTION, Section, StatusBadge, THEME, type ThemePref, useToast} from '../ui';
import {Connections} from './Connections';
import {ExportButton} from './ExportButton';

const themes: { id: ThemePref; label: string; icon: typeof Sun }[] = [
    {id: 'system', label: '시스템 설정 따르기', icon: Monitor}, {id: 'light', label: '라이트', icon: Sun}, {id: 'dark', label: '다크', icon: Moon},
];

export function Settings() {
    const status = useLoad(() => api.systemStatus(), []);
    const settings = useLoad(() => api.settings(), []);
    const services = useLoad(() => api.dashboard(true), []);
    const [error, setError] = useState<string | null>(null);
    const toast = useToast();
    const reduced = settings.data?.[REDUCED_MOTION] === 'true';
    const theme = (settings.data?.[THEME] as ThemePref | undefined) ?? 'system';
    const save = (key: string, value: string, apply: () => void) =>
        api.setSetting(key, value).then(() => { apply(); settings.reload(); toast('화면 설정을 바꿨습니다'); }, (err) => setError(String(err)));
    return (
        <div className="page narrow">
            <PageHeader title={t.nav.settings} desc="화면, AI 연결, 내보내기를 설정합니다. 설정은 이 PC에만 저장됩니다."/>
            <ErrorBox error={error}/>

            <Section title={<><Icon icon={Monitor} size={18}/>화면</>} label="화면">
                <div className="card stack">
                    <fieldset className="stack tight">
                        <legend className="field-label" style={{marginBottom: 8}}>테마</legend>
                        <div className="segmented" role="group" aria-label="테마">
                            {themes.map((x) => (
                                <button key={x.id} type="button" className="btn small" aria-pressed={theme === x.id}
                                        onClick={() => save(THEME, x.id, () => applyTheme(x.id))}>
                                    <Icon icon={x.icon} size={14}/>{x.label}
                                </button>
                            ))}
                        </div>
                    </fieldset>
                    <label className="check">
                        <input type="checkbox" checked={reduced} onChange={(e) => {
                            const on = e.target.checked;
                            save(REDUCED_MOTION, String(on), () => applyMotion(on));
                        }}/>
                        <span>모션 줄이기 <span className="muted small">— 사무실 캐릭터 동작과 화면 전환 효과를 멈춥니다. OS 설정도 따릅니다.</span></span>
                    </label>
                </div>
            </Section>

            <Connections/>

            <Section title={<><Icon icon={Package} size={18}/>서비스 내보내기</>} label="서비스 내보내기">
                <div className="card stack">
                    <p className="muted small">서비스마다 ZIP 하나로 저장합니다. API 키·로그인 정보·공급자 세션은 포함하지 않습니다.</p>
                    {services.data ? null : <Loading rows={1}/>}
                    {(services.data ?? []).map((s) => (
                        <div key={s.project.id} className="row between">
                            <span className="row"><strong>{s.project.name}</strong>{s.project.status === 'archived' ? <StatusBadge>보관됨</StatusBadge> : null}</span>
                            <ExportButton projectId={s.project.id}/>
                        </div>
                    ))}
                    {services.data?.length === 0 ? <p className="muted small">아직 서비스가 없습니다.</p> : null}
                </div>
            </Section>

            <Section title={<><Icon icon={Info} size={18}/>정보</>} label="정보">
                <div className="card stack">
                    {status.data ? (
                        <dl className="kv">
                            <dt>버전</dt><dd>{status.data.version} · DB 스키마 v{status.data.schemaVersion}</dd>
                            <dt>저장 위치</dt><dd className="mono">{status.data.dataDir}</dd>
                            <dt>API 키 보관</dt><dd>{status.data.secretsPersistent ? 'OS 비밀 저장소'
                                : <StatusBadge tone="warn" icon={Database}>이번 세션 메모리만 (앱 종료 시 사라짐)</StatusBadge>}</dd>
                        </dl>
                    ) : <Loading rows={1}/>}
                    <ErrorBox error={status.data?.error}/>
                </div>
            </Section>
        </div>
    );
}
