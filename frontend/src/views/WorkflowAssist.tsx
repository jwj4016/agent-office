import {Download, Eye, Loader2, MessageSquare, Sparkles, Wand2, Workflow as WorkflowIcon} from 'lucide-react';
import {useEffect, useRef, useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {AssistKind, AssistRecord, DiagramResult, EditResult, Project, WorkflowSpec} from '../types';
import {Banner, Button, ConfirmButton, DetailHeader, ErrorBox, Field, Icon, Section, StatusBadge, useAction, useToast} from '../ui';
import {DiagramView} from './ArtifactViewer';
import {ActivityFeed} from './AIActivity';
import {WorkflowComparison} from './WorkflowComparison';

const statusLabel: Record<AssistRecord['status'], string> = {
    working: 'AI 작업 중', ready: '검토 대기', failed: '실패', applied: '적용됨', discarded: '버림',
};
const statusTone = {working: 'info', ready: 'warn', failed: 'bad', applied: 'ok', discarded: ''} as const;

// WorkflowAssist lets the person ask an AI to edit the workflow (a
// proposal they review before it replaces the draft) or to draw the
// pipeline as a diagram with the bundled diagram-design skill.
export function WorkflowAssist({project, workflowId, dirty, saveFirst, onApplied, workflowTitle, selectedNode, getEditorState, onClose, onPreview}: {
    project: Project; workflowId: string; dirty: boolean;
    // saveFirst stores unsaved edits so the AI works on what is on screen.
    saveFirst: () => Promise<unknown>;
    onApplied: () => void;
    workflowTitle?: string; selectedNode?: {id: string; title: string};
    getEditorState?: () => {dirty: boolean; revision: number};
    onClose?: () => void; onPreview?: (spec: WorkflowSpec | null) => void;
}) {
    const list = useLoad(() => api.assists(project.id, workflowId), [project.id, workflowId], project.id);
    const conns = useLoad(() => api.connections(), []);
    const [kind, setKind] = useState<AssistKind>('edit');
    const [request, setRequest] = useState('');
    const [connectionId, setConnectionId] = useState('');
    const [model, setModel] = useState('');
    const [scope, setScope] = useState(selectedNode?.id ?? '');
    const [parent, setParent] = useState<AssistRecord | null>(null);
    const [activeId, setActiveId] = useState('');
    const [shown, setShown] = useState(5);
    const requestField = useRef<HTMLTextAreaElement>(null);
    const action = useAction();
    const archived = project.status === 'archived';
    // Clear the canvas preview when this panel closes, including Escape.
    const preview = useRef(onPreview);
    useEffect(() => () => preview.current?.(null), []);

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
        const current = getEditorState?.();
        if (parent && (current?.dirty || (current && current.revision !== parent.baseRevision))) throw new Error('초안이 바뀌었습니다. 추가 수정을 취소하고 지금 흐름으로 새로 요청하세요.');
        if (current?.dirty ?? dirty) await saveFirst();
        const rec = await api.startAssist({projectId: project.id, workflowId, kind, request, connectionId, model,
            ...(parent ? {parentId: parent.id} : {}), ...(kind === 'edit' && scope ? {scopeNodeId: scope} : {})});
        setRequest('');
        setParent(null);
        setActiveId(rec.id);
        list.reload();
    });
    const beforeApply = () => {
        if (getEditorState?.().dirty ?? dirty) throw new Error('저장하지 않은 변경이 있습니다. 먼저 초안을 저장한 뒤 지금 흐름으로 수정안을 다시 요청하세요.');
    };
    const refine = (a: AssistRecord) => {
        setParent(a); setKind('edit'); setScope(a.scopeNodeId ?? ''); setConnectionId(a.connectionId); setModel(a.model); setRequest('');
        requestField.current?.focus(); requestField.current?.scrollIntoView?.({block: 'center'});
    };
    const active = (list.data ?? []).find((a) => a.id === activeId) ?? list.data?.[0];

    const canSubmit = !archived && !!connectionId && (kind === 'diagram' || request.trim() !== '');
    return (
        <div className="stack">
        {onClose ? <DetailHeader title="AI로 업무 흐름 수정" onClose={onClose}><span className="small">{project.name} · {workflowTitle}</span></DetailHeader> : null}
        <Section title={onClose ? '수정 요청' : 'AI 도우미'} label="AI 도우미">
            <div className="card stack">
                {parent ? <Banner title="이전 제안을 추가 수정합니다">요청: {parent.request}<div><Button size="sm" variant="ghost" onClick={() => setParent(null)}>추가 수정 취소</Button></div></Banner> : null}
                <div className="row" role="radiogroup" aria-label="요청 종류">
                    <label className="check"><input type="radio" checked={kind === 'edit'} onChange={() => setKind('edit')}/>
                        <Icon icon={Wand2} size={14}/> 업무 흐름 수정 요청</label>
                    <label className="check"><input type="radio" disabled={!!parent} checked={kind === 'diagram'} onChange={() => setKind('diagram')}/>
                        <Icon icon={WorkflowIcon} size={14}/> 파이프라인 다이어그램 만들기</label>
                </div>
                {kind === 'edit' ? <Field label="수정 범위" hint={scope ? '선택한 업무의 설정만 바꿉니다. 업무 추가·삭제는 전체 흐름으로 요청하세요.' : '업무 추가·삭제와 연결 변경을 포함해 전체 흐름을 수정합니다.'}>
                    <select value={scope} disabled={!!parent || archived} onChange={(e) => setScope(e.target.value)}>
                        <option value="">전체 흐름</option>
                        {selectedNode ? <option value={selectedNode.id}>선택한 업무 · {selectedNode.title}</option> : null}
                        {parent?.scopeNodeId && parent.scopeNodeId !== selectedNode?.id ? <option value={parent.scopeNodeId}>선택한 업무 · {parent.scopeNodeId}</option> : null}
                    </select>
                </Field> : null}
                <Field label={kind === 'edit' ? '어떻게 고칠까요?' : '강조할 점'} optional={kind === 'diagram'}
                       hint={kind === 'edit'
                           ? 'AI가 수정안을 만들면 바뀌는 업무와 검증 결과를 보고 적용할지 정합니다. 적용 전에는 흐름이 바뀌지 않습니다.'
                           : '앱에 포함된 diagram-design 스킬로 지금 흐름을 그림 한 장으로 그립니다.'}>
                    <textarea ref={requestField} value={request} disabled={archived} rows={3} onChange={(e) => setRequest(e.target.value)}
                              placeholder={kind === 'edit' ? '예: 백엔드와 프론트엔드 구현을 동시에 진행하고, 출시 전에 보안 검토를 넣어 줘' : '예: 사람이 하는 승인 단계를 눈에 띄게'}/>
                </Field>
                <details><summary className="small">AI 연결·모델</summary><div className="stack" style={{marginTop: 8}}>
                    <label className="field" style={{flex: 1}}><span>AI 연결</span>
                        <select value={connectionId} disabled={archived} onChange={(e) => setConnectionId(e.target.value)}>
                            {usable.length === 0 ? <option value="">사용할 수 있는 연결 없음</option> : null}
                            {usable.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                        </select></label>
                    <label className="field" style={{width: 180}}><span>모델 (선택)</span>
                        <input value={model} disabled={archived} onChange={(e) => setModel(e.target.value)}/></label>
                </div></details>
                <p className="muted small" style={{margin: 0}}>요청할 때 화면의 변경을 먼저 저장합니다.</p>
                <div className="row">
                    <ConfirmButton label={kind === 'edit' ? (parent ? '추가 수정안 요청' : '수정안 요청') : '다이어그램 만들기'} confirmLabel="AI 사용량을 써서 요청"
                                   icon={Sparkles} className="btn primary" disabled={!canSubmit || action.busy} onConfirm={submit}/>
                </div>
                <ErrorBox error={action.error ?? list.error}/>
            </div>
            <div className="stack" aria-label="AI 도우미 기록">
                {(list.data ?? []).slice(0, shown).map((a) => <AssistCard key={a.id} a={a} projectId={project.id} archived={archived} onChanged={list.reload} onApplied={onApplied}
                    beforeApply={beforeApply} onRefine={() => refine(a)} onActivity={() => setActiveId(a.id)} onPreview={onPreview}/>)}
                {(list.data?.length ?? 0) > shown ? <Button size="sm" onClick={() => setShown(shown + 5)}>이전 요청 더 보기</Button> : null}
            </div>
        </Section>
        {active ? <Section title="AI 활동" label="수정 요청 AI 활동">
            <Field label="확인할 요청"><select value={active.id} onChange={(e) => setActiveId(e.target.value)}>
                {(list.data ?? []).map((a) => <option key={a.id} value={a.id}>{a.request || '파이프라인 다이어그램'} · {statusLabel[a.status]}</option>)}
            </select></Field>
            <ActivityFeed key={active.id} projectId={project.id} assistId={active.id}/>
        </Section> : null}
        </div>
    );
}

