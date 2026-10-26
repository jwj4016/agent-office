import {Check, CheckCheck, CircleHelp, Lightbulb, Pencil, Radio, Send, X, XCircle} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Decisions, Department, Judgment, Project, Proposal, Subscription, WorkPriority} from '../types';
import {Button, DetailHeader, ErrorBox, Field, Icon, Loading, Section, StatusBadge, relativeTime, useAction, useDetail, useToast} from '../ui';
import {DocumentReviewRequest, eventLabel} from './Deliveries';
import {ScheduleSection} from './Schedule';

// The kinds a department can follow, in the order shown. A confirmed
// document only notifies by default (plan decision 4A), so it starts
// unchecked; direct requests reach a department without a subscription.
const subscribable = ['document.shared', 'document.revised', 'document.confirmed', 'document.withdrawn', 'work.created', 'work.completed', 'work.reopened'];
const defaultKinds = subscribable.filter((k) => k !== 'document.confirmed');

// SubscriptionPanel sets which events a department judges and from what
// point of view, in the detail pane.
export function SubscriptionPanel({project, department}: { project: Project; department: Department }) {
    const list = useLoad(() => api.subscriptions(project.id, department.id), [project.id, department.id], project.id);
    const categories = useLoad(() => api.categories(project.id), [project.id]);
    const setDetail = useDetail();
    const [adding, setAdding] = useState(false);
    if (list.error || categories.error) return <ErrorBox error={list.error || categories.error}/>;
    if (!list.data || !categories.data) return <Loading/>;
    const fresh = list.data.length === 0 || adding;
    return (
        <div className="stack">
            <DetailHeader title={`${department.name} 판단 설정`} onClose={() => setDetail(null)}/>
            <p className="muted small">고른 사건이 생기면 이 부서의 대표 AI가 할 일이 있는지 판단해 업무를 제안합니다.
                사건의 분류가 하나라도 겹칠 때만 받으려면 분류를 고르세요. 부서 점검 요청은 구독 없이도 받습니다.
                구독은 저장한 뒤에 생기는 사건부터 받습니다. 그전에 공유된 자료는 아래에서 골라 검토시키세요.</p>
            {department.leadAssignmentId ? null : <p className="small" role="note">대표 담당자가 없으면 판단하지 못합니다. 부서 편집에서 지정하세요.</p>}
            {list.data.map((s) => <SubscriptionForm key={s.id} project={project} department={department} initial={s}
                                                    categories={categories.data ?? []} onSaved={list.reload}/>)}
            {fresh ? (
                <SubscriptionForm key={`new-${list.data.length}`} project={project} department={department} categories={categories.data}
                                  onSaved={() => { setAdding(false); list.reload(); }}/>
            ) : <Button size="sm" icon={Radio} onClick={() => setAdding(true)}>구독 추가</Button>}
            <DocumentReviewRequest project={project} departments={[department]} departmentId={department.id}
                                   disabled={project.status === 'archived' || department.status !== 'active'} onSent={() => {}}/>
            <ScheduleSection project={project} department={department}/>
        </div>
    );
}

