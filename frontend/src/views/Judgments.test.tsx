import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Department, Judgment, Project, Proposal, SharedDocument, WorkItem} from '../types';
import {ToastProvider} from '../ui';
import {DeliveryInbox} from './Deliveries';
import {DepartmentDecisions, InboxDecisions, SubscriptionPanel} from './Judgments';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const dept = (id: string, name: string): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId: 'a1', memberIds: ['a1'], templateId: '', automation: {}, limits: {},
    status: 'active', createdAt: '', updatedAt: '',
});

const event = {departmentId: 'mkt', title: '캠페인 기획안 1판', categories: [], causeSequence: null, chainRootSequence: null, chainDepth: 0, automated: false};

const proposal = (over: Partial<Proposal>): Proposal => ({
    id: 'p', projectId: 'prj-1', projectName: '캠페인', judgmentId: 'j', position: 1, departmentId: 'dev', departmentName: '개발팀',
    title: '페이지 구현 검토', description: '', doneCriteria: '일정 정리', categories: ['개발'], priority: 'high', status: 'proposed',
    workItemId: '', duplicateOf: '', decisionNote: '', reason: '기획안에 새 페이지가 있다', eventSequence: 7, eventKind: 'document.shared',
    eventTitle: '캠페인 기획안 1판', createdAt: '', ...over,
});

const question = (over: Partial<Judgment>): Judgment => ({
    id: 'q1', projectId: 'prj-1', projectName: '캠페인', deliveryId: 'dl', round: 1, aiJobId: 'job', departmentId: 'fin', departmentName: '재무팀',
    outcome: 'question', reason: '예산을 모른다', evidence: [], question: '예산 한도는 얼마인가요?', answer: '', questionStatus: 'open',
    eventSequence: 7, eventKind: 'document.shared', event, createdAt: '', ...over,
});

const Shell = ({children}: { children: ReactNode }) => <LiveProvider><ToastProvider>{children}</ToastProvider></LiveProvider>;

beforeEach(() => {
    vi.clearAllMocks();
});

describe('department subscriptions', () => {
    it('starts with every kind but a confirmed document and saves categories and guidance', async () => {
        fakeApi.subscriptions.mockResolvedValue([]);
        fakeApi.categories.mockResolvedValue(['개발', '재무']);
        fakeApi.saveSubscription.mockResolvedValue({});
        render(<Shell><SubscriptionPanel project={project} department={dept('dev', '개발팀')}/></Shell>);
        const form = await screen.findByRole('form', {name: '새 구독'});
        expect(within(form).getByRole('checkbox', {name: '자료 공유됨'})).toBeChecked();
        expect(within(form).getByRole('checkbox', {name: /자료 확정됨/})).not.toBeChecked();
        fireEvent.click(within(form).getByRole('checkbox', {name: '개발'}));
        fireEvent.change(within(form).getByLabelText(/판단 관점/), {target: {value: '구현이 필요한지 본다'}});
        fireEvent.click(within(form).getByRole('button', {name: '저장'}));
        await waitFor(() => expect(fakeApi.saveSubscription).toHaveBeenCalled());
        const saved = fakeApi.saveSubscription.mock.calls[0][0] as unknown as Record<string, unknown>;
        expect(saved).toMatchObject({projectId: 'prj-1', departmentId: 'dev', categories: ['개발'], guidance: '구현이 필요한지 본다', enabled: true});
        expect(saved.eventKinds).not.toContain('document.confirmed');
        expect(saved.eventKinds).toContain('document.revised');
    });
});

