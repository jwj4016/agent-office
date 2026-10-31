import {PenLine, Save, Sparkles, Trash2, UserCheck, UserPlus, Users} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {Connection, DeskView, IntakeDesk, Project} from '../types';
import {Banner, Button, ErrorBox, Field, Icon, Loading, relativeTime, Section, useAction, useToast} from '../ui';

// Intake desks (company-requests plan §4.1, 3rd revision): the AI that
// takes requests, one for the company and one per service. A missing desk
// is set up by an AI when a request arrives; the person can also ask the
// AI, write one themselves, or pick one of the service's AI assignees.

type Mode = 'ai' | 'new' | 'existing';

const connName = (conns: Connection[], id: string) => conns.find((c) => c.id === id)?.name ?? '';

// StaffingNote says when an AI last wrote this desk.
function StaffingNote({view}: { view: DeskView }) {
    const s = view.lastStaffing;
    if (!s) return null;
    if (s.status === 'failed') return <p className="small muted">AI가 접수 담당을 쓰지 못했습니다({relativeTime(s.startedAt)}): {s.error}</p>;
    if (s.status !== 'succeeded') return null;
    return (
        <p className="small muted">
            {s.kind === 'create' ? 'AI가 서비스를 분석해 배치했습니다' : 'AI가 지침을 다시 썼습니다'} · {relativeTime(s.endedAt || s.startedAt)}
            {s.requestId ? ' · 요청을 받으며 자동으로' : ''}
        </p>
    );
}

