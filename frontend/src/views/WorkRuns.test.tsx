import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {AIJob, Assignment, PlanJobView, Project, RunDetail, WorkItem, WorkPlanView} from '../types';
import {ToastProvider} from '../ui';
import {WorkRun} from './WorkRuns';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};
const item: WorkItem = {
    id: 'w1', projectId: 'prj-1', kind: 'work', parentId: '', departmentId: 'dev', assigneeAssignmentId: '', title: '구현 검토', description: '',
    doneCriteria: '일정 정리', priority: 'normal', status: 'received', statusReason: '', categories: [], dueAt: '', createdAt: '', updatedAt: 't1', closedAt: '',
};
const assignments = [{id: 'a-dev', displayName: '개발 AI', actorKind: 'ai'}] as Assignment[];

const view = (over: Partial<WorkPlanView> = {}): WorkPlanView => ({
    plan: {workItemId: 'w1', projectId: 'prj-1', assignmentId: 'a-dev', instructions: '', materials: '', outputs: [{key: 'result', type: 'markdown'}], updatedAt: ''},
    saved: false, preview: '', problem: '', ...over,
});
const run = (over: Partial<RunDetail>): RunDetail => ({
    id: 'run-1', projectId: 'prj-1', versionId: 'v1', versionNumber: 1, title: '업무: 구현 검토', status: 'succeeded', paused: false,
    steps: [{id: 'work', title: '구현 검토', kind: 'task', status: 'succeeded', assignmentId: 'a-dev', attemptId: 'att', generation: 1, attempt: 1, round: 0,
        artifacts: [{id: 'art-1', outputKey: 'result', version: 1, type: 'markdown', path: '', hash: '', validity: 'valid'}]}],
    ...over,
} as RunDetail);

const onChanged = vi.fn();
const onOpenRun = vi.fn();
const Shell = ({children}: { children: ReactNode }) => <LiveProvider><ToastProvider>{children}</ToastProvider></LiveProvider>;
const renderIt = () => render(<Shell><WorkRun project={project} item={item} assignments={assignments} onChanged={onChanged} onOpenRun={onOpenRun}/></Shell>);

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.workRuns.mockResolvedValue([]);
    fakeApi.planJobs.mockResolvedValue([]);
});

const job = (over: Partial<AIJob>): AIJob => ({
    id: 'aij-1', projectId: 'prj-1', kind: 'plan', lane: 'interactive', requestKey: 'k', workItemId: 'w1', assignmentId: 'a-dev',
    status: 'ready', error: '', holdReason: '', maxAttempts: 3, attempts: 1, appliedAt: '', createdAt: '', updatedAt: '', ...over,
});
const proposal = {assignmentId: 'a-dev', instructions: '화면별로 나눈다', materials: '', outputs: [{key: 'review', type: 'markdown' as const}], notes: '3개 가정'};