describe('department decisions', () => {
    it('groups proposals by event, marks duplicates and registers one with edits', async () => {
        fakeApi.judgments.mockResolvedValue([]);
        fakeApi.proposals.mockResolvedValue([
            proposal({id: 'p1'}),
            proposal({id: 'p2', departmentId: 'fin', departmentName: '재무팀', title: '예산 검토', duplicateOf: 'work-1'}),
        ]);
        fakeApi.acceptProposal.mockResolvedValue({title: '페이지 구현(모바일)'} as WorkItem);
        render(<Shell><DepartmentDecisions project={project}/></Shell>);
        const group = await screen.findByRole('region', {name: '자료 공유됨 캠페인 기획안 1판 제안'});
        expect(within(group).getByText('비슷한 업무가 이미 있음')).toBeInTheDocument();
        expect(within(group).getAllByText(/판단 이유/)).toHaveLength(2);
        fireEvent.click(within(group).getByRole('button', {name: '페이지 구현 검토 고쳐서 등록'}));
        const edit = within(group).getByRole('form', {name: '제안 고쳐서 등록'});
        fireEvent.change(within(edit).getByLabelText('제목'), {target: {value: '페이지 구현(모바일)'}});
        fireEvent.click(within(edit).getByRole('button', {name: '등록'}));
        await waitFor(() => expect(fakeApi.acceptProposal).toHaveBeenCalledWith('prj-1', 'p1',
            {title: '페이지 구현(모바일)', doneCriteria: '일정 정리', priority: 'high'}));
        expect(fakeApi.proposals).toHaveBeenCalledWith('prj-1', 'proposed');
    });

    it('decides every proposal of one event at once', async () => {
        fakeApi.judgments.mockResolvedValue([]);
        fakeApi.proposals.mockResolvedValue([proposal({id: 'p1'}), proposal({id: 'p2', title: '추적 코드'})]);
        fakeApi.decideProposals.mockResolvedValue({done: 1, errors: ['이미 처리한 제안입니다']});
        render(<Shell><DepartmentDecisions project={project}/></Shell>);
        const group = await screen.findByRole('region', {name: /캠페인 기획안 1판 제안/});
        fireEvent.click(within(group).getByRole('button', {name: '모두 거절'}));
        await waitFor(() => expect(fakeApi.decideProposals).toHaveBeenCalledWith('prj-1', ['p1', 'p2'], false));
        expect(await within(group).findByText('이미 처리한 제안입니다')).toBeInTheDocument();
    });

    it('answers a question so the department judges again', async () => {
        fakeApi.judgments.mockResolvedValue([question({})]);
        fakeApi.proposals.mockResolvedValue([]);
        render(<Shell><DepartmentDecisions project={project}/></Shell>);
        const card = await screen.findByRole('article', {name: '재무팀 질문'});
        expect(within(card).getByText('예산 한도는 얼마인가요?')).toBeInTheDocument();
        const send = within(card).getByRole('button', {name: '답하기'});
        expect(send).toBeDisabled();
        fireEvent.change(within(card).getByLabelText('재무팀 질문에 대한 답변'), {target: {value: '500만 원'}});
        fireEvent.click(send);
        await waitFor(() => expect(fakeApi.answerJudgment).toHaveBeenCalledWith('prj-1', 'q1', '500만 원'));
        expect(fakeApi.judgments).toHaveBeenCalledWith('prj-1', 'open,manual');
    });

    it('leaves a repeated question to the person', async () => {
        fakeApi.judgments.mockResolvedValue([question({questionStatus: 'manual', round: 2})]);
        fakeApi.proposals.mockResolvedValue([]);
        render(<Shell><DepartmentDecisions project={project}/></Shell>);
        const card = await screen.findByRole('article', {name: '재무팀 질문'});
        expect(within(card).queryByRole('button', {name: '답하기'})).toBeNull();
        fireEvent.click(within(card).getByRole('button', {name: '닫기'}));
        await waitFor(() => expect(fakeApi.closeJudgmentQuestion).toHaveBeenCalledWith('prj-1', 'q1', ''));
    });
});

describe('inbox decisions', () => {
    it('shows every service’s proposals with the service name', async () => {
        render(<Shell><InboxDecisions data={{questions: [], proposals: [proposal({id: 'p1', projectName: '부동산'})]}} onDone={() => {}}/></Shell>);
        expect(screen.getByRole('region', {name: '부서의 제안과 질문'})).toBeInTheDocument();
        expect(screen.getByText(/부동산 · 자료 공유됨/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '페이지 구현 검토 거절'}));
        fireEvent.change(screen.getByLabelText('거절 이유'), {target: {value: '이미 진행 중'}});
        fireEvent.click(screen.getByRole('button', {name: '거절'}));
        await waitFor(() => expect(fakeApi.rejectProposal).toHaveBeenCalledWith('prj-1', 'p1', '이미 진행 중'));
    });

    it('shows nothing when no department waits', () => {
        const {container} = render(<Shell><InboxDecisions data={{questions: [], proposals: []}} onDone={() => {}}/></Shell>);
        expect(container.querySelector('section')).toBeNull();
    });
});

describe('existing document review', () => {
    it('asks a department to judge documents it can see', async () => {
        fakeApi.deliveries.mockResolvedValue([]);
        const doc = (id: string, title: string, versions: SharedDocument['versions']): SharedDocument => ({
            id, projectId: 'prj-1', title, categories: [], ownerDepartmentId: 'mkt', workItemId: '', scope: 'service', scopeDepartments: [],
            status: 'active', createdAt: '', updatedAt: '', versions,
        });
        const v = (id: string, number: number, withdrawnAt = '') => ({id, documentId: '', projectId: 'prj-1', number, stage: 'draft' as const,
            confirmedBy: '', confirmedAt: '', confirmationNote: '', sourceKind: 'workspace' as const, sourceArtifactId: '', sourcePath: '', fileName: '',
            path: '', hash: '', size: 1, withdrawnAt, createdAt: ''});
        fakeApi.documents.mockResolvedValue([doc('d1', '작년 계약서', [v('v1', 1), v('v2', 2, 'x')])]);
        fakeApi.requestDocumentReview.mockResolvedValue(1);
        render(<Shell><DeliveryInbox project={project} departments={[dept('law', '법무팀')]}/></Shell>);
        fireEvent.click(await screen.findByRole('button', {name: '기존 자료 검토 요청'}));
        const form = screen.getByRole('form', {name: '기존 자료 검토 요청'});
        fireEvent.change(within(form).getByLabelText('검토할 부서'), {target: {value: 'law'}});
        fireEvent.click(await within(form).findByRole('checkbox', {name: /작년 계약서/}));
        expect(within(form).getByText('1판')).toBeInTheDocument(); // the withdrawn 2판 is not offered
        fireEvent.click(within(form).getByRole('button', {name: '검토 요청 (1)'}));
        await waitFor(() => expect(fakeApi.requestDocumentReview).toHaveBeenCalledWith('prj-1', 'law', ['v1'], '기존 자료 검토'));
        expect(fakeApi.documents).toHaveBeenCalledWith('prj-1', {visibleTo: 'law'});
    });
});