// ServiceDeskSettings sets up who takes the service's requests.
export function ServiceDeskSettings({project}: { project: Project }) {
    const desk = useLoad(() => api.deskView(project.id), [project.id], project.id);
    const people = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const conns = useLoad(() => api.connections(), []);
    const [mode, setMode] = useState<Mode>('ai');
    const [pick, setPick] = useState({assignmentId: '', instructions: ''});
    const [fresh, setFresh] = useState({name: `${project.name} 접수 담당`, connectionId: '', model: '', instructions: ''});
    const action = useAction();
    const toast = useToast();
    useEffect(() => {
        if (!desk.data) return;
        setPick({assignmentId: desk.data.desk.assignmentId, instructions: desk.data.desk.instructions});
        setFresh((f) => ({...f, connectionId: f.connectionId || desk.data!.suggestedConnectionId, model: f.model || desk.data!.suggestedModel}));
    }, [desk.data]);
    if (desk.error || people.error || conns.error) return <ErrorBox error={desk.error || people.error || conns.error}/>;
    if (!desk.data || !people.data || !conns.data) return <Loading rows={1}/>;
    const view = desk.data;
    const ais = people.data.filter((a) => a.actorKind === 'ai');
    const archived = project.status === 'archived';
    const run = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); toast(done); desk.reload(); people.reload(); });
    const suggested = connName(conns.data, view.suggestedConnectionId);

    return (
        <Section title={<><Icon icon={UserCheck} size={18}/>접수 담당</>} label="접수 담당">
            <div className="card stack">
                <p className="small muted">‘요청’ 탭이나 회사 요청에서 이 서비스로 온 요청을 받아, 답하거나 업무·조치를 제안하는 AI 담당자입니다.
                    접수 담당 없이 요청이 오면 AI가 서비스를 분석해 새 접수 담당을 배치하고 이어서 처리합니다.</p>

                {view.exists ? (
                    <div className="card subtle tight stack tight" role="group" aria-label="지금 접수 담당">
                        <span className="row"><strong>{view.assignee?.displayName ?? view.desk.name}</strong>
                            <span className="muted small">{connName(conns.data, view.assignee?.connectionId ?? '')}{view.assignee?.model ? ` · ${view.assignee.model}` : ''}</span></span>
                        <StaffingNote view={view}/>
                        {view.assignee?.overrides.instructions ? (
                            <details><summary className="small">담당자 지침 보기</summary><p className="small pre">{view.assignee.overrides.instructions}</p></details>
                        ) : null}
                        <label className="check block">
                            <input type="checkbox" checked={view.desk.enabled} disabled={archived}
                                   onChange={(e) => run(() => api.saveDesk({...view.desk, enabled: e.target.checked}), e.target.checked ? '요청을 받습니다' : '요청 받기를 껐습니다')}/>
                            요청 받기
                        </label>
                        {!archived ? (
                            <div className="row">
                                <Button size="sm" icon={PenLine} busy={action.busy} onClick={() => run(() => api.rewriteDesk(project.id), 'AI가 지침을 다시 썼습니다')}>AI로 지침 다시 쓰기</Button>
                                <Button size="sm" variant="ghost" icon={Trash2} busy={action.busy} onClick={() => run(() => api.removeDesk(project.id), '접수 담당을 지웠습니다')}>접수 담당 지우기</Button>
                            </div>
                        ) : null}
                    </div>
                ) : <Banner tone="info">아직 접수 담당이 없습니다. 아래에서 정하거나, 요청을 보내면 AI가 배치합니다.</Banner>}
                {view.exists ? null : <StaffingNote view={view}/>}

                {!archived ? (
                    <>
                        <div className="segmented" role="group" aria-label={view.exists ? '접수 담당 바꾸기' : '접수 담당 정하기'}>
                            <button type="button" className="btn small" aria-pressed={mode === 'ai'} onClick={() => setMode('ai')}><Icon icon={Sparkles} size={14}/>AI에게 맡기기</button>
                            <button type="button" className="btn small" aria-pressed={mode === 'new'} onClick={() => setMode('new')}><Icon icon={UserPlus} size={14}/>새로 만들기</button>
                            <button type="button" className="btn small" aria-pressed={mode === 'existing'} onClick={() => setMode('existing')}><Icon icon={Users} size={14}/>기존 인원 중 지정</button>
                        </div>
                        {mode === 'ai' ? (
                            <div className="stack tight">
                                <p className="small">AI가 서비스의 목표·지침·부서·담당자를 읽고 새 접수 담당(공통 역할 ‘접수 담당’)과 이 서비스에 맞춘 지침을 만듭니다.
                                    {suggested ? ` 연결은 이 서비스에서 가장 많이 쓰는 ‘${suggested}’입니다.` : ''} AI 호출 한 번의 비용이 듭니다.</p>
                                {!suggested ? <Banner tone="warn">실제 호출 시험을 통과한 AI 연결이 없습니다. 연결 화면에서 먼저 확인하세요.</Banner> : null}
                                <div className="row"><Button variant="primary" icon={Sparkles} disabled={!suggested} busy={action.busy}
                                                             onClick={() => run(() => api.staffDesk(project.id), 'AI가 접수 담당을 배치했습니다')}>AI에게 맡기기</Button></div>
                            </div>
                        ) : null}
                        {mode === 'new' ? (
                            <div className="stack tight">
                                <Field label="이름"><input value={fresh.name} onChange={(e) => setFresh({...fresh, name: e.target.value})}/></Field>
                                <Field label="AI 연결">
                                    <select value={fresh.connectionId} onChange={(e) => setFresh({...fresh, connectionId: e.target.value})}>
                                        <option value="">고르세요</option>
                                        {conns.data.map((c) => <option key={c.id} value={c.id}>{c.name}{c.usable ? '' : ' (확인 필요)'}</option>)}
                                    </select>
                                </Field>
                                <Field label="모델" optional hint="비우면 연결의 기본 모델을 씁니다."><input value={fresh.model} onChange={(e) => setFresh({...fresh, model: e.target.value})}/></Field>
                                <Field label="담당자 지침" optional hint="공통 역할 ‘접수 담당’의 지침에 더해, 이 서비스에서만 쓰는 지침입니다.">
                                    <textarea rows={3} value={fresh.instructions} onChange={(e) => setFresh({...fresh, instructions: e.target.value})}/>
                                </Field>
                                <div className="row"><Button variant="primary" icon={UserPlus} disabled={!fresh.name.trim() || !fresh.connectionId} busy={action.busy}
                                                             onClick={() => run(() => api.createDeskAssignee({projectId: project.id, ...fresh}), '새 접수 담당을 만들었습니다')}>만들기</Button></div>
                            </div>
                        ) : null}
                        {mode === 'existing' ? (
                            <div className="stack tight">
                                {ais.length === 0 ? <Banner tone="warn">AI 담당자가 없습니다.</Banner> : null}
                                <Field label="접수 담당자">
                                    <select value={pick.assignmentId} onChange={(e) => setPick({...pick, assignmentId: e.target.value})}>
                                        <option value="">고르세요</option>
                                        {ais.map((a) => <option key={a.id} value={a.id}>{a.displayName}</option>)}
                                    </select>
                                </Field>
                                <Field label="접수 지침" optional hint="담당자 지침에 더해 접수할 때만 쓰는 지침입니다.">
                                    <textarea rows={2} value={pick.instructions} onChange={(e) => setPick({...pick, instructions: e.target.value})}/>
                                </Field>
                                <div className="row"><Button variant="primary" icon={Save} disabled={!pick.assignmentId} busy={action.busy}
                                                             onClick={() => run(() => api.saveDesk({projectId: project.id, name: '', assignmentId: pick.assignmentId, connectionId: '',
                                                                 model: '', instructions: pick.instructions, budget: {}, enabled: true}), '접수 담당을 정했습니다')}>저장</Button></div>
                            </div>
                        ) : null}
                    </>
                ) : null}
                <ErrorBox error={action.error}/>
            </div>
        </Section>
    );
}

