import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {CodeChange} from '../types';
import {ErrorBox} from '../ui';

export function ArtifactViewer({projectId, artifactId, onClose}: { projectId: string; artifactId: string; onClose: () => void }) {
    const {data, error} = useLoad(() => api.readArtifact(projectId, artifactId), [projectId, artifactId]);
    return (
        <div className="stack">
            <div className="row between"><h2>결과 보기</h2><button className="btn small" onClick={onClose}>{t.common.close}</button></div>
            <ErrorBox error={error}/>
            {!data ? <p className="muted">{t.common.loading}</p> : (
                <>
                    <div className="small">
                        <div><strong>{data.stepId}.{data.outputKey}</strong> · 버전 {data.version} · {t.outputType[data.type] ?? data.type}</div>
                        <div className="mono muted">sha256 {data.hash.slice(0, 16)}…</div>
                        <div className="row" style={{marginTop: 4}}>
                            {data.validity === 'valid' ? <span className="badge ok">최신 결과</span> : <span className="badge warn">이전 결과(재작업 전)</span>}
                            {data.hashOk ? <span className="badge ok">파일 확인됨</span> : <span className="badge bad">파일이 기록과 다릅니다</span>}
                        </div>
                    </div>
                    {data.binary ? <p className="muted">텍스트가 아닌 파일입니다.</p>
                        : data.type === 'code_change' ? <CodeChangeView content={data.content}/>
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
