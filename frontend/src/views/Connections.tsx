import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {CheckReport, Connection, ConnectionKind, ConnectionSettings} from '../types';
import {ConfirmButton, ErrorBox, useAction} from '../ui';

const kindLabel: Record<ConnectionKind, string> = {
    test: '테스트 (가짜 응답)',
    codex: 'Codex CLI (이 PC의 Codex 로그인)',
    claude: 'Claude Agent SDK',
    claude_api: 'Claude API',
    openai_api: 'OpenAI API',
};

const needsKey = (c: Connection) =>
    c.provider === 'claude_api' || c.provider === 'openai_api' || (c.provider === 'claude' && c.settings.authMode !== 'local_login');

export function Connections() {
    const list = useLoad(() => api.connections(), []);
    const [editing, setEditing] = useState<Form | null>(null);
    return (
        <section className="card stack" aria-label="AI 연결">
            <div className="row between">
                <h2>AI 연결</h2>
                <button className="btn" onClick={() => setEditing({...emptyForm})}>연결 추가</button>
            </div>
            <p className="muted small">
                연결 상태는 실행 파일 → 버전 → 인증 → 실제 호출 순서로 따로 확인합니다. ‘실제 호출 시험’에 성공한 연결만 업무를 실행할 수 있습니다.
            </p>
            <ErrorBox error={list.error}/>
            {(list.data ?? []).map((c) => <ConnectionCard key={c.id} conn={c} onEdit={() => setEditing(toForm(c))} onChanged={list.reload}/>)}
            {(list.data ?? []).length === 0 ? <p className="muted small">연결이 없습니다.</p> : null}
            {editing ? <ConnectionForm form={editing} onDone={() => { setEditing(null); list.reload(); }}/> : null}
        </section>
    );
}

function ConnectionCard({conn, onEdit, onChanged}: { conn: Connection; onEdit: () => void; onChanged: () => void }) {
    const action = useAction();
    const [report, setReport] = useState<CheckReport | null>(null);
    const [key, setKey] = useState('');
    const [model, setModel] = useState('');
    const isTest = conn.provider === 'test';
    return (
        <div className="card stack" aria-label={`연결 ${conn.name}`}>
            <div className="row between">
                <div>
                    <strong>{conn.name}</strong> <span className="muted small">{kindLabel[conn.provider] ?? conn.provider}</span>
                    {conn.provider === 'claude' && conn.settings.authMode === 'local_login' ? <span className="badge warn" style={{marginLeft: 6}}>개인용 로그인</span> : null}
                </div>
                <span className={`badge ${conn.usable ? 'ok' : 'bad'}`}>{isTest ? '테스트' : conn.usable ? '사용 가능' : '확인 필요'}</span>
            </div>
            <p className="muted small" style={{margin: 0}}>{conn.note}</p>
            {!isTest ? (
                <>
                    {needsKey(conn) ? (
                        <div className="row">
                            <span className="small">API 키: {conn.hasKey ? <span className="badge ok">저장됨</span> : <span className="badge bad">없음</span>}</span>
                            <input type="password" aria-label="API 키" placeholder={conn.hasKey ? '새 키로 바꾸기' : 'API 키 입력'} value={key}
                                   autoComplete="off" onChange={(e) => setKey(e.target.value)} style={{flex: 1, minWidth: 160}}/>
                            <button className="btn small" disabled={!key.trim() || action.busy}
                                    onClick={() => action.run(async () => { await api.setConnectionKey(conn.id, key); setKey(''); onChanged(); })}>키 저장</button>
                            {conn.hasKey ? <button className="btn small danger" disabled={action.busy}
                                                   onClick={() => action.run(async () => { await api.clearConnectionKey(conn.id); onChanged(); })}>키 삭제</button> : null}
                        </div>
                    ) : null}
                    <div className="row">
                        <button className="btn small" onClick={onEdit}>{t.common.edit}</button>
                        <button className="btn small" disabled={action.busy}
                                onClick={() => action.run(async () => setReport(await api.checkConnection(conn.id)))}>연결 확인 (무료)</button>
                        <input aria-label="시험 모델" placeholder={conn.provider === 'openai_api' ? '모델 이름 (필수)' : '모델 (선택)'} value={model}
                               onChange={(e) => setModel(e.target.value)} style={{width: 160}}/>
                        <ConfirmButton label="실제 호출 시험" confirmLabel="AI 사용량을 써서 시험" className="btn small" disabled={action.busy}
                                       onConfirm={() => action.run(async () => { setReport(await api.testConnectionCall(conn.id, model)); onChanged(); })}/>
                    </div>
                    {action.busy ? <p className="muted small">확인 중…</p> : null}
                    {report ? <ReportView report={report}/> : null}
                </>
            ) : null}
            <ErrorBox error={action.error}/>
        </div>
    );
}

const stepMark = {ok: '✓', failed: '✗', skipped: '–'};

function ReportView({report}: { report: CheckReport }) {
    return (
        <ul className="stack small" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="확인 결과">
            {report.steps.map((s) => (
                <li key={s.id} className="row">
                    <span className={`badge ${s.status === 'ok' ? 'ok' : s.status === 'failed' ? 'bad' : ''}`} aria-label={s.status}>{stepMark[s.status]}</span>
                    <strong>{s.label}</strong><span className="muted">{s.detail}</span>
                </li>
            ))}
            <li>{report.ready ? <span className="badge ok">실제 호출까지 확인됨 — 업무 실행 가능</span>
                : <span className="badge warn">아직 실행할 수 없음 (실제 호출 시험 필요)</span>}</li>
        </ul>
    );
}

