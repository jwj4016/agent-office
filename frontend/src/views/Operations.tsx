import {AlertTriangle, Building, CircleDollarSign, Inbox as InboxIcon} from 'lucide-react';
import type {Route} from '../App';
import {api} from '../api';
import {useLoad} from '../live';
import type {DepartmentLoad, Project} from '../types';
import {Banner, Button, ErrorBox, Icon, Section, StatusBadge} from '../ui';
import {WorkStatusBadge} from './Work';

// waitingDecisions is what waits on the person for a department.
const waitingDecisions = (l: DepartmentLoad) => l.proposals + l.questions + l.collabIncoming + l.autoReviews;

// Operations is a service's operating picture on its overview (plan stage
// 9): budget and cost problems, deliveries in trouble, and each
// department's open work and what waits on it. Every number is counted
// from stored records; nothing is estimated.
export function Operations({project, onRoute}: { project: Project; onRoute: (r: Route) => void }) {
    const o = useLoad(() => api.operations(project.id), [project.id], project.id);
    if (o.error) return <ErrorBox error={o.error}/>;
    if (!o.data) return null;
    const {departments, troubles, troubleCount, usage} = o.data;
    const openWork = (workItemId: string) => onRoute({view: 'project', projectId: project.id, tab: 'work', workItemId});
    const toWork = () => onRoute({view: 'project', projectId: project.id, tab: 'work'});
    return (
        <>
            {usage.holdReason || usage.unknownCostAttempts > 0 || troubleCount > 0 ? (
                <Section title="운영 상태">
                    {usage.holdReason ? (
                        <Banner tone="bad" icon={CircleDollarSign} title="예산 때문에 새 AI 작업을 멈췄습니다"
                                actions={<Button size="sm" onClick={() => onRoute({view: 'project', projectId: project.id, tab: 'settings'})}>예산 설정</Button>}>
                            {usage.holdReason}
                        </Banner>
                    ) : null}
                    {usage.unknownCostAttempts > 0 ? (
                        <Banner tone="info" icon={CircleDollarSign} title={`비용을 알 수 없는 AI 호출 ${usage.unknownCostAttempts}회`}>
                            연결이 비용을 알려 주지 않은 호출입니다. 표시된 비용 합계에는 들어 있지 않습니다.
                        </Banner>
                    ) : null}
                    {troubleCount > 0 ? (
                        <Banner tone="warn" icon={AlertTriangle} title={`부서 전달 실패·보류 ${troubleCount}건`}
                                actions={<Button size="sm" onClick={toWork}>부서 전달함</Button>}>
                            <ul className="stack tight" style={{listStyle: 'none', padding: 0, margin: 0}}>
                                {troubles.map((t) => (
                                    <li key={t.id}>{t.departmentName} · {t.status === 'failed' ? '실패' : '보류'} · {t.eventTitle || '사건'}: {t.reason}</li>
                                ))}
                            </ul>
                        </Banner>
                    ) : null}
                </Section>
            ) : null}
            {departments.length ? (
                <Section title={<><Icon icon={Building} size={18}/>부서별 현황</>} label="부서별 현황" count={departments.length}>
                    <div className="table-wrap">
                        <table className="list">
                            <thead><tr><th>부서</th><th>진행 중 업무</th><th>결과 확인</th><th>막힘</th><th>기다리는 결정</th><th>전달 문제</th></tr></thead>
                            <tbody>
                            {departments.map((l) => (
                                <tr key={l.departmentId || 'none'}>
                                    <td><strong>{l.name}</strong>{l.status !== 'active' ? <> <StatusBadge>사용 안 함</StatusBadge></> : null}</td>
                                    <td>{l.open}</td>
                                    <td>{l.resultsToCheck ? <StatusBadge tone="info">{l.resultsToCheck}</StatusBadge> : 0}</td>
                                    <td>{l.blocked ? <StatusBadge tone="bad">{l.blocked}</StatusBadge> : 0}</td>
                                    <td>{waitingDecisions(l) ? (
                                        <button className="btn ghost small" onClick={() => onRoute({view: 'inbox'})}
                                                title={`제안 ${l.proposals} · 질문 ${l.questions} · 협업 ${l.collabIncoming} · 자동 처리 ${l.autoReviews}`}>
                                            <Icon icon={InboxIcon} size={14}/>{waitingDecisions(l)}</button>
                                    ) : 0}</td>
                                    <td>{l.failedDeliveries + l.heldDeliveries ? (
                                        <StatusBadge tone="warn">실패 {l.failedDeliveries} · 보류 {l.heldDeliveries}</StatusBadge>
                                    ) : 0}</td>
                                </tr>
                            ))}
                            </tbody>
                        </table>
                    </div>
                    {departments.some((l) => l.attention.length) ? (
                        <div className="stack tight" role="group" aria-label="막히거나 기다리는 업무">
                            <span className="kind-head">막히거나 기다리는 업무</span>
                            {departments.flatMap((l) => l.attention.map((w) => (
                                <span key={w.id} className="row small">
                                    <span className="muted">{l.name}</span>
                                    <button className="btn ghost small" onClick={() => openWork(w.id)}>{w.title}</button>
                                    <WorkStatusBadge status={w.status} reason={w.reason}/>
                                </span>
                            )))}
                        </div>
                    ) : null}
                </Section>
            ) : null}
        </>
    );
}
