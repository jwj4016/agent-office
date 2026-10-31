import {Building, Check, Sparkles, X} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {OrgApplied, OrgPlan, Project} from '../types';
import {Banner, Button, ErrorBox, Icon, Loading, relativeTime, Section, useAction, useToast} from '../ui';

// Organizations (company-requests plan, 4th revision): the departments a
// service works in and its intake desk, proposed by an AI.

const eventName: Record<string, string> = {
    'work.created': '업무 등록', 'work.completed': '업무 완료', 'document.shared': '자료 공유', 'document.revised': '자료 개정',
};

// OrgPlanView shows a planned organization; name turns a member ref or id
// into a person's name.
export function OrgPlanView({plan, name}: { plan: OrgPlan; name: (ref: string) => string }) {
    return (
        <div className="stack tight">
            {(plan.departments ?? []).map((d) => (
                <div key={d.name} className="card subtle tight stack tight" role="group" aria-label={`부서 ${d.name}`}>
                    <span className="row"><strong>{d.name}</strong><span className="small muted">{d.responsibility}</span></span>
                    <span className="small">구성원: {(d.members ?? []).map((m) => (m === d.lead ? `${name(m)}(대표)` : name(m))).join(', ')}</span>
                    {(d.watches ?? []).map((w, i) => (
                        <span key={i} className="small muted">받아 봄: {(w.eventKinds ?? []).map((k) => eventName[k] ?? k).join('·')} — {w.guidance}</span>
                    ))}
                </div>
            ))}
            {plan.intakeDesk ? (
                <div className="card subtle tight stack tight" role="group" aria-label="접수 담당">
                    <strong>접수 담당: {plan.intakeDesk.name}</strong>
                    <details><summary className="small">지침 보기</summary><p className="small pre">{plan.intakeDesk.instructions}</p></details>
                </div>
            ) : null}
        </div>
    );
}

export function appliedText(a: OrgApplied): string {
    const parts = [`부서 ${a.departments.length}개`];
    if (a.desk) parts.push(`접수 담당 ‘${a.desk}’`);
    if (a.adoptedWork) parts.push(`업무 ${a.adoptedWork}건을 부서로 옮김`);
    return parts.join(', ') + '을 만들었습니다' + (a.autoStart ? '(부서 자동 시작 켬)' : '');
}

// OrganizeService lets an AI propose an existing service's organization;
// the person applies or sets it aside.
export function OrganizeService({project, onApplied}: { project: Project; onApplied?: () => void }) {
    const view = useLoad(() => api.latestOrgProposal(project.id), [project.id], project.id);
    const action = useAction();
    const toast = useToast();
    const [allow, setAllow] = useState<string[]>([]);
    // The suggested connections start ticked once per proposal; a reload
    // keeps what the person changed.
    const proposalId = view.data?.proposal?.id;
    useEffect(() => {
        setAllow((view.data?.suggestAllow ?? []).map((c) => c.id));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [proposalId]);
    if (view.error) return <ErrorBox error={view.error}/>;
    if (!view.data) return <Loading rows={1}/>;
    const v = view.data;
    const p = v.proposal;
    const archived = project.status === 'archived';
    const name = (id: string) => v.names[id] ?? id;
    const propose = () => action.run(async () => { await api.proposeOrganization(project.id); view.reload(); });
    return (
        <Section title={<><Icon icon={Building} size={18}/>조직 정비</>} label="조직 정비">
            <div className="card stack">
                <p className="small muted">AI가 아직 부서에 없는 담당자를 함께 일하는 부서로 묶고, 접수 담당이 없으면 함께 제안합니다. 보고 적용하세요. AI 호출 한 번의 비용이 듭니다.</p>
                {p?.status === 'ready' ? (
                    <>
                        <OrgPlanView plan={p.plan} name={name}/>
                        {v.autoMode ? (
                            <div className="stack tight">
                                <span className="small">자동 모드라 새 부서는 업무를 스스로 시작합니다(초안 공유·참고용 협업 결정 포함).</span>
                                {v.suggestAllow.length ? (
                                    <div className="stack tight" role="group" aria-label="자동 실행에 허용할 연결">
                                        <span className="small">자동 실행에 허용할 AI 연결(지금 허용: {v.allowed.map((c) => c.name).join(', ') || '없음'})</span>
                                        {v.suggestAllow.map((c) => (
                                            <label key={c.id} className="check block">
                                                <input type="checkbox" checked={allow.includes(c.id)}
                                                       onChange={(e) => setAllow(e.target.checked ? [...allow, c.id] : allow.filter((x) => x !== c.id))}/>{c.name}
                                            </label>
                                        ))}
                                    </div>
                                ) : null}
                                {!v.hasBudget ? <Banner tone="warn" title="서비스 예산이 없습니다">예산이 없으면 자동 시작하지 않고 ‘자동 시작 검토’로 내 할 일에 올라옵니다. 설정 탭에서 예산을 정하세요.</Banner> : null}
                            </div>
                        ) : null}
                        <div className="row">
                            <Button variant="primary" icon={Check} disabled={archived} busy={action.busy} onClick={() => action.run(async () => {
                                const out = await api.applyOrganization(project.id, p.id, allow);
                                toast(appliedText(out));
                                view.reload();
                                onApplied?.();
                            })}>적용</Button>
                            <Button variant="ghost" icon={X} busy={action.busy} onClick={() => action.run(async () => { await api.discardOrgProposal(project.id, p.id); view.reload(); })}>버리기</Button>
                        </div>
                    </>
                ) : (
                    <>
                        {p?.status === 'applied' && p.applied ? <p className="small muted">{relativeTime(p.endedAt || p.startedAt)} 적용: {appliedText(p.applied)}</p> : null}
                        {p?.status === 'applied' && p.applied?.problems.length ? <Banner tone="warn" title="일부를 만들지 못했습니다">{p.applied.problems.join(' · ')}</Banner> : null}
                        {p?.status === 'failed' ? <Banner tone="bad" title="지난 제안이 실패했습니다">{p.error}</Banner> : null}
                        <div className="row"><Button icon={Sparkles} disabled={archived} busy={action.busy} onClick={propose}>AI에게 조직 제안받기</Button></div>
                    </>
                )}
                <ErrorBox error={action.error}/>
            </div>
        </Section>
    );
}
