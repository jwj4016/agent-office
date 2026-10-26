import {ArrowRightLeft, Check, CircleHelp, ExternalLink, Handshake, Send, ShieldCheck, Undo2, X} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {
    CollabAcceptance, CollabRequest, CollabRequirement, Department, NewCollabRequest, Project, RequiredNotice, WorkItem,
} from '../types';
import {Banner, Button, ErrorBox, Field, Icon, Loading, Section, StatusBadge, type Tone, relativeTime, shortTime, useAction, useToast} from '../ui';

const statusTone: Record<CollabRequest['status'], Tone> = {requested: 'info', question: 'warn', accepted: 'ok', declined: 'bad', cancelled: ''};

const undecided = (r: CollabRequest) => r.status === 'requested' || r.status === 'question';
const closedWork = (s: string) => s === 'done' || s === 'cancelled';

// blockReason says why a required request keeps work from completing; the
// same wording the app uses when it refuses a completion.
export function blockReason(r: CollabRequest) {
    let s: string;
    switch (r.status) {
    case 'requested': s = '수락 대기'; break;
    case 'question': s = '받는 부서의 질문에 답해야 함'; break;
    case 'declined': s = '거절됨, 필수 해제나 대체가 필요'; break;
    case 'cancelled': s = '취소됨, 필수 해제나 대체가 필요'; break;
    default: s = r.resultWorkStatus === 'cancelled' ? `수행 업무 ‘${r.resultWorkTitle}’ 취소됨` : `수행 업무 ‘${r.resultWorkTitle}’ 미완료`;
    }
    return `${r.toDepartmentName}: ${s}${r.overdue ? ', 기한 지남' : ''}`;
}

// acting reads "개발팀으로서": the department a person decides for, with
// the particle its last syllable takes.
export function acting(name: string) {
    const c = name.charCodeAt(name.length - 1) - 0xac00;
    if (c < 0 || c > 11171) return `${name}(으)로서`;
    const final = c % 28;
    return name + (final === 0 || final === 8 ? '로서' : '으로서');
}

// actorName is who acted in a history entry: a department, or the person
// in charge of the asking work.
const actorName = (r: CollabRequest, id: string) =>
    id === r.fromDepartmentId ? acting(r.fromDepartmentName) : id === r.toDepartmentId ? acting(r.toDepartmentName) : '책임자';

