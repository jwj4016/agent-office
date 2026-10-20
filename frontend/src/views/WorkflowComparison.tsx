import {useLoad} from '../live';
import {api} from '../api';
import {t} from '../i18n';
import type {WorkflowNode, WorkflowSpec} from '../types';
import {StatusBadge} from '../ui';

export function WorkflowComparison({projectId, before, after}: {projectId: string; before?: WorkflowSpec; after: WorkflowSpec}) {
    const team = useLoad(() => api.assignments(projectId), [projectId]);
    if (!before?.nodes) return <p className="muted small">이전 요청에는 기준 초안이 저장되지 않아 변경 전후 비교를 제공할 수 없습니다.</p>;
    const fields = (n: WorkflowNode, spec: WorkflowSpec): Record<string, string> => ({
        '이름': n.title, '종류': t.nodeKind[n.kind], '담당자': team.data?.find((a) => a.id === n.assignmentId)?.displayName || n.assignmentId || '미정',
        '선행 업무': n.dependsOn.map((id) => spec.nodes.find((x) => x.id === id)?.title ?? id).join(', ') || '없음',
        '업무 지시': n.instructions || '없음',
        '입력': (n.inputs ?? []).map((i) => `${i.name} ← ${spec.nodes.find((x) => x.id === i.fromStep)?.title ?? i.fromStep}.${i.outputKey}${i.required === false ? ' (선택)' : ''}`).join('\n') || '없음',
        '결과': (n.outputs ?? []).map(({key, type, ...options}) => `${key} · ${t.outputType[type]}${Object.keys(options).length ? ` · ${JSON.stringify(options)}` : ''}`).join('\n') || '없음',
        '수정 요청 대상': (n.reworkTargets ?? []).map((id) => spec.nodes.find((x) => x.id === id)?.title ?? id).join(', ') || '없음',
        '완료 조건': JSON.stringify(n.completion ?? {}), '실행 제한': JSON.stringify(n.limits ?? {}),
        '분기': JSON.stringify(n.routing ?? {}), '회의': JSON.stringify(n.meeting ?? {}),
    });
    const ids = [...new Set([...before.nodes.map((n) => n.id), ...after.nodes.map((n) => n.id)])];
    const changed = ids.filter((id) => JSON.stringify(before.nodes.find((n) => n.id === id)) !== JSON.stringify(after.nodes.find((n) => n.id === id)));
    return <details className="card subtle tight" open>
        <summary>변경 전후 비교 · 업무 {changed.length}개</summary>
        <div className="stack" style={{marginTop: 8}}>
            {before.title !== after.title ? <div className="small">흐름 이름: {before.title} → {after.title}</div> : null}
            {changed.map((id) => {
                const old = before.nodes.find((n) => n.id === id), next = after.nodes.find((n) => n.id === id);
                const left = old ? fields(old, before) : {}, right = next ? fields(next, after) : {};
                const useful = (v: string) => !!v && !['없음', '[]', '{}', '미정'].includes(v);
                const keys = [...new Set([...Object.keys(left), ...Object.keys(right)])].filter((key) => left[key] !== right[key] && (old && next || useful(left[key]) || useful(right[key])));
                return <section key={id} className="stack tight">
                    <div className="row"><strong>{next?.title ?? old?.title}</strong><StatusBadge tone={!old ? 'ok' : !next ? 'bad' : 'info'}>{!old ? '추가' : !next ? '삭제' : '변경'}</StatusBadge></div>
                    <div className="workflow-diff">
                        <div className="card tight"><strong className="small">변경 전</strong>{keys.map((key) => <div className="small" key={key}><span className="muted">{key}</span><div className="pre">{left[key] ?? '없음'}</div></div>)}</div>
                        <div className="card tight"><strong className="small">변경 후</strong>{keys.map((key) => <div className="small" key={key}><span className="muted">{key}</span><div className="pre">{right[key] ?? '없음'}</div></div>)}</div>
                    </div>
                </section>;
            })}
            {!changed.length && before.title === after.title ? <p className="small muted">바뀌는 설정이 없습니다.</p> : null}
        </div>
    </details>;
}
