import {FolderOutput, GitBranch, GitMerge, RefreshCw} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {useLoad} from '../live';
import type {CodeDeliveryRecord, DeliveryMethod, Project, RunDetail} from '../types';
import {Banner, Button, ErrorBox, Field, Section, shortTime, useAction, useToast} from '../ui';
import {FolderInput} from './FolderInput';

const methodLabel: Record<DeliveryMethod, string> = {
    base: '기준 코드 갱신', folder: '폴더로 꺼내기', branch: '결과 브랜치', advance: '현재 브랜치에 반영',
};

function doneText(d: CodeDeliveryRecord): string {
    switch (d.method) {
    case 'folder':
        return d.workspace ? `‘${d.folder}’에 꺼내고 작업 폴더로 정했습니다` : `‘${d.folder}’에 꺼냈습니다`;
    case 'base':
        return '서비스 기준 코드를 갱신했습니다';
    case 'branch':
        return `${d.branch} 브랜치를 만들었습니다`;
    default:
        return d.note ? `${d.branch} 브랜치를 만들었습니다. ${d.note}` : `${d.branch} 브랜치를 만들고 현재 브랜치에 반영했습니다`;
    }
}

// CodeHandover hands a succeeded run's code to the person (company-requests
// plan §4.6): into the service's base code and a folder, or as a branch of
// the person's own repository.
export function CodeHandover({project, run}: { project: Project; run: RunDetail }) {
    const {data, error, reload} = useLoad(() => api.finalCode(project.id, run.id), [project.id, run.id], project.id);
    const action = useAction();
    const toast = useToast();
    const [folder, setFolder] = useState('');
    const [useAsWorkspace, setUseAsWorkspace] = useState(true);
    if (error) return <ErrorBox error={error}/>;
    if (!data || data.steps.length === 0) return null;
    const fc = data;
    const deliver = (method: DeliveryMethod) => action.run(async () => {
        try {
            const d = await api.deliverCode({projectId: project.id, runId: run.id, method, folder, useAsWorkspace});
            toast(doneText(d));
            if (method === 'folder') setFolder('');
        } finally {
            reload();
        }
    });
    const what = fc.steps.length === 1 ? `‘${fc.steps[0].title}’의 결과` : `${fc.steps.map((s) => `‘${s.title}’`).join(', ')}의 결과를 합친 코드`;

    return (
        <Section title="코드 결과" label="코드 결과">
            <div className="card stack">
                <p>
                    {what}
                    {fc.commit ? <span className="muted small"> · 파일 {fc.stat.files}개 (+{fc.stat.insertions} −{fc.stat.deletions}) · <span className="mono">{fc.commit.slice(0, 8)}</span></span> : null}
                </p>
                {fc.included ? (
                    <Banner tone="ok" title="서비스 기준 코드에 들어 있습니다">다음 실행은 이 코드를 이어서 시작합니다.</Banner>
                ) : fc.appRepo ? (
                    <Banner tone="warn" title="아직 서비스 기준 코드에 들어 있지 않습니다">
                        지금 다음 실행을 시작하면 이 결과 없이 시작합니다. 폴더로 꺼내거나 기준 코드를 갱신하세요.
                    </Banner>
                ) : (
                    <Banner tone="warn" title="작업 폴더의 현재 브랜치에 들어 있지 않습니다">
                        결과 브랜치를 만들어 직접 합치거나, ‘현재 브랜치에도 반영’을 고르세요. 커밋하지 않은 변경이 있거나 브랜치가 그사이 바뀌었으면 반영하지 않습니다.
                    </Banner>
                )}

                {fc.appRepo ? (
                    <div className="stack">
                        <Field label="꺼낼 폴더" hint="비어 있거나 아직 없는 폴더만 됩니다. 폴더 선택 창에서 새 폴더를 만들 수 있습니다.">
                            <FolderInput value={folder} onChange={setFolder} title="코드를 꺼낼 폴더 선택"/>
                        </Field>
                        <label className="check block">
                            <input type="checkbox" checked={useAsWorkspace} onChange={(e) => setUseAsWorkspace(e.target.checked)}/>
                            이 폴더를 서비스 작업 폴더로 쓰기<span className="muted xsmall"> · 새로 확정하는 업무 흐름 버전과 새 업무부터 이 폴더에서 작업합니다. 이미 확정한 버전은 그대로 앱 저장소에서 작업하고, 그 기준 코드도 이 결과로 갱신됩니다</span>
                        </label>
                        <div className="row">
                            <Button variant="primary" icon={FolderOutput} disabled={!folder} busy={action.busy} onClick={() => deliver('folder')}>폴더로 꺼내기</Button>
                            <Button icon={RefreshCw} busy={action.busy} onClick={() => deliver('base')}>기준 코드만 갱신</Button>
                        </div>
                    </div>
                ) : (
                    <div className="row">
                        <Button variant="primary" icon={GitBranch} busy={action.busy} onClick={() => deliver('branch')}>결과 브랜치 만들기</Button>
                        <Button icon={GitMerge} busy={action.busy} onClick={() => deliver('advance')}>현재 브랜치에도 반영</Button>
                    </div>
                )}
                <ErrorBox error={action.error}/>

                {fc.deliveries.length ? (
                    <ul className="stack tight small" style={{listStyle: 'none', padding: 0, margin: 0}} aria-label="전달 기록">
                        {fc.deliveries.slice().reverse().map((d, i) => (
                            <li key={i} className={d.error ? 'bad' : undefined}>
                                <span className="muted">{shortTime(d.at)}</span> {methodLabel[d.method]}
                                {d.folder ? <> · <span className="mono">{d.folder}</span></> : null}
                                {d.branch ? <> · <span className="mono">{d.branch}</span></> : null}
                                {d.error ? <> · 실패: {d.error}</> : d.note ? <> · {d.note}</> : null}
                            </li>
                        ))}
                    </ul>
                ) : null}
            </div>
        </Section>
    );
}
