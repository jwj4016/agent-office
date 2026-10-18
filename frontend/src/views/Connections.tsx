import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {CheckReport, Connection, ConnectionKind, ConnectionSettings} from '../types';
import {
    AlertTriangle, Bot, CheckCircle2, FlaskConical, KeyRound, MinusCircle, Pencil, Plug, Plus, Stethoscope, XCircle, Zap,
} from 'lucide-react';
import {Banner, Button, ConfirmButton, EmptyState, ErrorBox, Icon, Section, StatusBadge, useAction, useToast} from '../ui';

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
        <Section title={<><Icon icon={Plug} size={18}/>AI 연결</>} label="AI 연결"
                 actions={<Button icon={Plus} onClick={() => setEditing({...emptyForm})}>연결 추가</Button>}>
            <p className="muted small">
                연결은 실행 파일 → 버전 → 인증 → 실제 호출 순서로 확인합니다. ‘실제 호출 시험’에 성공한 연결만 업무를 실행할 수 있습니다.
            </p>
            <ErrorBox error={list.error}/>
            {editing ? <ConnectionForm form={editing} onDone={() => { setEditing(null); list.reload(); }}/> : null}
            {(list.data ?? []).map((c) => <ConnectionCard key={c.id} conn={c} onEdit={() => setEditing(toForm(c))} onChanged={list.reload}/>)}
            {list.data && list.data.length === 0 && !editing ? (
                <EmptyState compact icon={Plug} title="연결이 없습니다" actions={<Button variant="primary" icon={Plus} onClick={() => setEditing({...emptyForm})}>연결 추가</Button>}>
                    Claude·Codex·API 키 중 하나를 연결하면 AI 담당자가 일할 수 있습니다.
                </EmptyState>
            ) : null}
        </Section>
    );
}

function ConnectionCard({conn, onEdit, onChanged}: { conn: Connection; onEdit: () => void; onChanged: () => void }) {
    const action = useAction();
    const toast = useToast();
    const [report, setReport] = useState<CheckReport | null>(null);
    const [key, setKey] = useState('');
    const [model, setModel] = useState('');
    const isTest = conn.provider === 'test';
    return (
        <section className="card stack" aria-label={`연결 ${conn.name}`}>
            <div className="row between nowrap">
                <div className="row nowrap" style={{minWidth: 0}}>
                    <span className={`icon-wrap ${conn.usable ? 'ok' : 'warn'}`}><Icon icon={Bot} size={18}/></span>
                    <div className="stack tight" style={{minWidth: 0}}>
                        <div className="row"><strong>{conn.name}</strong>
                            {conn.provider === 'claude' && conn.settings.authMode === 'local_login' ? <StatusBadge tone="warn">개인용 로그인</StatusBadge> : null}</div>
                        <span className="muted xsmall">{kindLabel[conn.provider] ?? conn.provider}</span>
                    </div>
                </div>
                <StatusBadge tone={isTest ? 'info' : conn.usable ? 'ok' : 'bad'} icon={isTest ? FlaskConical : conn.usable ? CheckCircle2 : AlertTriangle}>
                    {isTest ? '테스트' : conn.usable ? '사용 가능' : '확인 필요'}</StatusBadge>
            </div>
            {conn.note ? <p className="muted small">{conn.note}</p> : null}
            {!isTest ? (
                <>
                    {needsKey(conn) ? (
                        <div className="row">
                            <span className="small row nowrap"><Icon icon={KeyRound} size={14}/>API 키 {conn.hasKey ? <StatusBadge tone="ok">저장됨</StatusBadge> : <StatusBadge tone="bad">없음</StatusBadge>}</span>
                            <input type="password" aria-label="API 키" placeholder={conn.hasKey ? '새 키로 바꾸기' : 'API 키 입력'} value={key}
                                   autoComplete="off" onChange={(e) => setKey(e.target.value)} style={{flex: 1, minWidth: 160}}/>
                            <Button size="sm" disabled={!key.trim() || action.busy}
                                    onClick={() => action.run(async () => { await api.setConnectionKey(conn.id, key); setKey(''); onChanged(); toast('키를 저장했습니다'); })}>키 저장</Button>
                            {conn.hasKey ? <ConfirmButton label="키 삭제" confirmLabel="키 삭제하기" className="btn small danger" disabled={action.busy}
                                                          onConfirm={() => action.run(async () => { await api.clearConnectionKey(conn.id); onChanged(); })}/> : null}
                        </div>
                    ) : null}
                    <div className="row">
                        <Button size="sm" icon={Pencil} onClick={onEdit}>{t.common.edit}</Button>
                        <Button size="sm" icon={Stethoscope} busy={action.busy}
                                onClick={() => action.run(async () => setReport(await api.checkConnection(conn.id)))}>연결 확인 (무료)</Button>
                        <span className="spacer"/>
                        <input aria-label="시험 모델" placeholder={conn.provider === 'openai_api' ? '모델 이름 (필수)' : '모델 (선택)'} value={model}
                               onChange={(e) => setModel(e.target.value)} style={{width: 160, minHeight: 28}}/>
                        <ConfirmButton label="실제 호출 시험" confirmLabel="AI 사용량을 써서 시험" className="btn small" disabled={action.busy} icon={Zap}
                                       onConfirm={() => action.run(async () => { setReport(await api.testConnectionCall(conn.id, model)); onChanged(); })}/>
                    </div>
                    {action.busy ? <p className="muted small" role="status">확인 중…</p> : null}
                    {report ? <ReportView report={report}/> : null}
                </>
            ) : null}
            <ErrorBox error={action.error}/>
        </section>
    );
}

