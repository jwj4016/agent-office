import {Bot, RotateCcw, X} from 'lucide-react';
import {useState} from 'react';
import {api} from '../api';
import {t} from '../i18n';
import {useLoad} from '../live';
import type {AutoStart, Department, DepartmentAutomation, OutputType, Project} from '../types';
import {Banner, Button, DetailHeader, ErrorBox, Loading, StatusBadge, type Tone, useAction, useDetail, useToast} from '../ui';

const outputTypes: OutputType[] = ['markdown', 'json', 'report', 'file', 'code_change', 'diagram'];

const off: DepartmentAutomation = {
    autoStart: false, categories: [], connectionIds: [], outputTypes: [], workflowVersionIds: [], autoShare: false, autoCollab: false,
};

// rulesOf reads a department's stored rules; anything missing is off.
export function rulesOf(d: Department): DepartmentAutomation {
    const raw = (d.automation ?? {}) as Partial<DepartmentAutomation>;
    return {...off, ...raw, categories: raw.categories ?? [], connectionIds: raw.connectionIds ?? [], outputTypes: raw.outputTypes ?? [],
        workflowVersionIds: raw.workflowVersionIds ?? []};
}

// AutomationPanel sets what a department lets happen without a person in
// an auto-mode service, in the detail pane. Everything starts off.
export function AutomationPanel({project, department}: { project: Project; department: Department }) {
    const categories = useLoad(() => api.categories(project.id), [project.id]);
    const connections = useLoad(() => api.connections(), []);
    const workflows = useLoad(() => api.workflowChoices(project.id), [project.id], project.id);
    const [rules, setRules] = useState<DepartmentAutomation>(rulesOf(department));
    const action = useAction();
    const toast = useToast();
    const setDetail = useDetail();
    const archived = project.status === 'archived';
    if (categories.error || connections.error || workflows.error) return <ErrorBox error={categories.error || connections.error || workflows.error}/>;
    if (!categories.data || !connections.data || !workflows.data) return <Loading/>;
    const toggle = <K extends 'categories' | 'connectionIds' | 'outputTypes' | 'workflowVersionIds'>(key: K, v: DepartmentAutomation[K][number], on: boolean) =>
        setRules({...rules, [key]: on ? [...(rules[key] as string[]), v] : (rules[key] as string[]).filter((x) => x !== v)});
    const save = () => action.run(async () => {
        await api.setDepartmentAutomation(project.id, department.id, rules);
        toast(`${department.name} 자동화 규칙을 저장했습니다`);
    });
    const flag = (key: 'autoStart' | 'autoShare' | 'autoCollab', label: string, hint: string) => (
        <label className="check block">
            <input type="checkbox" checked={rules[key]} disabled={archived} onChange={(e) => setRules({...rules, [key]: e.target.checked})}/>
            {label}<span className="muted xsmall"> · {hint}</span>
        </label>
    );
    return (
        <form className="stack" aria-label={`${department.name} 자동화 규칙`} onSubmit={(e) => { e.preventDefault(); save(); }}>
            <DetailHeader title={`${department.name} 자동화`} onClose={() => setDetail(null)}/>
            {project.mode !== 'auto' ? (
                <Banner tone="info">이 서비스는 검토 모드입니다. 아래 규칙은 서비스를 자동 모드로 바꿨을 때만 적용됩니다.</Banner>
            ) : null}
            <p className="muted small">사건으로 생긴 업무를 자동으로 시작하려면 서비스가 자동 모드이고, 서비스 자동 실행 허용 연결과 이 부서의 규칙을 모두 만족해야 합니다.
                맞지 않으면 받은 일의 ‘검토 필요’로 넘어옵니다. 완료에 필요한 협업과 업무 완료는 항상 사람이 결정합니다.</p>
            <fieldset className="stack tight">
                <legend className="field-label">허용할 일</legend>
                {flag('autoStart', '사건으로 생긴 업무 자동 시작', 'AI가 계획을 쓰고 승인된 계획으로 실행합니다')}
                {flag('autoShare', '자동 실행 결과를 초안으로 공유', '확정은 사람이 합니다')}
                {flag('autoCollab', '참고용 협업 요청을 대표 AI가 결정', '수락·거절·질문을 사람이 확인하고 되돌릴 수 있습니다')}
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">자동 시작할 분류 <span className="optional">(고르지 않으면 모든 분류)</span></legend>
                <span className="row">
                    {categories.data.map((c) => (
                        <label key={c} className="check"><input type="checkbox" checked={rules.categories.includes(c)} disabled={archived}
                                                               onChange={(e) => toggle('categories', c, e.target.checked)}/>{c}</label>
                    ))}
                </span>
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">쓸 수 있는 AI 연결 <span className="optional">(고르지 않으면 서비스 자동 실행 허용 연결 그대로)</span></legend>
                {connections.data.map((c) => (
                    <label key={c.id} className="check block"><input type="checkbox" checked={rules.connectionIds.includes(c.id)} disabled={archived}
                                                                     onChange={(e) => toggle('connectionIds', c.id, e.target.checked)}/>{c.name}</label>
                ))}
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">작업 범위: 만들 수 있는 결과 <span className="optional">(고르지 않으면 문서·JSON·보고서만)</span></legend>
                <span className="row">
                    {outputTypes.map((ty) => (
                        <label key={ty} className="check"><input type="checkbox" checked={rules.outputTypes.includes(ty)} disabled={archived}
                                                                onChange={(e) => toggle('outputTypes', ty, e.target.checked)}/>{t.outputType[ty]}</label>
                    ))}
                </span>
            </fieldset>
            <fieldset className="stack tight">
                <legend className="field-label">자동 실행할 수 있는 업무 흐름 버전 <span className="optional">(선택)</span></legend>
                {workflows.data.length === 0 ? <span className="muted small">확정한 업무 흐름이 없습니다.</span> : null}
                {workflows.data.map((w) => (
                    <label key={w.versionId} className="check block"><input type="checkbox" checked={rules.workflowVersionIds.includes(w.versionId)} disabled={archived}
                                                                            onChange={(e) => toggle('workflowVersionIds', w.versionId, e.target.checked)}/>{w.title} {w.number}판</label>
                ))}
            </fieldset>
            <ErrorBox error={action.error}/>
            <div className="form-actions">
                <Button type="submit" variant="primary" busy={action.busy} disabled={archived}>{t.common.save}</Button>
            </div>
        </form>
    );
}

