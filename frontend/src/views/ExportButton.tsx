import {useState} from 'react';
import {api} from '../api';
import type {ExportResult} from '../types';
import {ErrorBox, useAction} from '../ui';

// ExportButton saves one service as a zip without keys or login data.
export function ExportButton({projectId}: { projectId: string }) {
    const action = useAction();
    const [done, setDone] = useState<ExportResult | null>(null);
    const run = () => action.run(async () => {
        const res = await api.exportProject(projectId);
        setDone(res.path ? res : null);
    });
    return (
        <div className="stack">
            <div className="row">
                <button className="btn" disabled={action.busy} onClick={run}>{action.busy ? '내보내는 중…' : '내보내기 (키 제외)'}</button>
            </div>
            <ErrorBox error={action.error}/>
            {done ? (
                <div className="small" role="status">
                    <div>저장했습니다: <span className="mono">{done.path}</span></div>
                    <div>기록 {done.rows.toLocaleString()}건 · 결과 파일 {done.artifacts}개
                        {done.masked > 0 ? ` · 키로 보이는 값 ${done.masked}곳을 가렸습니다` : ''}</div>
                    {done.missing.length ? <div style={{color: 'var(--danger)'}}>찾지 못한 결과 파일 {done.missing.length}개는 목록에만 남겼습니다.</div> : null}
                </div>
            ) : null}
        </div>
    );
}
