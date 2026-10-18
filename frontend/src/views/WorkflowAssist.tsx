import {Download, Loader2, Sparkles, Wand2, Workflow as WorkflowIcon} from 'lucide-react';
import {useEffect, useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {AssistKind, AssistRecord, DiagramResult, EditResult, Project} from '../types';
import {Banner, Button, ConfirmButton, ErrorBox, Field, Icon, Section, StatusBadge, useAction, useToast} from '../ui';
import {DiagramView} from './ArtifactViewer';

const statusLabel: Record<AssistRecord['status'], string> = {
    working: 'AI 작업 중', ready: '검토 대기', failed: '실패', applied: '적용됨', discarded: '버림',
};
const statusTone = {working: 'info', ready: 'warn', failed: 'bad', applied: 'ok', discarded: ''} as const;

// WorkflowAssist lets the person ask an AI to edit the workflow (a
// proposal they review before it replaces the draft) or to draw the
// pipeline as a diagram with the bundled diagram-design skill.
export function WorkflowAssist({project, workflowId, dirty, saveFirst, onApplied}: {
    project: Project; workflowId: string; dirty: boolean;
    // saveFirst stores unsaved edits so the AI works on what is on screen.
    saveFirst: () => Promise<unknown>;
    onApplied: () => void;
}) {
    const list = useLoad(() => api.assists(project.id, workflowId), [project.id, workflowId]);
    const conns = useLoad(() => api.connections(), []);
    const [kind, setKind] = useState<AssistKind>('edit');
    const [request, setRequest] = useState('');
    const [connectionId, setConnectionId] = useState('');
    const [model, setModel] = useState('');
    const action = useAction();
    const archived = project.status === 'archived';

    // Model APIs cannot read the skill's files, so they cannot draw.
    const usable = (conns.data ?? []).filter((c) => c.usable && (kind === 'edit' || (c.provider !== 'claude_api' && c.provider !== 'openai_api')));
    useEffect(() => {
        if (!usable.some((c) => c.id === connectionId)) setConnectionId(usable.find((c) => c.provider !== 'test')?.id ?? usable[0]?.id ?? '');
    }, [usable, connectionId]);

    const working = (list.data ?? []).some((a) => a.status === 'working');
    useEffect(() => {
        if (!working) return;
        const timer = window.setInterval(list.reload, 2000);
        return () => window.clearInterval(timer);
    }, [working, list.reload]);

    const submit = () => action.run(async () => {
        if (dirty) await saveFirst();
        await api.startAssist({projectId: project.id, workflowId, kind, request, connectionId, model});
        setRequest('');
        list.reload();
    });

    const canSubmit = !archived && !!connectionId && (kind === 'diagram' || request.trim() !== '');
    return (
        <Section title="AI 도우미" label="AI 도우미">
            <div className="card stack">
                <div className="row" role="radiogroup" aria-label="요청 종류">
                    <label className="check"><input type="radio" checked={kind === 'edit'} onChange={() => setKind('edit')}/>
                        <Icon icon={Wand2} size={14}/> 업무 흐름 수정 요청</label>
                    <label className="check"><input type="radio" checked={kind === 'diagram'} onChange={() => setKind('diagram')}/>
                        <Icon icon={WorkflowIcon} size={14}/> 파이프라인 다이어그램 만들기</label>
                </div>
                <Field label={kind === 'edit' ? '어떻게 고칠까요?' : '강조할 점'} optional={kind === 'diagram'}
                       hint={kind === 'edit'
                           ? 'AI가 수정안을 만들면 바뀌는 업무와 검증 결과를 보고 적용할지 정합니다. 적용 전에는 흐름이 바뀌지 않습니다.'
                           : '앱에 포함된 diagram-design 스킬로 지금 흐름을 그림 한 장으로 그립니다.'}>
                    <textarea value={request} disabled={archived} rows={3} onChange={(e) => setRequest(e.target.value)}
                              placeholder={kind === 'edit' ? '예: 백엔드와 프론트엔드 구현을 동시에 진행하고, 출시 전에 보안 검토를 넣어 줘' : '예: 사람이 하는 승인 단계를 눈에 띄게'}/>
                </Field>
                <div className="row">
                    <label className="field" style={{flex: 1}}><span>AI 연결</span>
                        <select value={connectionId} disabled={archived} onChange={(e) => setConnectionId(e.target.value)}>
                            {usable.length === 0 ? <option value="">사용할 수 있는 연결 없음</option> : null}
                            {usable.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                        </select></label>
                    <label className="field" style={{width: 180}}><span>모델 (선택)</span>
                        <input value={model} disabled={archived} onChange={(e) => setModel(e.target.value)}/></label>
                </div>
                {dirty ? <p className="muted small" style={{margin: 0}}>저장하지 않은 수정이 있어 요청할 때 먼저 저장합니다.</p> : null}
                <div className="row">
                    <ConfirmButton label={kind === 'edit' ? '수정안 요청' : '다이어그램 만들기'} confirmLabel="AI 사용량을 써서 요청"
                                   icon={Sparkles} className="btn primary" disabled={!canSubmit || action.busy} onConfirm={submit}/>
                </div>
                <ErrorBox error={action.error ?? list.error}/>
            </div>
            <div className="stack" aria-label="AI 도우미 기록">
                {(list.data ?? []).slice(0, 5).map((a) => <AssistCard key={a.id} a={a} onChanged={list.reload} onApplied={onApplied}/>)}
            </div>
        </Section>
    );
}

function AssistCard({a, onChanged, onApplied}: { a: AssistRecord; onChanged: () => void; onApplied: () => void }) {
    const action = useAction();
    const toast = useToast();
    const open = a.status === 'working' || a.status === 'ready' || a.status === 'failed';
    return (
        <details className="card tight" open={open} aria-label={`${a.kind === 'edit' ? '수정 요청' : '다이어그램'} ${a.createdAt}`}>
            <summary className="row">
                <strong>{a.kind === 'edit' ? '수정 요청' : '파이프라인 다이어그램'}</strong>
                <StatusBadge tone={statusTone[a.status]}>{a.status === 'working' ? <Icon icon={Loader2} size={13} className="spin"/> : null}{statusLabel[a.status]}</StatusBadge>
                <span className="muted small">수정 {a.baseRevision} 기준 · {a.createdAt.slice(0, 16).replace('T', ' ')}</span>
            </summary>
            <div className="stack" style={{marginTop: 8}}>
                {a.request ? <p className="small" style={{margin: 0}}>요청: {a.request}</p> : null}
                {a.status === 'failed' ? <Banner tone="bad">{a.error}</Banner> : null}
                {a.status === 'ready' && a.kind === 'edit' ? <EditProposal r={a.result as EditResult}/> : null}
                {a.status === 'ready' && a.kind === 'diagram' ? <DiagramPreview a={a}/> : null}
                <ErrorBox error={action.error}/>
                {a.status === 'ready' ? (
                    <div className="row">
                        {a.kind === 'edit' ? (
                            <Button variant="primary" busy={action.busy} onClick={() => action.run(async () => {
                                await api.applyAssist(a.id);
                                toast('수정안을 적용했습니다. 저장·검증 후 버전을 확정하세요');
                                onChanged();
                                onApplied();
                            })}>수정안 적용</Button>
                        ) : (
                            <Button icon={Download} busy={action.busy} onClick={() => action.run(async () => {
                                const path = await api.saveWorkflowDiagram(a.id);
                                if (path) toast(`저장했습니다: ${path}`);
                            })}>HTML로 저장…</Button>
                        )}
                        <ConfirmButton label="버리기" confirmLabel="버리기" className="btn ghost"
                                       onConfirm={() => action.run(async () => { await api.discardAssist(a.id); onChanged(); })}/>
                    </div>
                ) : null}
            </div>
        </details>
    );
}

function EditProposal({r}: { r: EditResult }) {
    const errors = r.issues.filter((i) => i.severity === 'error');
    const list = (label: string, items: string[]) => items.length
        ? <div className="small"><strong>{label}</strong> {items.join(', ')}</div> : null;
    return (
        <div className="stack tight">
            {r.notes ? <p style={{margin: 0}}>{r.notes}</p> : null}
            {list('추가', r.changes.added)}
            {list('변경', r.changes.changed)}
            {list('삭제', r.changes.removed)}
            {!r.changes.added.length && !r.changes.changed.length && !r.changes.removed.length
                ? <p className="muted small" style={{margin: 0}}>바뀌는 업무가 없습니다.</p> : null}
            {r.issues.length ? (
                <Banner tone={errors.length ? 'bad' : 'warn'} title={errors.length ? `적용 뒤 고쳐야 할 문제 ${errors.length}건` : `실행 전에 해결할 항목 ${r.issues.length}건`}>
                    <ul className="issue-list" style={{marginTop: 4}}>
                        {r.issues.map((i, n) => <li key={n} className={i.severity}>{i.nodeId ? <strong>{i.nodeId}: </strong> : null}{i.message}</li>)}
                    </ul>
                </Banner>
            ) : <p className="small" style={{margin: 0}}>검증 문제 없음</p>}
        </div>
    );
}

function DiagramPreview({a}: { a: AssistRecord }) {
    const html = useLoad(() => api.workflowDiagram(a.id), [a.id]);
    const r = a.result as DiagramResult;
    if (html.error) return <ErrorBox error={html.error}/>;
    if (!html.data) return <p className="muted small">불러오는 중…</p>;
    return (
        <div className="stack tight">
            <DiagramView content={html.data} title="파이프라인"/>
            {!r.checked ? <p className="muted small" style={{margin: 0}}>Python이 없어 스킬의 자동 검사는 건너뛰었습니다.</p> : null}
        </div>
    );
}
