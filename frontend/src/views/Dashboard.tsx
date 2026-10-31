import {AlertTriangle, Archive, ArrowRight, CircleDot, FolderPlus, Hand, Handshake, Inbox as InboxIcon, Layers, PauseCircle, Plus, Sparkles} from 'lucide-react';
import {useState} from 'react';
import type {Route} from '../App';
import {serviceState} from '../App';
import {api} from '../api';
import {t} from '../i18n';
import type {InboxItem, ProjectSummary} from '../types';
import {
    Banner, Button, EmptyState, ErrorBox, Field, Icon, inboxKindIcon, Loading, PageHeader, relativeTime, RunBadge, Section, StatusBadge, useAction,
    useToast,
} from '../ui';

// Dashboard is the home screen: what needs a person first, then every
// service with its state.
// decisions: department questions, proposals, collaboration and
// automatic-start reviews waiting for the person.
export function Dashboard({summaries, inbox, decisions, loaded, onRoute, onCreated}: {
    summaries: ProjectSummary[]; inbox: InboxItem[]; decisions: number; loaded: boolean; onRoute: (r: Route) => void; onCreated: () => void;
}) {
    const [creating, setCreating] = useState(false);
    const [showArchived, setShowArchived] = useState(false);
    const [archived, setArchived] = useState<ProjectSummary[]>([]);
    const archiveAction = useAction();
    const open = (id: string) => onRoute({view: 'project', projectId: id, tab: 'status'});

    const toggleArchived = async () => {
        if (!showArchived) {
            const all = await archiveAction.run(() => api.dashboard(true));
            setArchived((all ?? []).filter((s) => s.project.status === 'archived'));
        }
        setShowArchived(!showArchived);
    };

    const blocked = summaries.filter((s) => s.failedRuns > 0);
    const paused = summaries.filter((s) => s.pausedRuns > 0 && s.failedRuns === 0);
    const active = summaries.reduce((n, s) => n + s.activeRuns, 0);
    const blockedCount = summaries.reduce((n, s) => n + s.failedRuns, 0);
    const newForm = creating ? <NewProject onDone={(id) => { setCreating(false); onCreated(); if (id) open(id); }}/> : null;

    if (!loaded) return <div className="page"><Loading/></div>;

    if (summaries.length === 0) {
        return (
            <div className="page narrow">
                <PageHeader title={t.nav.dashboard}/>
                {newForm ?? (
                    <EmptyState icon={Sparkles} title="첫 서비스를 시작하세요"
                                actions={<>
                                    <Button variant="primary" icon={Sparkles} onClick={() => onRoute({view: 'design'})}>새 목표로 시작</Button>
                                    <Button icon={FolderPlus} onClick={() => setCreating(true)}>직접 만들기</Button>
                                </>}>
                        목표를 적으면 설계 AI가 필요한 역할·담당자·업무 흐름을 제안합니다. 직접 하나씩 만들어도 됩니다.
                    </EmptyState>
                )}
                <ArchivedList show={showArchived} toggle={toggleArchived} items={archived} error={archiveAction.error}
                              onOpen={open} onRestore={async (id) => {
                                  await archiveAction.run(() => api.setArchived(id, false));
                                  setArchived((a) => a.filter((x) => x.project.id !== id));
                                  onCreated();
                              }}/>
            </div>
        );
    }

    const lastRunRoute = (s: ProjectSummary): Route =>
        s.lastRun ? {view: 'project', projectId: s.project.id, tab: 'runs', runId: s.lastRun.id} : {view: 'project', projectId: s.project.id, tab: 'runs'};
    const todo = inbox.length + decisions;
    const attention = blocked.length + paused.length + todo;

    return (
        <div className="page">
            <PageHeader title={t.nav.dashboard}
                        desc={attention ? `확인이 필요한 일이 ${attention}건 있습니다.` : '지금 확인이 필요한 일은 없습니다.'}
                        actions={<Button icon={Plus} onClick={() => setCreating(true)}>{t.nav.newService}</Button>}/>
            {newForm}

            <div className="stats">
                <button className={`stat ${todo ? 'warn' : ''}`} onClick={() => onRoute({view: 'inbox'})}>
                    <span className="stat-label"><Icon icon={InboxIcon}/>내 할 일</span>
                    <span className="stat-value">{todo}</span>
                </button>
                <div className={`stat ${blockedCount ? 'bad' : ''}`}>
                    <span className="stat-label"><Icon icon={AlertTriangle}/>막힌 실행</span>
                    <span className="stat-value">{blockedCount}</span>
                </div>
                <div className={`stat ${active ? 'info' : ''}`}>
                    <span className="stat-label"><Icon icon={CircleDot}/>진행 중 실행</span>
                    <span className="stat-value">{active}</span>
                </div>
                <div className="stat">
                    <span className="stat-label"><Icon icon={Layers}/>서비스</span>
                    <span className="stat-value">{summaries.length}</span>
                </div>
            </div>

            <Section title="확인이 필요한 일" count={attention}>
                {attention === 0 ? <Banner tone="ok">막힌 실행도, 기다리는 일도 없습니다.</Banner> : (
                    <ul className="attention" style={{listStyle: 'none', padding: 0, margin: 0}}>
                        {blocked.map((s) => (
                            <li key={`b-${s.project.id}`} className="attention-item">
                                <span className="icon-wrap bad"><Icon icon={AlertTriangle} size={18}/></span>
                                <div className="body">
                                    <span className="title">{s.project.name} · 실행이 막혔습니다</span>
                                    <span className="meta">{s.lastRun ? `${s.lastRun.title} · ${relativeTime(s.lastRun.startedAt)} 시작` : `막힌 실행 ${s.failedRuns}건`}</span>
                                </div>
                                <Button size="sm" variant="primary" onClick={() => onRoute(lastRunRoute(s))}>원인 보기<Icon icon={ArrowRight} size={14}/></Button>
                            </li>
                        ))}
                        {inbox.slice(0, 5).map((it) => (
                            <li key={`i-${it.kind}-${it.attemptId}-${it.approvalId ?? ''}-${it.messageId ?? ''}`} className="attention-item">
                                <span className="icon-wrap warn"><Icon icon={inboxKindIcon[it.kind]} size={18}/></span>
                                <div className="body">
                                    <span className="title">{it.stepTitle}</span>
                                    <span className="meta">{t.inboxKind[it.kind]} · {it.projectName} · {relativeTime(it.since)}</span>
                                </div>
                                <Button size="sm" onClick={() => onRoute({view: 'inbox'})}>처리하기</Button>
                            </li>
                        ))}
                        {decisions ? (
                            <li className="attention-item">
                                <span className="icon-wrap warn"><Icon icon={Handshake} size={18}/></span>
                                <div className="body">
                                    <span className="title">부서 간 협업과 결정 {decisions}건</span>
                                    <span className="meta">협업 요청·부서 제안과 질문·자동 처리 검토</span>
                                </div>
                                <Button size="sm" onClick={() => onRoute({view: 'inbox'})}>처리하기</Button>
                            </li>
                        ) : null}
                        {inbox.length > 5 ? (
                            <li className="attention-item">
                                <span className="icon-wrap"><Icon icon={Hand} size={18}/></span>
                                <div className="body"><span className="title">내 할 일 {inbox.length - 5}건 더</span></div>
                                <Button size="sm" variant="ghost" onClick={() => onRoute({view: 'inbox'})}>모두 보기</Button>
                            </li>
                        ) : null}
                        {paused.map((s) => (
                            <li key={`p-${s.project.id}`} className="attention-item">
                                <span className="icon-wrap warn"><Icon icon={PauseCircle} size={18}/></span>
                                <div className="body">
                                    <span className="title">{s.project.name} · 일시 정지된 실행 {s.pausedRuns}건</span>
                                    <span className="meta">새 업무가 시작되지 않습니다. 실행 화면에서 재개할 수 있습니다.</span>
                                </div>
                                <Button size="sm" onClick={() => onRoute(lastRunRoute(s))}>실행 열기</Button>
                            </li>
                        ))}
                    </ul>
                )}
            </Section>

            <Section title="서비스" count={summaries.length}>
                <div className="grid-cards">
                    {summaries.map((s) => <ServiceCard key={s.project.id} s={s} onOpen={() => open(s.project.id)}/>)}
                </div>
            </Section>

            <ArchivedList show={showArchived} toggle={toggleArchived} items={archived} error={archiveAction.error}
                          onOpen={open} onRestore={async (id) => {
                              await archiveAction.run(() => api.setArchived(id, false));
                              setArchived((a) => a.filter((x) => x.project.id !== id));
                              onCreated();
                          }}/>
        </div>
    );
}

