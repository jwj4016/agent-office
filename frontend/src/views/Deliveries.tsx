import {FileSearch, Inbox, Radio, RotateCcw, Send, SkipForward} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {Delivery, DeliveryFilter, DeliveryStatus, Department, Project} from '../types';
import {Button, ErrorBox, Icon, Loading, Section, StatusBadge, type Tone, shortTime, useAction, useToast} from '../ui';

export const deliveryTone: Record<DeliveryStatus, Tone> = {
    pending: 'info', processing: 'info', waiting_answer: 'warn', done: 'ok', skipped: '', held: 'warn', failed: 'bad',
};

export const eventLabel = (kind: string) => t.workEvent[kind] ?? kind;

export function DeliveryBadge({d}: { d: Delivery }) {
    return <StatusBadge tone={deliveryTone[d.status]}>{t.deliveryStatus[d.status]}</StatusBadge>;
}

// reason is why a delivery stopped or how it ended, whichever applies.
const reason = (d: Delivery) => d.error || d.holdReason || d.note;

// DeliveryInbox is every department's inbox in this service: the events
// each department received, what happened to them, and a way to ask a
// department to look for work directly.
export function DeliveryInbox({project, departments}: { project: Project; departments: Department[] }) {
    const [filter, setFilter] = useState<DeliveryFilter>({status: 'open'});
    const list = useLoad(() => api.deliveries(project.id, filter), [project.id, JSON.stringify(filter)], project.id);
    const action = useAction();
    const toast = useToast();
    const archived = project.status === 'archived';
    const name = (id: string) => departments.find((d) => d.id === id)?.name ?? '(알 수 없는 부서)';
    const act = (fn: () => Promise<void>, done: string) => action.run(async () => { await fn(); list.reload(); toast(done); });
    return (
        <Section title={<><Icon icon={Inbox} size={18}/>부서 전달함</>} label="부서 전달함" count={list.data?.length}>
            <p className="muted small">부서가 구독한 사건과 직접 받은 요청입니다. 판단을 맡은 부서는 대표 AI가 할 일이 있는지 살펴 업무를 제안하거나, 대응하지 않는 이유를 남깁니다.</p>
            <CheckRequest project={project} departments={departments} disabled={archived} onSent={list.reload}/>
            <DocumentReviewRequest project={project} departments={departments} disabled={archived} onSent={list.reload}/>
            <div className="toolbar" role="group" aria-label="전달함 조회 조건">
                <label className="row nowrap small">부서
                    <select value={filter.departmentId ?? ''} onChange={(e) => setFilter({...filter, departmentId: e.target.value})}>
                        <option value="">모든 부서</option>
                        {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select></label>
                <label className="row nowrap small">상태
                    <select value={filter.status ?? ''} onChange={(e) => setFilter({...filter, status: e.target.value})}>
                        <option value="open">처리할 것</option>
                        <option value="">전체</option>
                        {(Object.keys(t.deliveryStatus) as DeliveryStatus[]).map((s) => <option key={s} value={s}>{t.deliveryStatus[s]}</option>)}
                    </select></label>
            </div>
            <ErrorBox error={list.error || action.error}/>
            {!list.data ? <Loading/> : list.data.length === 0 ? (
                <p className="muted small">{filter.status === 'open' ? '처리할 전달이 없습니다.' : '전달된 사건이 없습니다.'}</p>
            ) : (
                <div className="table-wrap">
                    <table className="list">
                        <thead><tr><th>받은 때</th><th>부서</th><th>사건</th><th>상태</th><th/></tr></thead>
                        <tbody>
                        {list.data.map((d) => (
                            <tr key={d.id}>
                                <td className="small nowrap">{shortTime(d.eventAt)}</td>
                                <td className="small">{name(d.departmentId)}<div className="muted xsmall">{t.deliveryPurpose[d.purpose]}</div></td>
                                <td className="small">
                                    <strong>{eventLabel(d.eventKind)}</strong>{d.event.title ? ` · ${d.event.title}` : ''}
                                    {d.event.note ? <div className="muted xsmall pre">{d.event.note}</div> : null}
                                </td>
                                <td><DeliveryBadge d={d}/>{reason(d) ? <div className="muted xsmall">{reason(d)}</div> : null}</td>
                                <td className="actions">
                                    <span className="row nowrap" style={{justifyContent: 'flex-end'}}>
                                        {d.status === 'failed' || d.status === 'held' ? (
                                            <Button size="sm" icon={RotateCcw} disabled={archived} busy={action.busy}
                                                    aria-label={`${name(d.departmentId)} 전달 다시 시도`}
                                                    onClick={() => act(() => api.retryDelivery(project.id, d.id), '다시 처리하도록 넣었습니다')}>다시 시도</Button>
                                        ) : null}
                                        {d.status === 'pending' || d.status === 'failed' || d.status === 'held' ? (
                                            <Button size="sm" variant="ghost" icon={SkipForward} disabled={archived} busy={action.busy}
                                                    aria-label={`${name(d.departmentId)} 전달 건너뛰기`}
                                                    onClick={() => act(() => api.skipDelivery(project.id, d.id, ''), '건너뛰었습니다')}>건너뛰기</Button>
                                        ) : null}
                                    </span>
                                </td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                </div>
            )}
        </Section>
    );
}

// CheckRequest asks one department to look for work to do. It reaches
// the department whether or not it subscribes to anything.
function CheckRequest({project, departments, disabled, onSent}: {
    project: Project; departments: Department[]; disabled: boolean; onSent: () => void;
}) {
    const active = departments.filter((d) => d.status === 'active');
    const [dept, setDept] = useState('');
    const [note, setNote] = useState('');
    const action = useAction();
    const toast = useToast();
    const send = () => action.run(async () => {
        await api.requestDepartmentCheck(project.id, dept, note);
        toast(`${active.find((d) => d.id === dept)?.name ?? '부서'}에 점검을 요청했습니다`);
        setNote('');
        onSent();
    });
    return (
        <form className="row" aria-label="부서 점검 요청" onSubmit={(e) => { e.preventDefault(); if (dept) send(); }}>
            <Icon icon={Radio} size={16} className="muted"/>
            <select value={dept} disabled={disabled} aria-label="점검할 부서" onChange={(e) => setDept(e.target.value)}>
                <option value="">점검할 부서 선택</option>
                {active.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
            <input value={note} disabled={disabled} aria-label="요청 내용" placeholder="무엇을 살펴보면 좋을지 (선택)" style={{flex: 1, minWidth: 200}}
                   onChange={(e) => setNote(e.target.value)}/>
            <Button type="submit" size="sm" icon={Send} busy={action.busy} disabled={disabled || !dept}>점검 요청</Button>
            <ErrorBox error={action.error}/>
        </form>
    );
}

// DocumentReviewRequest asks one department to judge documents shared
// before it subscribed (a new department catching up, for example).
function DocumentReviewRequest({project, departments, disabled, onSent}: {
    project: Project; departments: Department[]; disabled: boolean; onSent: () => void;
}) {
    const active = departments.filter((d) => d.status === 'active');
    const [open, setOpen] = useState(false);
    const [dept, setDept] = useState('');
    const [picked, setPicked] = useState<string[]>([]);
    const docs = useLoad(() => open && dept ? api.documents(project.id, {visibleTo: dept}) : Promise.resolve([]), [project.id, open, dept]);
    const action = useAction();
    const toast = useToast();
    // The latest version of each document that has not been withdrawn.
    const latest = (docs.data ?? []).flatMap((d) => {
        const v = d.versions.filter((x) => !x.withdrawnAt).at(-1);
        return d.status === 'active' && v ? [{doc: d, v}] : [];
    });
    const send = () => action.run(async () => {
        const n = await api.requestDocumentReview(project.id, dept, picked, '기존 자료 검토');
        toast(`${active.find((d) => d.id === dept)?.name ?? '부서'}에 자료 ${n}건 검토를 요청했습니다`);
        setPicked([]);
        setOpen(false);
        onSent();
    });
    if (!open) {
        return <span><Button size="sm" variant="ghost" icon={FileSearch} disabled={disabled} onClick={() => setOpen(true)}>기존 자료 검토 요청</Button></span>;
    }
    return (
        <form className="card stack tight" aria-label="기존 자료 검토 요청" onSubmit={(e) => { e.preventDefault(); if (dept && picked.length) send(); }}>
            <select value={dept} aria-label="검토할 부서" onChange={(e) => { setDept(e.target.value); setPicked([]); }}>
                <option value="">검토할 부서 선택</option>
                {active.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
            {dept && docs.data && latest.length === 0 ? <span className="muted small">이 부서가 볼 수 있는 공유 자료가 없습니다.</span> : null}
            {latest.map(({doc, v}) => (
                <label key={v.id} className="check block">
                    <input type="checkbox" checked={picked.includes(v.id)}
                           onChange={(e) => setPicked(e.target.checked ? [...picked, v.id] : picked.filter((x) => x !== v.id))}/>
                    {doc.title} <span className="muted xsmall">{v.number}판{v.stage === 'final' ? ' · 확정' : ''}</span>
                </label>
            ))}
            <ErrorBox error={docs.error || action.error}/>
            <span className="row nowrap">
                <Button type="submit" size="sm" icon={Send} busy={action.busy} disabled={!dept || picked.length === 0}>검토 요청 ({picked.length})</Button>
                <Button size="sm" onClick={() => setOpen(false)}>닫기</Button>
            </span>
        </form>
    );
}

// WorkEvents lists one work item's events and which departments got them.
export function WorkEvents({project, workItemId, departments}: { project: Project; workItemId: string; departments: Department[] }) {
    const list = useLoad(() => api.workEvents(project.id, workItemId), [project.id, workItemId], project.id);
    const name = (id: string) => departments.find((d) => d.id === id)?.name ?? '(알 수 없는 부서)';
    if (list.error) return <ErrorBox error={list.error}/>;
    if (!list.data) return null;
    return (
        <Section title="사건" count={list.data.length}>
            {list.data.length === 0 ? <p className="muted small">아직 사건이 없습니다.</p> : (
                <ul className="stack tight" aria-label="이 업무의 사건" style={{listStyle: 'none', padding: 0, margin: 0}}>
                    {list.data.map((ev) => (
                        <li key={ev.sequence} className="stack tight">
                            <span className="row nowrap small"><strong>{eventLabel(ev.kind)}</strong>
                                <span className="muted xsmall">{shortTime(ev.createdAt)}</span></span>
                            <span className="row small">
                                {ev.deliveries.length === 0 ? <span className="muted xsmall">받은 부서 없음</span> : ev.deliveries.map((d) => (
                                    <span key={d.id} className="row nowrap" style={{gap: 4}}>{name(d.departmentId)}<DeliveryBadge d={d}/></span>
                                ))}
                            </span>
                        </li>
                    ))}
                </ul>
            )}
        </Section>
    );
}