type Form = {
    id: string; name: string; provider: ConnectionKind; executablePath: string; settings: ConnectionSettings;
};

const emptyForm: Form = {id: '', name: '', provider: 'claude', executablePath: '', settings: {authMode: 'api_key'}};

function toForm(c: Connection): Form {
    return {id: c.id, name: c.name, provider: c.provider, executablePath: c.executablePath, settings: {...c.settings}};
}

const num = (v: string) => (v.trim() === '' ? undefined : Number(v));

function ConnectionForm({form: initial, onDone}: { form: Form; onDone: () => void }) {
    const [form, setForm] = useState(initial);
    const action = useAction();
    const set = (patch: Partial<Form>) => setForm({...form, ...patch});
    const setS = (patch: Partial<ConnectionSettings>) => setForm({...form, settings: {...form.settings, ...patch}});
    const detect = () => action.run(async () => {
        const d = await api.detectPaths();
        if (form.provider === 'codex') set({executablePath: d.codex});
        if (form.provider === 'claude') setS({node: d.node, script: d.bridge});
    });
    const save = () => action.run(async () => {
        await api.saveConnection({id: form.id || undefined, name: form.name, provider: form.provider, executablePath: form.executablePath, settings: form.settings});
        onDone();
    });
    return (
        <form className="card stack" aria-label="연결 편집" onSubmit={(e) => { e.preventDefault(); save(); }}>
            <h3>{form.id ? '연결 편집' : '새 연결'}</h3>
            <p className="muted small">설정을 바꾸면 실제 호출 시험을 다시 통과해야 실행할 수 있습니다.</p>
            <label className="field"><span>종류</span>
                <select value={form.provider} disabled={!!form.id} onChange={(e) => set({provider: e.target.value as ConnectionKind,
                    settings: e.target.value === 'claude' ? {authMode: 'api_key'} : {}})}>
                    {(['claude', 'codex', 'claude_api', 'openai_api'] as ConnectionKind[]).map((k) => <option key={k} value={k}>{kindLabel[k]}</option>)}
                </select></label>
            <label className="field"><span>이름</span><input value={form.name} onChange={(e) => set({name: e.target.value})} placeholder="예: 내 Claude"/></label>

            {form.provider === 'codex' ? (
                <>
                    <label className="field"><span>codex 실행 파일 (비우면 자동 탐지)</span>
                        <input value={form.executablePath} onChange={(e) => set({executablePath: e.target.value})}/></label>
                    <p className="muted small">이 PC에서 로그인한 Codex 계정으로 실행됩니다. 로그인은 터미널에서 codex로 직접 합니다.</p>
                </>
            ) : null}

            {form.provider === 'claude' ? (
                <>
                    <fieldset className="stack" style={{border: 'none', padding: 0}}>
                        <legend className="small muted">인증 방식</legend>
                        <label className="check"><input type="radio" checked={form.settings.authMode !== 'local_login'}
                                                        onChange={() => setS({authMode: 'api_key'})}/> Anthropic API 키 (기본)</label>
                        <label className="check"><input type="radio" checked={form.settings.authMode === 'local_login'}
                                                        onChange={() => setS({authMode: 'local_login'})}/> 이 PC의 Claude 로그인 (개인 사용 전용)</label>
                    </fieldset>
                    {form.settings.authMode === 'local_login' ? (
                        <p className="alert" role="note">
                            Anthropic은 사전 승인 없이 타사 제품이 claude.ai 로그인을 제공하는 것을 허용하지 않습니다. 이 방식은 본인 PC에서 본인만 쓰는 경우에만 선택하고,
                            앱을 배포하거나 다른 사람에게 제공할 때는 API 키를 사용하세요. 사용량은 구독 한도에서 차감됩니다.
                        </p>
                    ) : null}
                    <label className="field"><span>Node.js 실행 파일 (비우면 자동 탐지)</span>
                        <input value={form.settings.node ?? ''} onChange={(e) => setS({node: e.target.value})}/></label>
                    <label className="field"><span>Claude 실행 프로그램 (runners/claude/dist/main.js, 비우면 자동 탐지)</span>
                        <input value={form.settings.script ?? ''} onChange={(e) => setS({script: e.target.value})}/></label>
                </>
            ) : null}

            {form.provider === 'claude_api' || form.provider === 'openai_api' ? (
                <label className="field"><span>API 주소 (비우면 공식 주소)</span>
                    <input value={form.settings.baseUrl ?? ''} onChange={(e) => setS({baseUrl: e.target.value})}/></label>
            ) : null}

            <div className="row">
                <label className="field"><span>입력 단가 (USD/100만 토큰, 선택)</span>
                    <input inputMode="decimal" value={form.settings.inputPerMTok ?? ''} onChange={(e) => setS({inputPerMTok: num(e.target.value)})}/></label>
                <label className="field"><span>출력 단가 (USD/100만 토큰, 선택)</span>
                    <input inputMode="decimal" value={form.settings.outputPerMTok ?? ''} onChange={(e) => setS({outputPerMTok: num(e.target.value)})}/></label>
            </div>
            <p className="muted small">단가를 비워 두면 비용은 ‘알 수 없음’으로 표시됩니다 (0원으로 표시하지 않음).</p>
            <ErrorBox error={action.error}/>
            <div className="row">
                <button className="btn primary" type="submit" disabled={action.busy || !form.name.trim()}>{t.common.save}</button>
                {form.provider === 'codex' || form.provider === 'claude' ? <button className="btn" type="button" onClick={detect}>자동 탐지</button> : null}
                <button className="btn" type="button" onClick={onDone}>{t.common.cancel}</button>
            </div>
        </form>
    );
}