describe('work plan and runs', () => {
    it('saves a plan before anything can run', async () => {
        fakeApi.workPlan.mockResolvedValue(view());
        fakeApi.saveWorkPlan.mockResolvedValue(view().plan);
        renderIt();
        const form = await screen.findByRole('form', {name: '작업 계획'});
        expect(screen.getByRole('button', {name: '계획대로 실행'})).toBeDisabled();
        fireEvent.change(within(form).getByLabelText(/작업 지침/), {target: {value: '기존 화면을 먼저 본다'}});
        fireEvent.click(within(form).getByRole('button', {name: '산출물 추가'}));
        fireEvent.change(within(form).getByLabelText('산출물 2 이름'), {target: {value: 'schedule'}});
        fireEvent.change(within(form).getByLabelText('산출물 2 형식'), {target: {value: 'json'}});
        fireEvent.click(within(form).getByRole('button', {name: '계획 저장'}));
        await waitFor(() => expect(fakeApi.saveWorkPlan).toHaveBeenCalledWith(expect.objectContaining({
            projectId: 'prj-1', workItemId: 'w1', assignmentId: 'a-dev', instructions: '기존 화면을 먼저 본다',
            outputs: [{key: 'result', type: 'markdown'}, {key: 'schedule', type: 'json'}],
        })));
    });

    it('runs the saved plan, shows the result and sends a change request', async () => {
        fakeApi.workPlan.mockResolvedValue(view({saved: true, preview: '## 업무\n구현 검토\n\n## 완료 조건\n일정 정리'}));
        fakeApi.workRuns.mockResolvedValue([run({})]);
        fakeApi.readArtifact.mockResolvedValue({id: 'art-1', outputKey: 'result', version: 1, type: 'markdown', path: '', hash: '', validity: 'valid',
            stepId: 'work', content: '# 검토 결과\n2주 소요', binary: false, truncated: false, hashOk: true});
        fakeApi.startWork.mockResolvedValue({runId: 'run-2', issues: []});
        renderIt();
        expect(await screen.findByText('실행에 전달되는 내용 보기')).toBeInTheDocument();
        fireEvent.click(await screen.findByRole('button', {name: 'result 결과'}));
        expect(await screen.findByText(/2주 소요/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: 'v1 실행 화면에서 보기'}));
        expect(onOpenRun).toHaveBeenCalledWith('run-1');
        const form = screen.getByRole('form', {name: '수정 요청'});
        fireEvent.change(within(form).getByLabelText('수정 요청 내용'), {target: {value: '일정 근거 추가'}});
        fireEvent.click(within(form).getByRole('button', {name: '수정 요청하고 다시 실행'}));
        await waitFor(() => expect(fakeApi.startWork).toHaveBeenCalledWith('prj-1', 'w1', '일정 근거 추가'));
        expect(onChanged).toHaveBeenCalled();
    });

    it('does not start a second run while one is active', async () => {
        fakeApi.workPlan.mockResolvedValue(view({saved: true, preview: '...'}));
        fakeApi.workRuns.mockResolvedValue([run({status: 'running'})]);
        renderIt();
        expect(await screen.findByText(/실행 중입니다/)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: '계획대로 실행'})).toBeDisabled();
        expect(screen.getByRole('button', {name: '계획 저장'})).toBeDisabled();
    });

    it('explains why a saved plan cannot run', async () => {
        fakeApi.workPlan.mockResolvedValue(view({saved: true, problem: '닫힌 업무는 실행할 수 없습니다. 먼저 다시 여세요'}));
        renderIt();
        expect(await screen.findByText('닫힌 업무는 실행할 수 없습니다. 먼저 다시 여세요')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: '계획대로 실행'})).toBeDisabled();
    });
});

describe('AI plan proposals', () => {
    it('asks for a plan with a fresh request id per click and applies a ready proposal', async () => {
        fakeApi.workPlan.mockResolvedValue(view());
        fakeApi.requestWorkPlan.mockResolvedValue(job({status: 'pending'}));
        fakeApi.planJobs.mockResolvedValue([{job: job({}), proposal, stale: false}] satisfies PlanJobView[]);
        fakeApi.applyPlanJob.mockResolvedValue(view().plan);
        renderIt();
        const card = await screen.findByLabelText('AI 계획 제안');
        expect(within(card).getByText('화면별로 나눈다')).toBeInTheDocument();
        expect(within(card).getByText('review (문서(Markdown))')).toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: '이 계획 적용'}));
        await waitFor(() => expect(fakeApi.applyPlanJob).toHaveBeenCalledWith('prj-1', 'aij-1'));
        expect(fakeApi.startWork).not.toHaveBeenCalled();
        // While a request is being sent the button is locked; the next
        // click after it is a new request with a new id.
        const ask = screen.getByRole('button', {name: 'AI에게 계획 제안 받기'});
        fireEvent.click(ask);
        fireEvent.click(ask);
        await waitFor(() => expect(fakeApi.requestWorkPlan).toHaveBeenCalledTimes(1));
        await waitFor(() => expect(ask).not.toBeDisabled());
        fireEvent.click(ask);
        await waitFor(() => expect(fakeApi.requestWorkPlan).toHaveBeenCalledTimes(2));
        const [first, second] = fakeApi.requestWorkPlan.mock.calls.map((c) => c[2]);
        expect(first).toBeTruthy();
        expect(first).not.toBe(second);
    });

    it('refuses a stale proposal and offers a retry after a failure', async () => {
        fakeApi.workPlan.mockResolvedValue(view());
        fakeApi.planJobs.mockResolvedValue([{job: job({}), proposal, stale: true}] satisfies PlanJobView[]);
        const {unmount} = renderIt();
        const card = await screen.findByLabelText('AI 계획 제안');
        expect(within(card).getByText('업무가 바뀜')).toBeInTheDocument();
        expect(within(card).getByRole('button', {name: '이 계획 적용'})).toBeDisabled();
        unmount();

        fakeApi.planJobs.mockResolvedValue([{job: job({status: 'failed', error: 'rate limited', attempts: 3}), stale: false}] satisfies PlanJobView[]);
        renderIt();
        expect(await screen.findByText(/rate limited \(시도 3회\)/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '다시 시도'}));
        await waitFor(() => expect(fakeApi.retryAIJob).toHaveBeenCalledWith('prj-1', 'aij-1'));
    });
});
