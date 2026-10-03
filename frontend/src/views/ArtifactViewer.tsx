import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
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
                    {data.binary ? <p className="muted">텍스트가 아닌 파일입니다.</p> : <pre className="content">{pretty(data.content, data.type)}</pre>}
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