// CompanyDeskSettings sets up the company desk: its own AI connection and
// model (the company has no assignees), and an optional monthly budget.
export function CompanyDeskSettings() {
    const desk = useLoad(() => api.deskView(''), [], '*');
    const conns = useLoad(() => api.connections(), []);
    const [form, setForm] = useState<IntakeDesk | null>(null);
    const [budget, setBudget] = useState('');
    const action = useAction();
    const toast = useToast();
    useEffect(() => {
        if (!desk.data) return;
        const d = desk.data;
        setForm({...d.desk, projectId: '', name: d.desk.name || '비서실', enabled: d.exists ? d.desk.enabled : true,
            connectionId: d.desk.connectionId || d.suggestedConnectionId, model: d.exists ? d.desk.model : d.suggestedModel});
        setBudget(d.desk.budget?.monthlyCostUsd ? String(d.desk.budget.monthlyCostUsd) : '');
    }, [desk.data]);
    if (desk.error || conns.error) return <ErrorBox error={desk.error || conns.error}/>;
    if (!form || !conns.data || !desk.data) return <Loading rows={1}/>;
    const view = desk.data;
    const usage = view.monthUsage;
    const save = () => action.run(async () => {
        const n = budget.trim() ? Number(budget) : undefined;
        if (n !== undefined && !(n > 0)) throw new Error('월 예산은 0보다 큰 숫자로 적어 주세요 (제한하지 않으려면 비워 두세요)');
        await api.saveDesk({...form, budget: n ? {monthlyCostUsd: n} : {}});
        toast('회사 접수 담당을 저장했습니다');
        desk.reload();
    });
    const ai = (fn: () => Promise<unknown>, done: string) => action.run(async () => { await fn(); toast(done); desk.reload(); });
    return (
        <Section title={<><Icon icon={UserCheck} size={18}/>회사 접수 담당</>} label="회사 접수 담당">
            <div className="card stack">
                <p className="small muted">‘회사에 요청’으로 온 요청을 읽고 어느 서비스의 일인지 가려 넘기는 AI입니다. 서비스 안의 일은 각 서비스의 접수 담당이 정합니다.
                    회사 접수 담당 없이 요청이 오면 AI가 회사를 분석해 배치합니다.</p>
                <StaffingNote view={view}/>
                <div className="row">
                    <Button size="sm" icon={Sparkles} disabled={!view.suggestedConnectionId} busy={action.busy}
                            onClick={() => ai(() => api.staffDesk(''), 'AI가 회사 접수 담당을 배치했습니다')}>{view.exists ? 'AI에게 새로 맡기기' : 'AI에게 맡기기'}</Button>
                    {view.exists ? <Button size="sm" icon={PenLine} busy={action.busy}
                                           onClick={() => ai(() => api.rewriteDesk(''), 'AI가 지침을 다시 썼습니다')}>AI로 지침 다시 쓰기</Button> : null}
                </div>
                <Field label="이름"><input value={form.name} onChange={(e) => setForm({...form, name: e.target.value})}/></Field>
                <Field label="AI 연결" hint="실제 호출 시험을 통과한 연결만 쓸 수 있습니다.">
                    <select value={form.connectionId} onChange={(e) => setForm({...form, connectionId: e.target.value})}>
                        <option value="">고르세요</option>
                        {conns.data.map((c) => <option key={c.id} value={c.id}>{c.name}{c.usable ? '' : ' (확인 필요)'}</option>)}
                    </select>
                </Field>
                <Field label="모델" optional hint="비우면 연결의 기본 모델을 씁니다.">
                    <input value={form.model} onChange={(e) => setForm({...form, model: e.target.value})}/>
                </Field>
                <Field label="접수 지침" optional>
                    <textarea rows={3} value={form.instructions} onChange={(e) => setForm({...form, instructions: e.target.value})}/>
                </Field>
                <Field label="월 예산 (USD)" optional hint="보고된 비용이 이번 달 예산에 닿으면 회사 접수를 보류합니다.">
                    <input inputMode="decimal" value={budget} onChange={(e) => setBudget(e.target.value)}/>
                </Field>
                {usage ? (
                    <p className="small muted">이번 달: 호출 {usage.Attempts}회 · 보고된 비용 ${usage.CostUSD.toFixed(4)}
                        {usage.UnknownCost ? ` · 비용을 알 수 없는 호출 ${usage.UnknownCost}회` : ''}</p>
                ) : null}
                <label className="check block">
                    <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({...form, enabled: e.target.checked})}/>
                    요청 받기
                </label>
                <div className="row"><Button variant="primary" icon={Save} disabled={!form.connectionId} busy={action.busy} onClick={save}>저장</Button></div>
                <ErrorBox error={action.error}/>
            </div>
        </Section>
    );
}