function AssistCard({a, projectId, archived, onChanged, onApplied, beforeApply, onRefine, onActivity, onPreview}: {
    a: AssistRecord; projectId: string; archived: boolean; onChanged: () => void; onApplied: () => void; beforeApply: () => void;
    onRefine: () => void; onActivity: () => void; onPreview?: (spec: WorkflowSpec | null) => void;
}) {
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
                <p className="muted small" style={{margin: 0}}>{a.scopeNodeId ? `선택 업무: ${a.scopeNodeId}` : '전체 흐름'}{a.parentId ? ' · 이전 제안 추가 수정' : ''}</p>
                <Button size="sm" variant="ghost" icon={MessageSquare} onClick={onActivity}>활동 기록 보기</Button>
                {a.status === 'failed' ? <Banner tone="bad">{a.error}</Banner> : null}
                {a.status === 'ready' && a.kind === 'edit' ? <EditProposal r={a.result as EditResult}/> : null}
                {a.kind === 'edit' && (a.status === 'ready' || a.status === 'applied') && 'workflow' in a.result ? <>
                    <WorkflowComparison projectId={projectId} before={a.baseWorkflow} after={(a.result as EditResult).workflow}/>
                    {onPreview ? <Button size="sm" icon={Eye} onClick={() => onPreview((a.result as EditResult).workflow)}>캔버스에서 수정안 보기</Button> : null}
                </> : null}
                {a.status === 'ready' && a.kind === 'diagram' ? <DiagramPreview a={a}/> : null}
                <ErrorBox error={action.error}/>
                {a.status === 'ready' ? (
                    <div className="row">
                        {a.kind === 'edit' ? (
                            <Button variant="primary" disabled={archived} busy={action.busy} onClick={() => action.run(async () => {
                                beforeApply();
                                await api.applyAssist(a.id);
                                toast('수정안을 적용했습니다. 저장·검증 후 버전을 확정하세요');
                                onChanged();
                                onApplied();
                            })}>초안에 적용</Button>
                        ) : (
                            <Button icon={Download} busy={action.busy} onClick={() => action.run(async () => {
                                const path = await api.saveWorkflowDiagram(a.id);
                                if (path) toast(`저장했습니다: ${path}`);
                            })}>HTML로 저장…</Button>
                        )}
                        {a.kind === 'edit' ? <Button disabled={archived} onClick={onRefine}>추가 수정 요청</Button> : null}
                        <ConfirmButton label="버리기" confirmLabel="버리기" className="btn ghost" disabled={archived}
                                       onConfirm={() => action.run(async () => { await api.discardAssist(a.id); onChanged(); })}/>
                    </div>
                ) : null}
                {a.kind === 'edit' && a.status === 'ready' ? <p className="muted small">초안에 적용한 뒤 버전을 확정해야 실행할 수 있습니다. 실행 중인 버전은 유지됩니다.</p> : null}
            </div>
        </details>
    );
}

function EditProposal({r}: { r: EditResult }) {
    // Go sends null for an empty list (a proposal with no issues).
    const issues = r.issues ?? [];
    const errors = issues.filter((i) => i.severity === 'error');
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
            {issues.length ? (
                <Banner tone={errors.length ? 'bad' : 'warn'} title={errors.length ? `적용 뒤 고쳐야 할 문제 ${errors.length}건` : `실행 전에 해결할 항목 ${issues.length}건`}>
                    <ul className="issue-list" style={{marginTop: 4}}>
                        {issues.map((i, n) => <li key={n} className={i.severity}>{i.nodeId ? <strong>{i.nodeId}: </strong> : null}{i.message}</li>)}
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
