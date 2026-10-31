import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Decisions, Project, RequestItem, ServiceRequest} from '../types';
import {pendingCount} from './Judgments';
import {CompanyRequests, RequestCard, ServiceRequests} from './Requests';

const project: Project = {
    id: 'prj-1', organizationId: 'org-default', name: '빌라노트', goal: '', instructions: '', workspacePath: '', budget: {},
    mode: 'review', status: 'active', createdAt: '', updatedAt: '',
};
const base: ServiceRequest = {
    id: 'req-1', organizationId: 'org-default', projectId: 'prj-1', projectName: '빌라노트', parentId: '', previousId: '',
    body: '빌라노트 코드 내 폴더로 보내 줘', status: 'proposed', outcome: 'action', round: 1, answer: '', question: '', reply: '',
    reason: '코드가 아직 기준 코드에 없습니다', error: '', holdReason: '', maxAttempts: 3, attempts: 1, createdAt: '2026-10-30T00:00:00Z',
    updatedAt: '', closedAt: '', items: [], children: [],
};
const item = (over: Partial<RequestItem>): RequestItem => ({
    id: 'rqi-1', requestId: 'req-1', projectId: 'prj-1', round: 1, position: 1, kind: 'action', payload: {action: 'deliver_code', runId: 'run-1', title: 'MVP', reason: '전달'},
    status: 'proposed', workItemId: '', workTitle: '', workStatus: '', childRequestId: '', result: null, error: '', decidedBy: '', decisionNote: '',
    decidedAt: '', createdAt: '', ...over,
});

beforeEach(() => vi.clearAllMocks());

describe('RequestCard', () => {
    it('delivers code into the chosen folder once the person approves', async () => {
        fakeApi.finalCode.mockResolvedValue({runId: 'run-1', repo: '', appRepo: true, base: '', steps: [{stepId: 'web', title: '웹', commit: 'c'}],
            commit: 'c', stat: {files: 1, insertions: 1, deletions: 0}, included: false, deliveries: []});
        fakeApi.chooseFolder.mockResolvedValue('/Users/me/dev/villanote');
        fakeApi.runRequestAction.mockResolvedValue({item: item({status: 'done'}), cancelled: false});
        const onChanged = vi.fn();
        render(<LiveProvider><RequestCard req={{...base, items: [item({})]}} onChanged={onChanged}/></LiveProvider>);
        const action = await screen.findByRole('group', {name: '조치 제안: 코드 결과 전달'});
        expect(within(action).getByRole('button', {name: '승인: 기준 코드만 갱신'})).toBeInTheDocument();
        fireEvent.click(within(action).getByRole('button', {name: '폴더 선택…'}));
        fireEvent.click(await within(action).findByRole('button', {name: '승인: 폴더로 꺼내기'}));
        await waitFor(() => expect(fakeApi.runRequestAction).toHaveBeenCalledWith(
            {projectId: 'prj-1', itemId: 'rqi-1', method: 'folder', folder: '/Users/me/dev/villanote', useAsWorkspace: true}));
        expect(onChanged).toHaveBeenCalled();
    });

    it('fills in the folder the desk suggested and says which desk the AI set up', async () => {
        fakeApi.finalCode.mockResolvedValue({runId: 'run-1', repo: '', appRepo: true, base: '', steps: [{stepId: 'web', title: '웹', commit: 'c'}],
            commit: 'c', stat: {files: 1, insertions: 1, deletions: 0}, included: false, deliveries: []});
        fakeApi.runRequestAction.mockResolvedValue({item: item({status: 'done'}), cancelled: false});
        const suggested = item({payload: {action: 'deliver_code', runId: 'run-1', reason: '전달', folder: '/Users/me/dev/workspace/villanote'}});
        render(<LiveProvider><RequestCard req={{...base, staffedDesk: '빌라노트 접수 담당', items: [suggested]}} onChanged={() => {}}/></LiveProvider>);
        expect(screen.getByText(/‘빌라노트 접수 담당’를 새로 배치했습니다/)).toBeInTheDocument();
        const action = await screen.findByRole('group', {name: '조치 제안: 코드 결과 전달'});
        expect(within(action).getByText(/접수 담당이 요청을 보고 제안한 폴더/)).toBeInTheDocument();
        fireEvent.click(within(action).getByRole('button', {name: '승인: 폴더로 꺼내기'}));
        await waitFor(() => expect(fakeApi.runRequestAction).toHaveBeenCalledWith(expect.objectContaining(
            {method: 'folder', folder: '/Users/me/dev/workspace/villanote'})));
    });

    it('registers proposed work with the person’s title and answers a question', async () => {
        const work = item({id: 'rqi-2', kind: 'work', payload: {title: '역전세 지표 추가', doneCriteria: '리포트에 나온다', departmentId: 'dep-1'}});
        fakeApi.departments.mockResolvedValue([{id: 'dep-1', name: '개발팀'}]);
        render(<LiveProvider><RequestCard req={{...base, outcome: 'work', items: [work]}} onChanged={() => {}}/></LiveProvider>);
        const group = await screen.findByRole('group', {name: '업무 제안: 역전세 지표 추가'});
        await waitFor(() => expect(group).toHaveTextContent('부서 개발팀'));
        fireEvent.change(within(group).getByLabelText('업무 제목'), {target: {value: '역전세 위험 지표 추가'}});
        fireEvent.click(within(group).getByRole('button', {name: '업무 등록'}));
        await waitFor(() => expect(fakeApi.acceptRequestWork).toHaveBeenCalledWith('prj-1', 'rqi-2', {title: '역전세 위험 지표 추가'}));
    });

    it('sends the answer to the desk’s question', async () => {
        render(<LiveProvider><RequestCard req={{...base, status: 'question', outcome: 'question', question: '어느 기준을 고칠까요?'}} onChanged={() => {}}/></LiveProvider>);
        expect(screen.getByText('어느 기준을 고칠까요?')).toBeInTheDocument();
        fireEvent.change(screen.getByLabelText('답'), {target: {value: '역전세'}});
        fireEvent.click(screen.getByRole('button', {name: '답 보내기'}));
        await waitFor(() => expect(fakeApi.answerRequest).toHaveBeenCalledWith('prj-1', 'req-1', '역전세'));
    });
});

