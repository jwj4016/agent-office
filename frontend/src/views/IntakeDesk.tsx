import {Save, Trash2, UserCheck} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {IntakeDesk, Project} from '../types';
import {Banner, Button, ErrorBox, Field, Icon, Loading, Section, useAction, useToast} from '../ui';

// Intake desks (company-requests plan §4.1): the AI that takes requests,
// one for the company and one per service.

// ServiceDeskSettings picks which of the service's AI assignees takes its
// requests.
export function ServiceDeskSettings({project}: { project: Project }) {
    const desk = useLoad(() => api.deskView(project.id), [project.id], project.id);
    const people = useLoad(() => api.assignments(project.id), [project.id], project.id);
    const [form, setForm] = useState<IntakeDesk | null>(null);
    const action = useAction();
    const toast = useToast();
    useEffect(() => {
        if (desk.data) setForm({...desk.data.desk, projectId: project.id, enabled: desk.data.exists ? desk.data.desk.enabled : true});
    }, [desk.data, project.id]);
    if (desk.error || people.error) return <ErrorBox error={desk.error || people.error}/>;
    if (!form || !people.data) return <Loading rows={1}/>;
    const ais = people.data.filter((a) => a.actorKind === 'ai');
    const archived = project.status === 'archived';
    const save = () => action.run(async () => {
        await api.saveDesk(form);
        toast('접수 담당을 저장했습니다');
        desk.reload();
    });
    return (
        <Section title={<><Icon icon={UserCheck} size={18}/>접수 담당</>} label="접수 담당">
            <div className="card stack">
                <p className="small muted">‘요청’ 탭이나 회사 요청에서 이 서비스로 온 요청을 받아, 답하거나 업무·조치를 제안하는 AI 담당자입니다.</p>
                {ais.length === 0 ? <Banner tone="warn">AI 담당자가 없습니다. 먼저 담당자를 추가하세요.</Banner> : null}
                <Field label="접수 담당자">
                    <select value={form.assignmentId} disabled={archived} onChange={(e) => setForm({...form, assignmentId: e.target.value})}>
                        <option value="">고르세요</option>
                        {ais.map((a) => <option key={a.id} value={a.id}>{a.displayName}</option>)}
                    </select>
                </Field>
                <Field label="접수 지침" optional hint="담당자 지침에 더해 접수할 때만 쓰는 지침입니다.">
                    <textarea rows={2} value={form.instructions} disabled={archived} onChange={(e) => setForm({...form, instructions: e.target.value})}/>
                </Field>
                <label className="check block">
                    <input type="checkbox" checked={form.enabled} disabled={archived} onChange={(e) => setForm({...form, enabled: e.target.checked})}/>
                    요청 받기
                </label>
                <div className="row">
                    <Button variant="primary" icon={Save} disabled={!form.assignmentId || archived} busy={action.busy} onClick={save}>저장</Button>
                    {desk.data?.exists && !archived ? (
                        <Button variant="ghost" icon={Trash2} busy={action.busy}
                                onClick={() => action.run(async () => { await api.removeDesk(project.id); toast('접수 담당을 지웠습니다'); desk.reload(); })}>
                            접수 담당 지우기</Button>
                    ) : null}
                </div>
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
        setForm({...desk.data.desk, projectId: '', name: desk.data.desk.name || '비서실', enabled: desk.data.exists ? desk.data.desk.enabled : true});
        setBudget(desk.data.desk.budget?.monthlyCostUsd ? String(desk.data.desk.budget.monthlyCostUsd) : '');
    }, [desk.data]);
    if (desk.error || conns.error) return <ErrorBox error={desk.error || conns.error}/>;
    if (!form || !conns.data) return <Loading rows={1}/>;
    const usage = desk.data?.monthUsage;
    const save = () => action.run(async () => {
        const n = budget.trim() ? Number(budget) : undefined;
        if (n !== undefined && !(n > 0)) throw new Error('월 예산은 0보다 큰 숫자로 적어 주세요 (제한하지 않으려면 비워 두세요)');
        await api.saveDesk({...form, budget: n ? {monthlyCostUsd: n} : {}});
        toast('회사 접수 담당을 저장했습니다');
        desk.reload();
    });
    return (
        <Section title={<><Icon icon={UserCheck} size={18}/>회사 접수 담당</>} label="회사 접수 담당">
            <div className="card stack">
                <p className="small muted">‘회사에 요청’으로 온 요청을 읽고 어느 서비스의 일인지 가려 넘기는 AI입니다. 서비스 안의 일은 각 서비스의 접수 담당이 정합니다.</p>
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
                    <textarea rows={2} value={form.instructions} onChange={(e) => setForm({...form, instructions: e.target.value})}/>
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
