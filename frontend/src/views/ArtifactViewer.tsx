import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {CodeChange} from '../types';
import {CheckCircle2, Download, FolderOpen, History, ShieldAlert, ShieldCheck} from 'lucide-react';
import {Button, DetailHeader, ErrorBox, Loading, StatusBadge, useAction, useToast} from '../ui';

export function ArtifactViewer({projectId, artifactId, onClose}: { projectId: string; artifactId: string; onClose: () => void }) {
    const {data, error} = useLoad(() => api.readArtifact(projectId, artifactId), [projectId, artifactId]);
    const action = useAction();
    const toast = useToast();
    // A copy saved elsewhere opens in any program; the app's own file stays
    // as recorded (its hash is checked).
    const saveAs = () => action.run(async () => {
        const path = await api.saveArtifactAs(projectId, artifactId);
        if (path) toast(`저장했습니다: ${path}`);
    });
    const reveal = () => action.run(() => api.revealArtifact(projectId, artifactId));
    return (
        <div className="stack">
            <DetailHeader title="결과 보기" onClose={onClose}>
                {data ? <span className="small"><strong>{data.stepId}.{data.outputKey}</strong> · 버전 {data.version} · {t.outputType[data.type] ?? data.type}</span> : null}
            </DetailHeader>
            <ErrorBox error={error}/>
            {!data ? <Loading rows={2}/> : (
                <>
                    <div className="row">
                        {data.validity === 'valid' ? <StatusBadge tone="ok" icon={CheckCircle2}>최신 결과</StatusBadge> : <StatusBadge tone="warn" icon={History}>이전 결과(재작업 전)</StatusBadge>}
                        {data.hashOk ? <StatusBadge tone="ok" icon={ShieldCheck}>파일 확인됨</StatusBadge> : <StatusBadge tone="bad" icon={ShieldAlert}>파일이 기록과 다릅니다</StatusBadge>}
                        <span className="mono muted" title={data.hash}>sha256 {data.hash.slice(0, 12)}…</span>
                    </div>
                    <div className="row">
                        <Button size="sm" icon={Download} busy={action.busy} onClick={saveAs}>파일로 저장…</Button>
                        <Button size="sm" variant="ghost" icon={FolderOpen} disabled={action.busy} onClick={reveal}>폴더에서 보기</Button>
                        <span className="muted xsmall">다른 프로그램에서 열려면 저장한 사본을 쓰세요. 앱 폴더의 파일을 고치면 ‘파일이 기록과 다릅니다’가 됩니다.</span>
                    </div>
                    <ErrorBox error={action.error}/>
                    {data.binary ? <p className="muted">텍스트가 아닌 파일입니다.</p>
                        : data.type === 'code_change' ? <CodeChangeView content={data.content}/>
                            : data.type === 'diagram' && !data.truncated ? <DiagramView content={data.content} title={`${data.stepId}.${data.outputKey}`}/>
                                : <pre className="content">{pretty(data.content, data.type)}</pre>}
                    {data.truncated ? <p className="muted small">앞부분 1MB만 표시합니다.</p> : null}
                </>
            )}
        </div>
    );
}

function pretty(content: string, type: string) {
    if (type === 'json' || type === 'report' || type === 'code_change') {
        try {
            return JSON.stringify(JSON.parse(content), null, 2);
        } catch {
            return content;
        }
    }
    return content;
}

const changeLabel: Record<string, string> = {added: '추가', modified: '수정', deleted: '삭제', renamed: '이름 변경', copied: '복사', typechange: '형식 변경'};

// CodeChangeView shows a code result the app recorded from Git: what
// changed since the run's base commit, and the diff.
export function CodeChangeView({content}: { content: string }) {
    let cc: CodeChange | null = null;
    try {
        cc = JSON.parse(content) as CodeChange;
    } catch {
        return <pre className="content">{content}</pre>;
    }
    if (!cc || !cc.commit) return <pre className="content">{pretty(content, 'code_change')}</pre>;
    return (
        <div className="stack small">
            <div>브랜치 <span className="mono">{cc.branch}</span> · 커밋 <span className="mono">{cc.commit.slice(0, 10)}</span>
                {' '}(실행 기준 <span className="mono">{cc.baseCommit.slice(0, 10)}</span>부터)</div>
            <div>{cc.diffStat.files}개 파일 · +{cc.diffStat.insertions} −{cc.diffStat.deletions}</div>
            {cc.summary ? <div>요약: {cc.summary}</div> : null}
            <ul aria-label="변경된 파일" style={{margin: 0}}>
                {cc.changes.map((c) => <li key={c.path}><span className="badge">{changeLabel[c.status] ?? c.status}</span> <span className="mono">{c.path}</span></li>)}
            </ul>
            <details open>
                <summary>diff{cc.patchTruncated ? ' (앞부분만)' : ''}</summary>
                <pre className="content">{cc.patch || '(변경 없음)'}</pre>
            </details>
            <p className="muted">저장소에 반영하려면 이 브랜치를 검토한 뒤 직접 병합하세요 (예: git merge {cc.branch}).</p>
        </div>
    );
}

// DiagramView previews a diagram result. The frame is sandboxed with no
// permissions: no scripts, no same-origin access to the app.
export function DiagramView({content, title}: { content: string; title: string }) {
    return (
        <div className="stack small">
            <iframe className="diagram-frame" sandbox="" srcDoc={content} title={`다이어그램 ${title}`}/>
            <details>
                <summary>HTML 원본</summary>
                <pre className="content">{content}</pre>
            </details>
        </div>
    );
}
