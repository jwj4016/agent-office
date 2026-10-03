import {api} from '../api';
import {t} from '../i18n';
import {useLive, useLoad} from '../live';
import type {Project, RunDetail} from '../types';
import {ConfirmButton, ErrorBox, RunBadge, StepBadge, useAction, useDetail} from '../ui';
import {ArtifactViewer} from './ArtifactViewer';

export function Runs({project, runId, onOpen}: { project: Project; runId?: string; onOpen: (runId?: string) => void }) {
    const list = useLoad(() => api.runs(project.id), [project.id], project.id);
    if (runId) return <RunView project={project} runId={runId} onBack={() => onOpen(undefined)}/>;
    return (
        <div className="stack">
            <ErrorBox error={list.error}/>
            {(list.data ?? []).length === 0 ? <p className="muted">아직 실행 기록이 없습니다. 업무 흐름에서 버전을 확정하고 실행하세요.</p> : null}
            <table className="list">
                <thead><tr><th>흐름</th><th>버전</th><th>상태</th><th>진행</th></tr></thead>
                <tbody>
                {(list.data ?? []).map((r) => {
                    const done = r.steps.filter((s) => s.status === 'succeeded' || s.status === 'skipped').length;
                    return (
                        <tr key={r.id}>
                            <td><button className="btn small" onClick={() => onOpen(r.id)}>{r.title}</button></td>
                            <td>v{r.versionNumber}</td>
                            <td><RunBadge status={r.status}/></td>
                            <td className="small">{done}/{r.steps.length}</td>
                        </tr>
                    );
                })}
                </tbody>
            </table>
        </div>
    );
}

function RunView({project, runId, onBack}: { project: Project; runId: string; onBack: () => void }) {
    const {data, error, reload} = useLoad(() => api.run(project.id, runId), [project.id, runId], project.id);
    const live = useLive();
    const action = useAction();
    const setDetail = useDetail();
    if (error) return <ErrorBox error={error}/>;
    if (!data) return <p className="muted">{t.common.loading}</p>;
    const run: RunDetail = data;
    const finished = run.status === 'succeeded' || run.status === 'cancelled';
    const act = (fn: () => Promise<void>) => action.run(async () => { await fn(); reload(); });
    const openArtifact = (artifactId: string) =>
        setDetail(<ArtifactViewer projectId={project.id} artifactId={artifactId} onClose={() => setDetail(null)}/>);

    return (
        <div className="stack">
            <div className="row between">
                <div className="row">
                    <button className="btn small" onClick={onBack}>← 목록</button>
                    <h2 style={{margin: 0}}>{run.title} <span className="muted small">v{run.versionNumber}</span></h2>
                    <RunBadge status={run.status}/>
                </div>
                {!finished ? (
                    <div className="row">
                        {run.paused
                            ? <button className="btn" onClick={() => act(() => api.resumeRun(project.id, runId))}>재개</button>
                            : <button className="btn" onClick={() => act(() => api.pauseRun(project.id, runId))}>일시 정지</button>}
                        <ConfirmButton label="실행 취소" confirmLabel="취소하기" className="btn danger"
                                       onConfirm={() => act(() => api.cancelRun(project.id, runId))}/>
                    </div>
                ) : null}
            </div>
            {run.paused ? <p className="notice">일시 정지 중입니다. 진행 중인 업무는 계속되고, 새 업무는 시작하지 않습니다.</p> : null}
            {run.status === 'cancelled' ? <p className="muted small">취소된 실행입니다. 이미 만들어진 파일이나 외부 조치는 되돌려지지 않았습니다.</p> : null}
            <ErrorBox error={action.error}/>
            <table className="list" aria-label="업무 진행">
                <thead><tr><th>업무</th><th>상태</th><th>회차</th><th>결과</th><th/></tr></thead>
                <tbody>
                {run.steps.map((s) => (
                    <tr key={s.id}>
                        <td>{s.title}<div className="muted small">{t.nodeKind[s.kind]}</div></td>
                        <td>
                            <StepBadge status={s.status}/>
                            {s.error ? <div className="small" style={{color: 'var(--danger)'}}>{s.error}</div> : null}
                            {s.attemptId && live.deltas[s.attemptId] ? (
                                <pre className="content small" aria-live="polite">{live.deltas[s.attemptId].slice(-600)}</pre>
                            ) : null}
                        </td>
                        <td className="small">{s.round > 0 ? `수정 ${s.round}회` : ''}{s.attempt > 1 ? ` · 시도 ${s.attempt}` : ''}</td>
                        <td>
                            <div className="row">
                                {s.artifacts.map((a) => (
                                    <button key={a.id} className="btn small" onClick={() => openArtifact(a.id)}>{a.outputKey} v{a.version}</button>
                                ))}
                            </div>
                        </td>
                        <td>
                            {['failed', 'interrupted', 'cancelled'].includes(s.status) && run.status !== 'cancelled'
                                ? <button className="btn small" onClick={() => act(() => api.retryStep(project.id, runId, s.id))}>다시 시도</button> : null}
                        </td>
                    </tr>
                ))}
                </tbody>
            </table>
            <p className="muted small">사람이 맡은 업무와 승인은 ‘내 할 일’에서 처리합니다.</p>
        </div>
    );
}