// WorkCollaboration is a work item's (or goal's) collaboration: the request
// it came from, what it asked other departments, what keeps it from being
// completed, and a way to ask for more help.
export function WorkCollaboration({project, item, departments, onOpenWork, onChanged}: {
    project: Project; item: WorkItem; departments: Department[]; onOpenWork: (id: string) => void; onChanged: () => void;
}) {
    const c = useLoad(() => api.workCollab(project.id, item.id), [project.id, item.id, item.status, item.updatedAt], project.id);
    const action = useAction();
    const [asking, setAsking] = useState(false);
    const readOnly = project.status === 'archived';
    if (c.error) return <ErrorBox error={c.error}/>;
    if (!c.data) return <Loading rows={1}/>;
    const {requests, incoming, blockers, checks} = c.data;
    const done = () => { c.reload(); onChanged(); };
    const closeCheck = (id: string, status: 'resolved' | 'dismissed') =>
        action.run(async () => { await api.closeImpactCheck(project.id, id, status, ''); c.reload(); });
    const own = departments.find((d) => d.id === item.departmentId);
    const canAsk = !readOnly && !closedWork(item.status) && !!own;
    return (
        <Section title={<><Icon icon={Handshake} size={18}/>협업</>} label="협업" count={requests.length}
                 actions={canAsk && !asking ? <Button size="sm" icon={Handshake} onClick={() => setAsking(true)}>다른 부서에 요청</Button> : null}>
            {checks.map((ch) => (
                <Banner key={ch.id} tone="warn" title="필수 협업이 바뀌었습니다"
                        actions={<span className="row nowrap">
                            <Button size="sm" busy={action.busy} disabled={readOnly} onClick={() => closeCheck(ch.id, 'resolved')}>확인함</Button>
                            <Button size="sm" variant="ghost" busy={action.busy} disabled={readOnly} onClick={() => closeCheck(ch.id, 'dismissed')}>무시</Button>
                        </span>}>{ch.summary}</Banner>
            ))}
            <ErrorBox error={action.error}/>
            {incoming.map((r) => (
                <Banner key={r.id} tone="info" title={`${r.fromDepartmentName}의 협업 요청으로 맡은 업무 · ${t.collabRequirement[r.requirement]}`}
                        actions={<Button size="sm" icon={ExternalLink} onClick={() => onOpenWork(r.fromWorkItemId)}>요청한 업무 열기</Button>}>
                    <span className="pre">{r.question}</span>
                    <span className="muted xsmall"> · 요청한 업무: {r.fromWorkTitle}</span>
                </Banner>
            ))}
            {blockers.length > 0 && !closedWork(item.status) ? (
                <Banner tone="warn" title={`완료하려면 필수 협업 ${blockers.length}건이 끝나야 합니다`}>
                    <ul style={{margin: 0, paddingLeft: 18}}>{blockers.map((b) => <li key={b.id}>{blockReason(b)}</li>)}</ul>
                </Banner>
            ) : null}
            {requests.length === 0 && incoming.length === 0 ? (
                <p className="muted small">
                    {own ? '다른 부서에 도움이 필요하면 요청하세요. 받는 부서가 수락하면 그 부서의 업무가 생깁니다. ‘완료에 필요’로 보낸 요청은 결과가 끝나야 이 업무와 목표를 완료할 수 있습니다.'
                        : '담당 부서를 정하면 다른 부서에 협업을 요청할 수 있습니다.'}
                </p>
            ) : null}
            {requests.map((r) => (
                <CollabCard key={r.id} r={r} onDone={done} onOpenWork={onOpenWork} showFrom={r.fromWorkItemId !== item.id} readOnly={readOnly}/>
            ))}
            {asking && own ? (
                <CollabRequestForm project={project} item={item} from={own} departments={departments}
                                   onDone={() => setAsking(false)} onSent={() => { setAsking(false); done(); }}/>
            ) : null}
        </Section>
    );
}