function SubscriptionForm({project, department, initial, categories, onSaved}: {
    project: Project; department: Department; initial?: Subscription; categories: string[]; onSaved: () => void;
}) {
    const [form, setForm] = useState({
        eventKinds: initial?.eventKinds ?? defaultKinds, categories: initial?.categories ?? [] as string[],
        guidance: initial?.guidance ?? '', enabled: initial?.enabled ?? true,
    });
    const action = useAction();
    const toast = useToast();
    const archived = project.status === 'archived';
    const toggle = (key: 'eventKinds' | 'categories', v: string, on: boolean) =>
        setForm({...form, [key]: on ? [...form[key], v] : form[key].filter((x) => x !== v)});
    const save = () => action.run(async () => {
        await api.saveSubscription({id: initial?.id, projectId: project.id, departmentId: department.id, ...form});
        toast(`${department.name} 구독을 저장했습니다`);
        onSaved();
    });
    return (
        <form className="card stack" aria-label={initial ? '구독 편집' : '새 구독'} onSubmit={(e) => { e.preventDefault(); save(); }}>
            <fieldset className="stack tight">
                <legend className="field-label">받을 사건</legend>
                {subscribable.map((k) => (
                    <label key={k} className="check block">
                        <input type="checkbox" checked={form.eventKinds.includes(k)} disabled={archived} onChange={(e) => toggle('eventKinds', k, e.target.checked)}/>
                        {eventLabel(k)}{k === 'document.confirmed' ? <span className="muted xsmall"> (확정만으로는 보통 새 일이 생기지 않습니다)</span> : null}
                    </label>
                ))}
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">분류 <span className="optional">(선택: 고르지 않으면 모든 분류)</span></legend>
                <span className="row">
                    {categories.length === 0 ? <span className="muted small">서비스 설정에서 분류를 만드세요.</span> : null}
                    {categories.map((c) => (
                        <label key={c} className="check">
                            <input type="checkbox" checked={form.categories.includes(c)} disabled={archived} onChange={(e) => toggle('categories', c, e.target.checked)}/>{c}
                        </label>
                    ))}
                </span>
            </fieldset>
            <Field label="판단 관점" optional hint="예: 개발이 필요한 기능 변경이 있는지 본다. 판단 지시에 그대로 들어갑니다.">
                <textarea rows={2} value={form.guidance} disabled={archived} onChange={(e) => setForm({...form, guidance: e.target.value})}/></Field>
            <label className="check">
                <input type="checkbox" checked={form.enabled} disabled={archived} onChange={(e) => setForm({...form, enabled: e.target.checked})}/>사용
            </label>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={archived || form.eventKinds.length === 0}>{t.common.save}</Button>
            </div>
        </form>
    );
}

// QuestionCard is a department's question: answer it (the department
// judges once more) or close it after deciding yourself.
export function QuestionCard({j, onDone, showProject}: { j: Judgment; onDone: () => void; showProject?: boolean }) {
    const [answer, setAnswer] = useState('');
    const action = useAction();
    const toast = useToast();
    const manual = j.questionStatus === 'manual';
    const send = () => action.run(async () => {
        await api.answerJudgment(j.projectId, j.id, answer);
        toast(`${j.departmentName}에 답했습니다. 다시 판단합니다`);
        onDone();
    });
    const close = () => action.run(async () => {
        await api.closeJudgmentQuestion(j.projectId, j.id, answer);
        toast('질문을 닫았습니다');
        onDone();
    });
    return (
        <article className="card stack tight" aria-label={`${j.departmentName} 질문`}>
            <span className="row nowrap small">
                <Icon icon={CircleHelp} size={16}/>
                <strong>{showProject ? `${j.projectName} · ` : ''}{j.departmentName}</strong>
                <span className="muted xsmall">{eventLabel(j.eventKind)}{j.event.title ? ` · ${j.event.title}` : ''}</span>
                <span className="spacer"/>
                <span className="muted xsmall">{relativeTime(j.createdAt)}</span>
            </span>
            <p className="pre" style={{margin: 0}}>{j.question}</p>
            {manual ? <p className="muted small" style={{margin: 0}}>답변 뒤에도 다시 물었습니다. 직접 결정하고 닫으세요.</p> : null}
            <form className="row" onSubmit={(e) => { e.preventDefault(); if (answer.trim() && !manual) send(); }}>
                <input value={answer} style={{flex: 1, minWidth: 200}} aria-label={`${j.departmentName} 질문에 대한 ${manual ? '메모' : '답변'}`}
                       placeholder={manual ? '어떻게 결정했는지 (선택)' : '답변'} onChange={(e) => setAnswer(e.target.value)}/>
                {manual ? null : <Button type="submit" size="sm" variant="primary" icon={Send} busy={action.busy} disabled={!answer.trim()}>답하기</Button>}
                <Button size="sm" variant="ghost" icon={X} busy={action.busy} onClick={close}>{manual ? '닫기' : '답하지 않고 닫기'}</Button>
            </form>
            <ErrorBox error={action.error}/>
        </article>
    );
}

// groupByEvent keeps the proposals one event caused together.
export function groupByEvent(list: Proposal[]) {
    const groups = new Map<string, Proposal[]>();
    for (const p of list) {
        const key = `${p.projectId}:${p.eventSequence}`;
        groups.set(key, [...(groups.get(key) ?? []), p]);
    }
    return [...groups.values()];
}