const stepMark = {ok: CheckCircle2, failed: XCircle, skipped: MinusCircle};
const stepTone = {ok: 'ok', failed: 'bad', skipped: ''} as const;

function ReportView({report}: { report: CheckReport }) {
    return (
        <div className="card subtle tight stack">
            <ul className="stack tight small" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="확인 결과">
                {report.steps.map((s) => (
                    <li key={s.id} className="row nowrap" style={{alignItems: 'flex-start'}}>
                        <StatusBadge tone={stepTone[s.status]} icon={stepMark[s.status]}><span className="sr-only">{s.status}</span>{s.label}</StatusBadge>
                        <span className="muted" style={{minWidth: 0, overflowWrap: 'anywhere'}}>{s.detail}</span>
                    </li>
                ))}
            </ul>
            {report.ready ? <Banner tone="ok">실제 호출까지 확인됨 — 업무 실행 가능</Banner>
                : <Banner tone="warn">아직 실행할 수 없음 (실제 호출 시험 필요)</Banner>}
        </div>
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
        <form className="card stack" aria-label="연결 편집" onSubmit={(e) => { e.preventDefault(); save(); }} style={{borderColor: 'var(--primary)'}}>
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
                        <p className="banner warn" role="note">
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

            <div className="grid-2">
                <label className="field"><span>입력 단가 (USD/100만 토큰, 선택)</span>
                    <input inputMode="decimal" value={form.settings.inputPerMTok ?? ''} onChange={(e) => setS({inputPerMTok: num(e.target.value)})}/></label>
                <label className="field"><span>출력 단가 (USD/100만 토큰, 선택)</span>
                    <input inputMode="decimal" value={form.settings.outputPerMTok ?? ''} onChange={(e) => setS({outputPerMTok: num(e.target.value)})}/></label>
            </div>
            <p className="muted small">단가를 비워 두면 비용은 ‘알 수 없음’으로 표시됩니다 (0원으로 표시하지 않음).</p>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <button className="btn primary" type="submit" disabled={action.busy || !form.name.trim()}>{t.common.save}</button>
                {form.provider === 'codex' || form.provider === 'claude' ? <button className="btn" type="button" onClick={detect}>자동 탐지</button> : null}
                <button className="btn" type="button" onClick={onDone}>{t.common.cancel}</button>
            </div>
        </form>
    );
}