// CollabRequestForm sends a request from the work's department.
function CollabRequestForm({project, item, from, departments, onSent, onDone}: {
    project: Project; item: WorkItem; from: Department; departments: Department[]; onSent: () => void; onDone: () => void;
}) {
    const links = useLoad(() => (item.kind === 'work' ? api.workDocuments(project.id, item.id) : Promise.resolve([])), [project.id, item.id]);
    const targets = departments.filter((d) => d.status === 'active' && d.id !== from.id);
    const [form, setForm] = useState<NewCollabRequest>({toDepartmentId: targets[0]?.id ?? '', question: '', inputVersionIds: [], requirement: 'reference',
        requirementReason: '', dueAt: ''});
    const action = useAction();
    const toast = useToast();
    const set = (patch: Partial<NewCollabRequest>) => setForm({...form, ...patch});
    const send = () => action.run(async () => {
        const r = await api.requestCollab(project.id, {...form, fromWorkItemId: item.id});
        toast(`${r.toDepartmentName}에 협업을 요청했습니다`);
        onSent();
    });
    if (targets.length === 0) return <p className="muted small">요청할 수 있는 다른 부서가 없습니다.</p>;
    const versions = form.inputVersionIds ?? [];
    return (
        <form className="card stack" aria-label="협업 요청" onSubmit={(e) => { e.preventDefault(); send(); }}>
            <div className="grid-2">
                <Field label="요청하는 부서"><input value={from.name} readOnly/></Field>
                <Field label="받는 부서">
                    <select value={form.toDepartmentId} onChange={(e) => set({toDepartmentId: e.target.value})}>
                        {targets.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select></Field>
            </div>
            <Field label="요청 내용" hint="받는 부서가 무엇을 해 주면 되는지 적으세요.">
                <textarea rows={3} value={form.question} autoFocus onChange={(e) => set({question: e.target.value})}/></Field>
            {(links.data ?? []).length > 0 ? (
                <fieldset className="stack tight">
                    <legend className="field-label">함께 보낼 자료 <span className="optional">(선택)</span></legend>
                    {(links.data ?? []).map((l) => (
                        <label key={l.version.id} className="check block">
                            <input type="checkbox" checked={versions.includes(l.version.id)}
                                   onChange={(e) => set({inputVersionIds: e.target.checked ? [...versions, l.version.id] : versions.filter((v) => v !== l.version.id)})}/>
                            {l.document.title} {l.version.number}판
                        </label>
                    ))}
                </fieldset>
            ) : null}
            <div className="grid-2">
                <Field label="협업 종류" hint="‘완료에 필요’는 결과가 끝나야 이 업무와 목표를 완료할 수 있습니다.">
                    <select value={form.requirement} onChange={(e) => set({requirement: e.target.value as CollabRequirement})}>
                        <option value="reference">{t.collabRequirement.reference}</option>
                        <option value="required">{t.collabRequirement.required}</option>
                    </select></Field>
                <Field label="기한" optional><input type="date" value={form.dueAt} onChange={(e) => set({dueAt: e.target.value})}/></Field>
            </div>
            {form.requirement === 'required' ? (
                <Field label="필수로 정한 이유" optional>
                    <input value={form.requirementReason} onChange={(e) => set({requirementReason: e.target.value})}/></Field>
            ) : null}
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" variant="primary" icon={Send} busy={action.busy} disabled={!form.question.trim() || !form.toDepartmentId}>
                    {acting(from.name)} 요청 보내기</Button>
                <Button onClick={onDone}>{t.common.cancel}</Button>
            </div>
        </form>
    );
}

type Mode = null | 'accept' | 'ask' | 'answer' | 'decline' | 'cancel' | 'require' | 'release' | 'replace';

// CollabCard is one request with what can be done to it now. Every button
// says which department (or the person in charge) decides.
export function CollabCard({r, onDone, onOpenWork, showFrom, showProject, readOnly}: {
    r: CollabRequest; onDone: () => void; onOpenWork?: (id: string) => void; showFrom?: boolean; showProject?: boolean; readOnly?: boolean;
}) {
    const [mode, setMode] = useState<Mode>(null);
    const [text, setText] = useState('');
    const action = useAction();
    const toast = useToast();
    const act = (fn: () => Promise<unknown>, done: string) => action.run(async () => {
        await fn();
        toast(done);
        setMode(null);
        setText('');
        onDone();
    });
    const open = (m: Mode) => { setMode(m); setText(''); };
    const from = r.fromDepartmentName, to = r.toDepartmentName;
    const required = r.requirement === 'required';
    const lastAnswer = [...r.history].reverse().find((h) => h.action === 'answered');
    return (
        <article className="card stack tight" aria-label={`${from} → ${to} 협업 요청`}>
            <span className="row small">
                <Icon icon={Handshake} size={16}/>
                <strong>{showProject ? `${r.projectName} · ` : ''}{from} → {to}</strong>
                <StatusBadge tone={statusTone[r.status]}>{t.collabStatus[r.status]}</StatusBadge>
                <StatusBadge tone={required ? 'warn' : ''} icon={required ? ShieldCheck : undefined}>{t.collabRequirement[r.requirement]}</StatusBadge>
                {r.satisfied ? <StatusBadge tone="ok">충족</StatusBadge> : null}
                {r.dueAt ? <StatusBadge tone={r.overdue ? 'bad' : ''}>{r.overdue ? `${r.dueAt} 지남` : `기한 ${r.dueAt}`}</StatusBadge> : null}
                <span className="spacer"/>
                <span className="muted xsmall">{relativeTime(r.createdAt)}</span>
            </span>
            {showFrom || showProject ? (
                <span className="muted xsmall">요청한 업무: {onOpenWork
                    ? <button className="btn ghost small" onClick={() => onOpenWork(r.fromWorkItemId)}>{r.fromWorkTitle}</button>
                    : r.fromWorkTitle}</span>
            ) : null}
            <p className="pre small" style={{margin: 0}}>{r.question}</p>
            {r.requirementReason ? <span className="muted xsmall">{required ? '필수 이유' : '필수 해제 이유'}: {r.requirementReason}</span> : null}
            {r.status === 'question' ? <Banner tone="warn" icon={CircleHelp} title={`${to}의 질문`}>{r.followUp}</Banner> : null}
            {r.status === 'requested' && lastAnswer ? <span className="muted xsmall">{from}의 답변: {lastAnswer.body}</span> : null}
            {r.status === 'declined' ? <span className="small">거절 이유: {r.decisionReason}</span> : null}
            {r.resultWorkItemId ? (
                <span className="row nowrap small">수행 업무:
                    {onOpenWork ? <button className="btn ghost small" onClick={() => onOpenWork(r.resultWorkItemId)}>{r.resultWorkTitle}</button>
                        : <strong>{r.resultWorkTitle}</strong>}
                    {r.resultWorkStatus ? <StatusBadge tone={r.resultWorkStatus === 'done' ? 'ok' : r.resultWorkStatus === 'cancelled' ? '' : 'info'}>{t.workStatus[r.resultWorkStatus]}</StatusBadge> : null}
                </span>
            ) : null}
            {r.replacedById ? <span className="muted xsmall">다른 요청으로 대체했습니다.</span> : null}
            {r.replacesId ? <span className="muted xsmall">거절·취소된 필수 요청을 대신합니다.</span> : null}
            <details>
                <summary className="xsmall muted">기록 {r.history.length}건</summary>
                <ol className="stack tight small" style={{margin: '4px 0 0', paddingLeft: 18}}>
                    {r.history.map((h, i) => (
                        <li key={i}><strong>{t.collabAction[h.action]}</strong> · {actorName(r, h.asDepartmentId)}
                            {h.body ? <> · <span className="pre">{h.body}</span></> : null} <span className="muted xsmall">{shortTime(h.createdAt)}</span></li>
                    ))}
                </ol>
            </details>
            {readOnly ? null : mode === null ? (
                <span className="row">
                    {undecided(r) ? <>
                        <Button size="sm" variant="primary" icon={Check} onClick={() => open('accept')}>{acting(to)} 수락</Button>
                        <Button size="sm" icon={CircleHelp} onClick={() => open('ask')}>{acting(to)} 질문</Button>
                        <Button size="sm" variant="ghost" icon={X} onClick={() => open('decline')}>{acting(to)} 거절</Button>
                    </> : null}
                    {r.status === 'question' ? <Button size="sm" variant="primary" icon={Send} onClick={() => open('answer')}>{acting(from)} 답하기</Button> : null}
                    {undecided(r) || r.status === 'declined' ? <Button size="sm" variant="ghost" icon={Undo2} onClick={() => open('cancel')}>{acting(from)} 요청 취소</Button> : null}
                    {!required && !closedWork(r.fromWorkStatus) && r.decidedBy !== 'ai' ? (
                        <Button size="sm" icon={ShieldCheck} onClick={() => open('require')}>책임자로서 필수 지정</Button>
                    ) : null}
                    {required ? <Button size="sm" variant="ghost" onClick={() => open('release')}>책임자로서 필수 해제</Button> : null}
                    {required && (r.status === 'declined' || r.status === 'cancelled') && !r.replacedById ? (
                        <Button size="sm" icon={ArrowRightLeft} onClick={() => open('replace')}>다른 요청으로 대체</Button>
                    ) : null}
                </span>
            ) : mode === 'accept' ? (
                <AcceptForm r={r} busy={action.busy} onCancel={() => setMode(null)}
                            onAccept={(a) => act(() => api.acceptCollab(r.projectId, r.id, a), `${acting(to)} 수락했습니다`)}/>
            ) : mode === 'replace' ? (
                <ReplaceForm r={r} busy={action.busy} onCancel={() => setMode(null)}
                             onReplace={(n, reason) => act(() => api.replaceCollab(r.projectId, r.id, n, reason), '새 필수 요청을 보냈습니다')}/>
            ) : (
                <TextStep mode={mode} r={r} text={text} setText={setText} busy={action.busy} onCancel={() => setMode(null)} onSubmit={() => {
                    switch (mode) {
                    case 'ask': return act(() => api.askCollab(r.projectId, r.id, text), `${acting(to)} 질문했습니다`);
                    case 'answer': return act(() => api.answerCollab(r.projectId, r.id, text), `${acting(from)} 답했습니다`);
                    case 'decline': return act(() => api.declineCollab(r.projectId, r.id, text), `${acting(to)} 거절했습니다`);
                    case 'cancel': return act(() => api.cancelCollab(r.projectId, r.id, text), '요청을 취소했습니다');
                    case 'require': return act(() => api.setCollabRequirement(r.projectId, r.id, 'required', text), '완료에 필요한 협업으로 지정했습니다');
                    case 'release': return act(() => api.setCollabRequirement(r.projectId, r.id, 'reference', text), '필수 조건을 해제했습니다');
                    }
                }}/>
            )}
            <ErrorBox error={action.error}/>
        </article>
    );
}

const steps: Record<'ask' | 'answer' | 'decline' | 'cancel' | 'require' | 'release', {label: string; who: 'to' | 'from' | ''; button: string; needed: boolean}> = {
    ask: {label: '질문', who: 'to', button: '질문 보내기', needed: true},
    answer: {label: '답변', who: 'from', button: '답하기', needed: true},
    decline: {label: '거절 이유', who: 'to', button: '거절', needed: true},
    cancel: {label: '취소 이유 (선택)', who: 'from', button: '요청 취소', needed: false},
    require: {label: '필수로 정하는 이유 (선택)', who: '', button: '필수로 지정', needed: false},
    release: {label: '필수 조건을 해제하는 이유', who: '', button: '필수 해제', needed: true},
};

function TextStep({mode, r, text, setText, busy, onSubmit, onCancel}: {
    mode: keyof typeof steps; r: CollabRequest; text: string; setText: (s: string) => void; busy: boolean; onSubmit: () => void; onCancel: () => void;
}) {
    const s = steps[mode];
    const who = s.who === 'to' ? `${acting(r.toDepartmentName)} ` : s.who === 'from' ? `${acting(r.fromDepartmentName)} ` : '책임자로서 ';
    return (
        <form className="row" aria-label={`${who}${s.button}`} onSubmit={(e) => { e.preventDefault(); if (!s.needed || text.trim()) onSubmit(); }}>
            <input value={text} autoFocus style={{flex: 1, minWidth: 200}} aria-label={s.label} placeholder={s.label} onChange={(e) => setText(e.target.value)}/>
            <Button type="submit" size="sm" variant={mode === 'decline' ? 'danger' : 'primary'} busy={busy} disabled={s.needed && !text.trim()}>{who}{s.button}</Button>
            <Button size="sm" onClick={onCancel}>{t.common.cancel}</Button>
        </form>
    );
}

// AcceptForm takes a request on as new work of the receiving department,
// or as one of its open work items.
function AcceptForm({r, busy, onAccept, onCancel}: {
    r: CollabRequest; busy: boolean; onAccept: (a: CollabAcceptance) => void; onCancel: () => void;
}) {
    const [existing, setExisting] = useState(false);
    const [form, setForm] = useState<CollabAcceptance>({title: `${r.fromDepartmentName} 협업: ${r.fromWorkTitle}`, doneCriteria: '', note: '', existingWorkItemId: ''});
    const open = useLoad(() => (existing ? api.workItems(r.projectId, {departmentId: r.toDepartmentId, status: 'open', kind: 'work'}) : Promise.resolve([])),
        [r.projectId, r.toDepartmentId, existing]);
    const set = (patch: Partial<CollabAcceptance>) => setForm({...form, ...patch});
    const ready = existing ? !!form.existingWorkItemId : !!form.title?.trim();
    return (
        <form className="stack tight" aria-label={`${acting(r.toDepartmentName)} 수락`}
              onSubmit={(e) => { e.preventDefault(); if (ready) onAccept(existing ? {existingWorkItemId: form.existingWorkItemId, note: form.note} : {...form, existingWorkItemId: ''}); }}>
            <span className="row">
                <label className="check"><input type="radio" checked={!existing} onChange={() => setExisting(false)}/>새 업무 등록</label>
                <label className="check"><input type="radio" checked={existing} onChange={() => setExisting(true)}/>{r.toDepartmentName}의 기존 업무로</label>
            </span>
            {existing ? (
                <Field label="기존 업무">
                    <select value={form.existingWorkItemId} onChange={(e) => set({existingWorkItemId: e.target.value})}>
                        <option value="">업무 선택</option>
                        {(open.data ?? []).map((w) => <option key={w.id} value={w.id}>{w.title}</option>)}
                    </select></Field>
            ) : <>
                <Field label="업무 제목"><input value={form.title} onChange={(e) => set({title: e.target.value})}/></Field>
                <Field label="완료 조건" optional><input value={form.doneCriteria} onChange={(e) => set({doneCriteria: e.target.value})}/></Field>
            </>}
            <Field label="메모" optional><input value={form.note} onChange={(e) => set({note: e.target.value})}/></Field>
            <span className="row nowrap">
                <Button type="submit" size="sm" variant="primary" icon={Check} busy={busy} disabled={!ready}>{acting(r.toDepartmentName)} 수락</Button>
                <Button size="sm" onClick={onCancel}>{t.common.cancel}</Button>
            </span>
        </form>
    );
}

// ReplaceForm sends a new required request in place of a declined or
// cancelled one; the old one becomes reference with the reason.
function ReplaceForm({r, busy, onReplace, onCancel}: {
    r: CollabRequest; busy: boolean; onReplace: (n: NewCollabRequest, reason: string) => void; onCancel: () => void;
}) {
    const depts = useLoad(() => api.departments(r.projectId), [r.projectId]);
    const targets = (depts.data ?? []).filter((d) => d.status === 'active' && d.id !== r.fromDepartmentId);
    const [to, setTo] = useState('');
    const [question, setQuestion] = useState(r.question);
    const [reason, setReason] = useState('');
    const target = to || targets.find((d) => d.id !== r.toDepartmentId)?.id || targets[0]?.id || '';
    return (
        <form className="stack tight" aria-label="다른 요청으로 대체"
              onSubmit={(e) => { e.preventDefault(); if (target && question.trim() && reason.trim()) onReplace({toDepartmentId: target, question}, reason); }}>
            <Field label="받는 부서">
                <select value={target} onChange={(e) => setTo(e.target.value)}>
                    {targets.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                </select></Field>
            <Field label="요청 내용"><textarea rows={2} value={question} onChange={(e) => setQuestion(e.target.value)}/></Field>
            <Field label="대체하는 이유"><input value={reason} onChange={(e) => setReason(e.target.value)}/></Field>
            <span className="row nowrap">
                <Button type="submit" size="sm" variant="primary" icon={ArrowRightLeft} busy={busy} disabled={!target || !question.trim() || !reason.trim()}>
                    책임자로서 대체</Button>
                <Button size="sm" onClick={onCancel}>{t.common.cancel}</Button>
            </span>
        </form>
    );
}

// InboxCollaboration is every service's collaboration waiting on the
// person, and required work that changed after something was finished.
export function InboxCollaboration({collabs, notices, onDone}: { collabs: CollabRequest[]; notices: RequiredNotice[]; onDone: () => void }) {
    const action = useAction();
    if (collabs.length + notices.length === 0) return null;
    const close = (n: RequiredNotice, status: 'resolved' | 'dismissed') =>
        action.run(async () => { await api.closeImpactCheck(n.projectId, n.id, status, ''); onDone(); });
    return (
        <Section title={<><Icon icon={Handshake} size={18}/>부서 간 협업</>} label="부서 간 협업" count={collabs.length + notices.length}>
            <div className="stack">
                {notices.map((n) => (
                    <Banner key={n.id} tone="warn" title={`${n.projectName} · ${n.workKind === 'goal' ? '공동 목표' : '업무'} ‘${n.workTitle}’`}
                            actions={<span className="row nowrap">
                                <Button size="sm" busy={action.busy} onClick={() => close(n, 'resolved')}>확인함</Button>
                                <Button size="sm" variant="ghost" busy={action.busy} onClick={() => close(n, 'dismissed')}>무시</Button>
                            </span>}>{n.summary}</Banner>
                ))}
                <ErrorBox error={action.error}/>
                {collabs.map((r) => <CollabCard key={r.id} r={r} onDone={onDone} showProject/>)}
            </div>
        </Section>
    );
}
