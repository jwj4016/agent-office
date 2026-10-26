import {CheckCircle2, ExternalLink, PlayCircle, Target} from 'lucide-react';
import type {ReactNode} from 'react';
import {api} from '../api';
import type {AutoStart, CollabRequest, Decisions, GoalRef, Judgment, Proposal, RequiredNotice, ResultReview} from '../types';
import {Banner, Button, ErrorBox, Icon, relativeTime, Section, useAction} from '../ui';
import {AutoStartBanner} from './Automation';
import {CollabCard} from './Collab';
import {groupByEvent, ProposalGroup, QuestionCard} from './Judgments';

// What a department waits on the person for, by kind (plan stage 9).
export type DecisionKind = 'result' | 'collab' | 'notice' | 'auto' | 'question' | 'proposal';
export const decisionKinds: DecisionKind[] = ['result', 'collab', 'notice', 'auto', 'question', 'proposal'];
export const decisionKindLabel: Record<DecisionKind, string> = {
    result: '결과 검토', collab: '협업', notice: '영향 확인', auto: '자동 처리', question: '질문', proposal: '제안',
};

type Item =
    | { kind: 'result'; projectId: string; projectName: string; work: string; v: ResultReview }
    | { kind: 'collab'; projectId: string; projectName: string; work: string; v: CollabRequest }
    | { kind: 'notice'; projectId: string; projectName: string; work: string; v: RequiredNotice }
    | { kind: 'auto'; projectId: string; projectName: string; work: string; v: AutoStart }
    | { kind: 'question'; projectId: string; projectName: string; work: string; v: Judgment }
    | { kind: 'proposal'; projectId: string; projectName: string; work: string; v: Proposal };

// decisionItems flattens Decisions; each item names the work it is about,
// which decides its shared goal.
function decisionItems(d: Decisions): Item[] {
    return [
        ...d.results.map((v): Item => ({kind: 'result', projectId: v.projectId, projectName: v.projectName, work: v.workItemId, v})),
        ...[...d.collabs, ...d.aiDecisions].map((v): Item => ({kind: 'collab', projectId: v.projectId, projectName: v.projectName, work: v.fromWorkItemId, v})),
        ...d.notices.map((v): Item => ({kind: 'notice', projectId: v.projectId, projectName: v.projectName, work: v.workItemId, v})),
        ...d.autoReviews.map((v): Item => ({kind: 'auto', projectId: v.projectId, projectName: v.projectName, work: v.workItemId, v})),
        ...d.questions.map((v): Item => ({kind: 'question', projectId: v.projectId, projectName: v.projectName, work: v.event.workItemId ?? '', v})),
        ...d.proposals.map((v): Item => ({kind: 'proposal', projectId: v.projectId, projectName: v.projectName, work: v.eventWorkItemId, v})),
    ];
}

export function decisionKindCounts(d: Decisions): Record<DecisionKind, number> {
    const out = Object.fromEntries(decisionKinds.map((k) => [k, 0])) as Record<DecisionKind, number>;
    for (const it of decisionItems(d)) out[it.kind]++;
    return out;
}

type Group = { key: string; projectName: string; goal?: GoalRef; items: Item[] };

// groupByGoal puts items under their service's shared goal; items outside
// a goal come last in each service.
export function groupByGoal(d: Decisions, kind?: DecisionKind): Group[] {
    const groups = new Map<string, Group>();
    for (const it of decisionItems(d)) {
        if (kind && it.kind !== kind) continue;
        const goal = it.work ? d.goals[it.work] : undefined;
        const key = `${it.projectId}:${goal?.id ?? ''}`;
        if (!groups.has(key)) groups.set(key, {key, projectName: it.projectName, goal, items: []});
        groups.get(key)!.items.push(it);
    }
    return [...groups.values()].sort((a, b) =>
        a.projectName.localeCompare(b.projectName) || Number(!a.goal) - Number(!b.goal) || (a.goal?.title ?? '').localeCompare(b.goal?.title ?? ''));
}

// InboxGroups shows the departments' decisions grouped by shared goal, each
// kind under its own heading.
export function InboxGroups({data, kind, onDone, onOpenWork, onOpenRun}: {
    data: Decisions; kind?: DecisionKind; onDone: () => void;
    onOpenWork?: (projectId: string, workItemId: string) => void; onOpenRun?: (projectId: string, runId: string) => void;
}) {
    const groups = groupByGoal(data, kind);
    if (groups.length === 0) return null;
    const services = new Set(groups.map((g) => g.projectName)).size;
    return (
        <>
            {groups.map((g) => (
                <Section key={g.key} label={g.goal ? `공동 목표 ${g.goal.title}` : `${g.projectName} 공동 목표 없음`} count={g.items.length}
                         title={<><Icon icon={Target} size={18}/>{g.goal ? `공동 목표 ‘${g.goal.title}’` : '공동 목표 밖의 일'}
                             {services > 1 || !g.goal ? <span className="muted small"> · {g.projectName}</span> : null}</>}>
                    <div className="stack">
                        {decisionKinds.map((k) => {
                            const items = g.items.filter((it) => it.kind === k);
                            if (!items.length) return null;
                            return (
                                <div key={k} className="stack tight" role="group" aria-label={decisionKindLabel[k]}>
                                    <span className="kind-head">{decisionKindLabel[k]} <span className="muted">{items.length}</span></span>
                                    {render(k, items, onDone, onOpenWork, onOpenRun)}
                                </div>
                            );
                        })}
                    </div>
                </Section>
            ))}
        </>
    );
}