describe('following a request to the end', () => {
    it('shows the goal, progress, order and why a start waits, then the report to confirm', async () => {
        const build = item({id: 'rqi-b', kind: 'work', status: 'accepted', workItemId: 'w-b', workTitle: '지표 구현', workStatus: 'done',
            payload: {title: '지표 구현', index: 1}});
        const check = item({id: 'rqi-c', kind: 'work', status: 'accepted', workItemId: 'w-c', workTitle: '지표 검증', workStatus: 'received',
            payload: {title: '지표 검증', index: 2, after: [1]}, autoStatus: 'review', autoNote: '자동 실행에는 서비스 예산을 먼저 정해야 합니다'});
        const req: ServiceRequest = {...base, status: 'in_progress', outcome: 'work', goalId: 'goal-1', goalTitle: '역전세 지표 출시', items: [build, check]};
        const onOpenWork = vi.fn();
        const {rerender} = render(<LiveProvider><RequestCard req={req} onChanged={() => {}} onOpenWork={onOpenWork}/></LiveProvider>);
        const progress = screen.getByRole('group', {name: '업무 진행'});
        expect(progress).toHaveTextContent('업무 2건 중 1건 완료');
        expect(progress).toHaveTextContent('모두 끝나면 접수 담당이 완료 보고를 씁니다');
        fireEvent.click(within(progress).getByRole('button', {name: '역전세 지표 출시'}));
        expect(onOpenWork).toHaveBeenCalledWith('prj-1', 'goal-1');
        const checkGroup = screen.getByRole('group', {name: '업무 제안: 지표 검증'});
        expect(checkGroup).toHaveTextContent('앞 업무: ‘지표 구현’');
        expect(checkGroup).toHaveTextContent('자동 시작 못 함: 자동 실행에는 서비스 예산을 먼저 정해야 합니다');

        rerender(<LiveProvider><RequestCard req={{...req, status: 'done', report: '## 요약\n지표를 출시했습니다.', items: [build, {...check, workStatus: 'done'}]}}
                                            onChanged={() => {}} onOpenWork={onOpenWork}/></LiveProvider>);
        const report = screen.getByRole('group', {name: '완료 보고'});
        expect(report).toHaveTextContent('지표를 출시했습니다.');
        fireEvent.click(within(report).getByRole('button', {name: '확인했습니다'}));
        await waitFor(() => expect(fakeApi.markReportRead).toHaveBeenCalledWith('prj-1', 'req-1'));
    });
});

describe('request screens', () => {
    it('says when the service has no desk and still takes the request', async () => {
        fakeApi.deskView.mockResolvedValueOnce({desk: {projectId: 'prj-1', name: '', assignmentId: '', connectionId: '', model: '', instructions: '',
            budget: {}, enabled: true}, exists: false});
        fakeApi.submitRequest.mockResolvedValue(base);
        const onTeam = vi.fn();
        render(<LiveProvider><ServiceRequests project={project} onTeam={onTeam}/></LiveProvider>);
        fireEvent.click(await screen.findByRole('button', {name: '접수 담당 정하기'}));
        expect(onTeam).toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('요청'), {target: {value: '기준 고쳐 줘'}});
        fireEvent.click(screen.getByRole('button', {name: '요청 보내기'}));
        await waitFor(() => expect(fakeApi.submitRequest).toHaveBeenCalledWith('prj-1', '기준 고쳐 줘', ''));
    });

    it('asks the company, or a chosen service directly', async () => {
        fakeApi.submitRequest.mockResolvedValue(base);
        render(<LiveProvider><CompanyRequests services={[project]}/></LiveProvider>);
        fireEvent.change(screen.getByLabelText('요청'), {target: {value: '코드 보내 줘'}});
        fireEvent.click(screen.getByRole('button', {name: '요청 보내기'}));
        await waitFor(() => expect(fakeApi.submitRequest).toHaveBeenCalledWith('', '코드 보내 줘', ''));
        fireEvent.change(screen.getByLabelText('요청'), {target: {value: '코드 보내 줘'}});
        fireEvent.change(screen.getByLabelText('받을 곳'), {target: {value: 'prj-1'}});
        fireEvent.click(screen.getByRole('button', {name: '요청 보내기'}));
        await waitFor(() => expect(fakeApi.submitRequest).toHaveBeenLastCalledWith('prj-1', '코드 보내 줘', ''));
    });

    it('counts requests waiting on the person as to-dos', () => {
        const d = {questions: [], proposals: [], collabs: [], notices: [], aiDecisions: [], autoReviews: [], results: [], goals: {},
            requests: [base, {...base, id: 'req-2', projectId: '', projectName: ''}]} as Decisions;
        expect(pendingCount(d)).toBe(2);
        expect(pendingCount(d, 'prj-1')).toBe(1);
    });
});