const autoTone: Record<AutoStart['status'], Tone> = {
    pending: 'info', planning: 'info', approved: 'info', started: 'info', finished: 'ok', review: 'warn', cancelled: '',
};

// AutoStartBanner is the auto starter's state for one work item, with
// what the person can do when it needs them.
export function AutoStartBanner({a, onDone, showProject}: { a: AutoStart; onDone: () => void; showProject?: boolean }) {
    const action = useAction();
    const toast = useToast();
    const review = a.status === 'review';
    return (
        <Banner tone={review ? 'warn' : 'info'} icon={Bot}
                title={<>{showProject ? `${a.projectName} · ‘${a.workTitle}’ ` : '자동 시작 '}<StatusBadge tone={autoTone[a.status]}>{t.autoStartStatus[a.status]}</StatusBadge></>}
                actions={review ? <span className="row nowrap">
                    <Button size="sm" icon={RotateCcw} busy={action.busy}
                            onClick={() => action.run(async () => { await api.retryAutoStart(a.projectId, a.id); toast('자동 시작 조건을 다시 확인합니다'); onDone(); })}>다시 확인</Button>
                    <Button size="sm" variant="ghost" icon={X} busy={action.busy}
                            onClick={() => action.run(async () => { await api.dismissAutoStart(a.projectId, a.id, '사람이 직접 진행'); onDone(); })}>직접 진행</Button>
                </span> : null}>
            {review ? a.reason : a.note}
            <ErrorBox error={action.error}/>
        </Banner>
    );
}