function render(k: DecisionKind, items: Item[], onDone: () => void,
                onOpenWork?: (projectId: string, workItemId: string) => void, onOpenRun?: (projectId: string, runId: string) => void): ReactNode {
    const open = (projectId: string) => onOpenWork ? (id: string) => onOpenWork(projectId, id) : undefined;
    switch (k) {
        case 'result':
            return items.map((it) => <ResultBanner key={`r-${(it.v as ResultReview).workItemId}`} r={it.v as ResultReview}
                                                   onOpenWork={onOpenWork} onOpenRun={onOpenRun}/>);
        case 'collab':
            return items.map((it) => <CollabCard key={`c-${(it.v as CollabRequest).id}`} r={it.v as CollabRequest} onDone={onDone} showProject
                                                 onOpenWork={open(it.projectId)}/>);
        case 'notice':
            return items.map((it) => <NoticeBanner key={`n-${(it.v as RequiredNotice).id}`} n={it.v as RequiredNotice} onDone={onDone}
                                                   onOpenWork={open(it.projectId)}/>);
        case 'auto':
            return items.map((it) => {
                const a = it.v as AutoStart;
                return <AutoStartBanner key={`a-${a.id}`} a={a} onDone={onDone} showProject
                                        onOpenWork={onOpenWork ? () => onOpenWork(a.projectId, a.workItemId) : undefined}/>;
            });
        case 'question':
            return items.map((it) => <QuestionCard key={`q-${(it.v as Judgment).id}`} j={it.v as Judgment} onDone={onDone} showProject/>);
        case 'proposal':
            return groupByEvent(items.map((it) => it.v as Proposal)).map((g) => (
                <ProposalGroup key={`p-${g[0].projectId}:${g[0].eventSequence}`} proposals={g} onDone={onDone} showProject/>
            ));
    }
}

// ResultBanner is work whose run finished: the person checks the result and
// completes the work or asks for changes in the work item.
function ResultBanner({r, onOpenWork, onOpenRun}: {
    r: ResultReview; onOpenWork?: (projectId: string, workItemId: string) => void; onOpenRun?: (projectId: string, runId: string) => void;
}) {
    return (
        <Banner tone="info" icon={CheckCircle2} title={`${r.projectName} · ‘${r.title}’ 결과를 확인하세요`}
                actions={<span className="row nowrap">
                    {onOpenRun ? <Button size="sm" variant="ghost" icon={PlayCircle} onClick={() => onOpenRun(r.projectId, r.runId)}>실행 보기</Button> : null}
                    {onOpenWork ? <Button size="sm" icon={ExternalLink} onClick={() => onOpenWork(r.projectId, r.workItemId)}>업무 열기</Button> : null}
                </span>}>
            {r.departmentName || '부서 없음'} · 실행이 {relativeTime(r.finishedAt)} 끝났습니다. 업무에서 완료하거나 수정을 요청하세요.
        </Banner>
    );
}

function NoticeBanner({n, onDone, onOpenWork}: { n: RequiredNotice; onDone: () => void; onOpenWork?: (id: string) => void }) {
    const action = useAction();
    const close = (status: 'resolved' | 'dismissed') => action.run(async () => { await api.closeImpactCheck(n.projectId, n.id, status, ''); onDone(); });
    return (
        <>
            <Banner tone="warn" title={`${n.projectName} · ${n.workKind === 'goal' ? '공동 목표' : '업무'} ‘${n.workTitle}’`}
                    actions={<span className="row nowrap">
                        {onOpenWork ? <Button size="sm" variant="ghost" icon={ExternalLink} onClick={() => onOpenWork(n.workItemId)}>업무 열기</Button> : null}
                        <Button size="sm" busy={action.busy} onClick={() => close('resolved')}>확인함</Button>
                        <Button size="sm" variant="ghost" busy={action.busy} onClick={() => close('dismissed')}>무시</Button>
                    </span>}>{n.summary}</Banner>
            <ErrorBox error={action.error}/>
        </>
    );
}