// ProposalGroup is the work one event made departments propose, decided
// one by one or all at once.
export function ProposalGroup({proposals, onDone, showProject, readOnly}: {
    proposals: Proposal[]; onDone: () => void; showProject?: boolean; readOnly?: boolean;
}) {
    const first = proposals[0];
    const action = useAction();
    const toast = useToast();
    const [batchErrors, setBatchErrors] = useState<string[]>([]);
    const all = (accept: boolean) => action.run(async () => {
        const res = await api.decideProposals(first.projectId, proposals.map((p) => p.id), accept);
        setBatchErrors(res.errors);
        toast(accept ? `업무 ${res.done}건을 등록했습니다` : `제안 ${res.done}건을 거절했습니다`);
        onDone();
    });
    return (
        <section className="card stack" aria-label={`${eventLabel(first.eventKind)} ${first.eventTitle} 제안`}>
            <div className="row nowrap small">
                <Icon icon={Lightbulb} size={16}/>
                <strong>{showProject ? `${first.projectName} · ` : ''}{eventLabel(first.eventKind)}{first.eventTitle ? ` · ${first.eventTitle}` : ''}</strong>
                <span className="spacer"/>
                {proposals.length > 1 && !readOnly ? (
                    <span className="row nowrap">
                        <Button size="sm" icon={CheckCheck} busy={action.busy} onClick={() => all(true)}>모두 등록</Button>
                        <Button size="sm" variant="ghost" icon={XCircle} busy={action.busy} onClick={() => all(false)}>모두 거절</Button>
                    </span>
                ) : null}
            </div>
            <ErrorBox error={action.error || (batchErrors.length ? batchErrors.join('\n') : null)}/>
            {proposals.map((p) => <ProposalCard key={p.id} p={p} onDone={onDone} readOnly={readOnly}/>)}
        </section>
    );
}

const priorities: WorkPriority[] = ['urgent', 'high', 'normal', 'low'];

function ProposalCard({p, onDone, readOnly}: { p: Proposal; onDone: () => void; readOnly?: boolean }) {
    const [editing, setEditing] = useState(false);
    const [rejecting, setRejecting] = useState(false);
    const [form, setForm] = useState({title: p.title, doneCriteria: p.doneCriteria, priority: p.priority});
    const [note, setNote] = useState('');
    const action = useAction();
    const toast = useToast();
    const accept = () => action.run(async () => {
        const w = await api.acceptProposal(p.projectId, p.id, editing ? form : {});
        toast(`‘${w.title}’ 업무를 ${p.departmentName}에 등록했습니다`);
        onDone();
    });
    const reject = () => action.run(async () => {
        await api.rejectProposal(p.projectId, p.id, note);
        toast('제안을 거절했습니다');
        onDone();
    });
    return (
        <article className="stack tight" aria-label={`${p.departmentName} 제안 ${p.title}`} style={{borderTop: '1px solid var(--border)', paddingTop: 8}}>
            <span className="row small">
                <strong>{p.departmentName}</strong>
                <span>{p.title}</span>
                <StatusBadge>{t.workPriority[p.priority]}</StatusBadge>
                {p.categories.map((c) => <span key={c} className="badge">{c}</span>)}
                {p.duplicateOf ? <StatusBadge tone="warn">비슷한 업무가 이미 있음</StatusBadge> : null}
            </span>
            {p.doneCriteria ? <span className="muted xsmall">완료 기준: {p.doneCriteria}</span> : null}
            {p.description ? <span className="small pre">{p.description}</span> : null}
            <span className="muted xsmall">판단 이유: {p.reason}</span>
            {readOnly ? null : editing ? (
                <form className="stack tight" aria-label="제안 고쳐서 등록" onSubmit={(e) => { e.preventDefault(); accept(); }}>
                    <Field label="제목"><input value={form.title} onChange={(e) => setForm({...form, title: e.target.value})}/></Field>
                    <Field label="완료 기준" optional><input value={form.doneCriteria} onChange={(e) => setForm({...form, doneCriteria: e.target.value})}/></Field>
                    <Field label="우선순위">
                        <select value={form.priority} onChange={(e) => setForm({...form, priority: e.target.value as WorkPriority})}>
                            {priorities.map((x) => <option key={x} value={x}>{t.workPriority[x]}</option>)}
                        </select></Field>
                    <span className="row nowrap">
                        <Button type="submit" size="sm" variant="primary" icon={Check} busy={action.busy} disabled={!form.title.trim()}>등록</Button>
                        <Button size="sm" onClick={() => setEditing(false)}>{t.common.cancel}</Button>
                    </span>
                </form>
            ) : rejecting ? (
                <form className="row" aria-label="제안 거절" onSubmit={(e) => { e.preventDefault(); reject(); }}>
                    <input value={note} autoFocus style={{flex: 1, minWidth: 200}} aria-label="거절 이유" placeholder="거절 이유 (선택)" onChange={(e) => setNote(e.target.value)}/>
                    <Button type="submit" size="sm" variant="danger" busy={action.busy}>거절</Button>
                    <Button size="sm" onClick={() => setRejecting(false)}>{t.common.cancel}</Button>
                </form>
            ) : (
                <span className="row nowrap">
                    <Button size="sm" variant="primary" icon={Check} busy={action.busy} aria-label={`${p.title} 업무로 등록`} onClick={accept}>업무로 등록</Button>
                    <Button size="sm" icon={Pencil} aria-label={`${p.title} 고쳐서 등록`} onClick={() => setEditing(true)}>고쳐서 등록</Button>
                    <Button size="sm" variant="ghost" icon={X} aria-label={`${p.title} 거절`} onClick={() => setRejecting(true)}>거절</Button>
                </span>
            )}
            <ErrorBox error={action.error}/>
        </article>
    );
}

