import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import type {ProjectSummary} from '../types';
import {ErrorBox, RunBadge, shortTime, useAction} from '../ui';

export function Dashboard({summaries, onOpen, onCreated}: {
    summaries: ProjectSummary[]; onOpen: (id: string) => void; onCreated: () => void;
}) {
    const [creating, setCreating] = useState(summaries.length === 0);
    const [showArchived, setShowArchived] = useState(false);
    const [archived, setArchived] = useState<ProjectSummary[]>([]);
    const archiveAction = useAction();

    const toggleArchived = async () => {
        if (!showArchived) {
            const all = await archiveAction.run(() => api.dashboard(true));
            setArchived((all ?? []).filter((s) => s.project.status === 'archived'));
        }
        setShowArchived(!showArchived);
    };

    return (
        <div className="stack">
            <div className="row between">
                <h1>{t.nav.dashboard}</h1>
                <button className="btn primary" onClick={() => setCreating(true)}>{t.nav.newService}</button>
            </div>
            {creating ? <NewProject onDone={(id) => { setCreating(false); onCreated(); if (id) onOpen(id); }}/> : null}
            {summaries.length === 0 && !creating ? <p className="muted">서비스를 만들어 시작하세요.</p> : null}
            <div className="grid-cards">
                {summaries.map((s) => (
                    <button key={s.project.id} className="card clickable" onClick={() => onOpen(s.project.id)}>
                        <div className="row between">
                            <h2 style={{margin: 0}}>{s.project.name}</h2>
                            {s.lastRun ? <RunBadge status={s.lastRun.status}/> : <span className="badge">실행 전</span>}
                        </div>
                        <p className="muted small">{s.project.goal || '목표 미입력'}</p>
                        <div className="row small">
                            {s.activeRuns > 0 ? <span className="badge info">진행 {s.activeRuns}</span> : null}
                            {s.waitingRuns > 0 ? <span className="badge warn">사람 대기 {s.waitingRuns}</span> : null}
                            {s.pausedRuns > 0 ? <span className="badge warn">정지 {s.pausedRuns}</span> : null}
                            {s.failedRuns > 0 ? <span className="badge bad">막힘 {s.failedRuns}</span> : null}
                            {s.inbox > 0 ? <span className="badge warn">내 할 일 {s.inbox}</span> : null}
                        </div>
                        {s.lastRun ? <p className="muted small" style={{marginTop: 8}}>최근: {s.lastRun.title} · {shortTime(s.lastRun.startedAt)}</p> : null}
                    </button>
                ))}
            </div>
            <div>
                <button className="btn small" onClick={toggleArchived}>{showArchived ? '보관된 서비스 숨기기' : t.nav.archived}</button>
                <ErrorBox error={archiveAction.error}/>
                {showArchived ? (
                    <div className="stack" style={{marginTop: 8}}>
                        {archived.length === 0 ? <p className="muted small">보관된 서비스가 없습니다.</p> : null}
                        {archived.map((s) => (
                            <div key={s.project.id} className="card row between">
                                <span>{s.project.name} <span className="badge">보관됨</span></span>
                                <span className="row">
                                    <button className="btn small" onClick={() => onOpen(s.project.id)}>기록 보기</button>
                                    <button className="btn small" onClick={async () => {
                                        await archiveAction.run(() => api.setArchived(s.project.id, false));
                                        setArchived((a) => a.filter((x) => x.project.id !== s.project.id));
                                        onCreated();
                                    }}>{t.project.restore}</button>
                                </span>
                            </div>
                        ))}
                    </div>
                ) : null}
            </div>
        </div>
    );
}

function NewProject({onDone}: { onDone: (id?: string) => void }) {
    const [name, setName] = useState('');
    const [goal, setGoal] = useState('');
    const action = useAction();
    const submit = async () => {
        const p = await action.run(() => api.createProject({name, goal, instructions: '', mode: 'review'}));
        if (p) onDone(p.id);
    };
    return (
        <form className="card stack" onSubmit={(e) => { e.preventDefault(); submit(); }} aria-label="새 서비스">
            <h2>{t.nav.newService}</h2>
            <label className="field"><span>{t.project.name}</span>
                <input value={name} onChange={(e) => setName(e.target.value)} placeholder="예: 게임 서비스" autoFocus/></label>
            <label className="field"><span>{t.project.goal}</span>
                <textarea value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="이 서비스로 이루려는 것"/></label>
            <ErrorBox error={action.error}/>
            <div className="row">
                <button className="btn primary" type="submit" disabled={action.busy || !name.trim()}>{t.project.create}</button>
                <button className="btn" type="button" onClick={() => onDone()}>{t.common.cancel}</button>
            </div>
        </form>
    );
}