function ServiceCard({s, onOpen}: { s: ProjectSummary; onOpen: () => void }) {
    const st = serviceState(s);
    return (
        <button className="card clickable stack" onClick={onOpen} aria-label={`${s.project.name}${st.label ? `, ${st.label}` : ''}`}>
            <div className="row between nowrap">
                <h3 className="truncate" style={{fontSize: 'var(--fs-lg)'}}>{s.project.name}</h3>
                {s.lastRun ? <RunBadge status={s.lastRun.status}/> : <StatusBadge>실행 전</StatusBadge>}
            </div>
            <p className="muted small" style={{display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden'}}>
                {s.project.goal || '목표가 아직 없습니다'}</p>
            <div className="row">
                {s.failedRuns > 0 ? <StatusBadge tone="bad" icon={AlertTriangle}>막힘 {s.failedRuns}</StatusBadge> : null}
                {s.inbox > 0 ? <StatusBadge tone="warn" icon={InboxIcon}>내 할 일 {s.inbox}</StatusBadge> : null}
                {s.waitingRuns > 0 ? <StatusBadge tone="warn" icon={Hand}>사람 대기 {s.waitingRuns}</StatusBadge> : null}
                {s.pausedRuns > 0 ? <StatusBadge tone="warn" icon={PauseCircle}>정지 {s.pausedRuns}</StatusBadge> : null}
                {s.activeRuns > 0 ? <StatusBadge tone="info" icon={CircleDot}>진행 {s.activeRuns}</StatusBadge> : null}
            </div>
            {s.lastRun ? <p className="muted xsmall truncate">최근 실행: {s.lastRun.title} · {relativeTime(s.lastRun.startedAt)}</p> : null}
        </button>
    );
}

function ArchivedList({show, toggle, items, error, onOpen, onRestore}: {
    show: boolean; toggle: () => void; items: ProjectSummary[]; error: string | null;
    onOpen: (id: string) => void; onRestore: (id: string) => void;
}) {
    return (
        <div className="stack">
            <div><Button variant="ghost" size="sm" icon={Archive} onClick={toggle} aria-expanded={show}>
                {show ? '보관된 서비스 숨기기' : t.nav.archived}</Button></div>
            <ErrorBox error={error}/>
            {show ? (
                items.length === 0 ? <p className="muted small">보관된 서비스가 없습니다.</p> : (
                    <div className="table-wrap">
                        <table className="list" aria-label="보관된 서비스">
                            <tbody>
                            {items.map((s) => (
                                <tr key={s.project.id}>
                                    <td>{s.project.name} <StatusBadge icon={Archive}>보관됨</StatusBadge></td>
                                    <td className="actions">
                                        <span className="row nowrap" style={{justifyContent: 'flex-end'}}>
                                            <Button size="sm" variant="ghost" onClick={() => onOpen(s.project.id)}>기록 보기</Button>
                                            <Button size="sm" onClick={() => onRestore(s.project.id)}>{t.project.restore}</Button>
                                        </span>
                                    </td>
                                </tr>
                            ))}
                            </tbody>
                        </table>
                    </div>
                )
            ) : null}
        </div>
    );
}

function NewProject({onDone}: { onDone: (id?: string) => void }) {
    const [name, setName] = useState('');
    const [goal, setGoal] = useState('');
    const action = useAction();
    const toast = useToast();
    const submit = async () => {
        const p = await action.run(() => api.createProject({name, goal, instructions: '', mode: 'review'}));
        if (p) {
            toast(`‘${p.name}’ 서비스를 만들었습니다`);
            onDone(p.id);
        }
    };
    return (
        <form className="card stack" onSubmit={(e) => { e.preventDefault(); submit(); }} aria-label="새 서비스">
            <h2>{t.nav.newService}</h2>
            <p className="muted small">이름만 정하면 바로 만들 수 있습니다. 역할과 업무 흐름은 나중에 추가합니다.</p>
            <Field label={t.project.name}>
                <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="예: 게임 서비스"/>
            </Field>
            <Field label={t.project.goal} optional hint="이 서비스로 이루려는 것. AI 담당자가 모두 참고합니다.">
                <textarea value={goal} onChange={(e) => setGoal(e.target.value)}/>
            </Field>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button variant="primary" type="submit" busy={action.busy} disabled={!name.trim()}>{t.project.create}</Button>
                <Button onClick={() => onDone()}>{t.common.cancel}</Button>
            </div>
        </form>
    );
}