// DepartmentDecisions is what this service's departments wait on a person
// for: their questions and the work they proposed.
export function DepartmentDecisions({project}: { project: Project }) {
    const questions = useLoad(() => api.judgments(project.id, 'open,manual'), [project.id], project.id);
    const proposals = useLoad(() => api.proposals(project.id, 'proposed'), [project.id], project.id);
    const reload = () => { questions.reload(); proposals.reload(); };
    const readOnly = project.status === 'archived';
    const groups = groupByEvent(proposals.data ?? []);
    const count = (questions.data?.length ?? 0) + (proposals.data?.length ?? 0);
    return (
        <Section title={<><Icon icon={Lightbulb} size={18}/>부서 판단과 제안</>} label="부서 판단과 제안" count={count}>
            <p className="muted small">부서가 받은 사건을 보고 제안한 업무와 판단에 필요한 질문입니다. 등록한 제안만 업무가 됩니다.</p>
            <ErrorBox error={questions.error || proposals.error}/>
            {!questions.data || !proposals.data ? <Loading rows={1}/> : count === 0 ? <p className="muted small">결정할 제안이나 질문이 없습니다.</p> : null}
            {(questions.data ?? []).map((j) => <QuestionCard key={j.id} j={j} onDone={reload}/>)}
            {groups.map((g) => <ProposalGroup key={`${g[0].projectId}:${g[0].eventSequence}`} proposals={g} onDone={reload} readOnly={readOnly}/>)}
        </Section>
    );
}

// pendingCount is how many decisions wait for the person, in one service
// or (no projectId) in all of them.
export function pendingCount(d: Decisions | undefined, projectId?: string): number {
    if (!d) return 0;
    const lists: { projectId: string }[][] = [d.questions, d.proposals, d.collabs, d.notices, d.aiDecisions, d.autoReviews];
    return lists.reduce((n, list) => n + list.filter((x) => !projectId || x.projectId === projectId).length, 0);
}

// InboxDecisions shows every service's department questions and
// proposals at the top of the inbox.
export function InboxDecisions({data, onDone}: { data: Decisions; onDone: () => void }) {
    if (data.questions.length + data.proposals.length === 0) return null;
    const groups = groupByEvent(data.proposals);
    return (
        <Section title={<><Icon icon={Lightbulb} size={18}/>부서의 제안과 질문</>} label="부서의 제안과 질문"
                 count={data.questions.length + data.proposals.length}>
            <div className="stack">
                {data.questions.map((j) => <QuestionCard key={j.id} j={j} onDone={onDone} showProject/>)}
                {groups.map((g) => <ProposalGroup key={`${g[0].projectId}:${g[0].eventSequence}`} proposals={g} onDone={onDone} showProject/>)}
            </div>
        </Section>
    );
}
