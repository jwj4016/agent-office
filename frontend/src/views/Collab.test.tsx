import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import type {ReactNode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {CollabRequest, Department, Project, RequiredNotice, WorkItem} from '../types';
import {ToastProvider} from '../ui';
import {InboxCollaboration, WorkCollaboration, acting, blockReason} from './Collab';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const dept = (id: string, name: string): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId: '', memberIds: [], templateId: '', automation: {}, limits: {},
    status: 'active', createdAt: '', updatedAt: '',
});
const departments = [dept('mkt', '마케팅팀'), dept('dev', '개발팀'), dept('fin', '재무팀')];

const plan: WorkItem = {
    id: 'w1', projectId: 'prj-1', kind: 'work', parentId: 'g1', departmentId: 'mkt', assigneeAssignmentId: '', title: '캠페인 기획', description: '',
    doneCriteria: '', priority: 'normal', status: 'in_progress', statusReason: '', categories: [], dueAt: '', createdAt: '', updatedAt: '', closedAt: '', reviewBeforeStart: false,
};

const request = (over: Partial<CollabRequest>): CollabRequest => ({
    id: 'c1', projectId: 'prj-1', projectName: '캠페인', fromWorkItemId: 'w1', fromWorkTitle: '캠페인 기획', fromWorkStatus: 'in_progress', goalId: 'g1',
    fromDepartmentId: 'mkt', fromDepartmentName: '마케팅팀', toDepartmentId: 'dev', toDepartmentName: '개발팀', question: '페이지를 검토해 주세요',
    inputVersionIds: [], requirement: 'required', requirementReason: '', status: 'requested', followUp: '', decidedBy: '', decisionJobId: '', decisionReason: '', reviewedAt: '', revertedAt: '', revertReason: '',
    decidedAt: '', resultWorkItemId: '', resultWorkTitle: '', resultWorkStatus: '', replacesId: '', replacedById: '', dueAt: '', eventSequence: 9,
    satisfied: false, overdue: false, history: [{action: 'requested', body: '페이지를 검토해 주세요', asDepartmentId: 'mkt', author: 'local-owner', createdAt: ''}],
    createdAt: '', updatedAt: '', ...over,
});

const Shell = ({children}: { children: ReactNode }) => <LiveProvider><ToastProvider>{children}</ToastProvider></LiveProvider>;

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.workDocuments.mockResolvedValue([]);
});

describe('work collaboration', () => {
    it('shows what keeps the work from completing and decides as the receiving department', async () => {
        const waiting = request({});
        fakeApi.workCollab.mockResolvedValue({requests: [waiting], incoming: [], blockers: [waiting], checks: []});
        fakeApi.acceptCollab.mockResolvedValue(request({status: 'accepted'}));
        render(<Shell><WorkCollaboration project={project} item={plan} departments={departments} onOpenWork={() => {}} onChanged={() => {}}/></Shell>);
        expect(await screen.findByText('완료하려면 필수 협업 1건이 끝나야 합니다')).toBeInTheDocument();
        expect(screen.getByText('개발팀: 수락 대기')).toBeInTheDocument();
        const card = screen.getByRole('article', {name: '마케팅팀 → 개발팀 협업 요청'});
        expect(within(card).getByText('완료에 필요')).toBeInTheDocument();
        expect(within(card).getByRole('button', {name: '개발팀으로서 거절'})).toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: '개발팀으로서 수락'}));
        const form = within(card).getByRole('form', {name: '개발팀으로서 수락'});
        expect(within(form).getByLabelText('업무 제목')).toHaveValue('마케팅팀 협업: 캠페인 기획');
        fireEvent.change(within(form).getByLabelText('업무 제목'), {target: {value: '캠페인 페이지 구현 검토'}});
        fireEvent.click(within(form).getByRole('button', {name: '개발팀으로서 수락'}));
        await waitFor(() => expect(fakeApi.acceptCollab).toHaveBeenCalledWith('prj-1', 'c1',
            expect.objectContaining({title: '캠페인 페이지 구현 검토', existingWorkItemId: ''})));
    });

    it('sends a required request from the work’s department to another', async () => {
        fakeApi.requestCollab.mockResolvedValue(request({toDepartmentName: '재무팀'}));
        render(<Shell><WorkCollaboration project={project} item={plan} departments={departments} onOpenWork={() => {}} onChanged={() => {}}/></Shell>);
        fireEvent.click(await screen.findByRole('button', {name: '다른 부서에 요청'}));
        const form = screen.getByRole('form', {name: '협업 요청'});
        const target = within(form).getByLabelText('받는 부서');
        expect(within(target).queryByRole('option', {name: '마케팅팀'})).toBeNull();
        fireEvent.change(target, {target: {value: 'fin'}});
        fireEvent.change(within(form).getByLabelText(/요청 내용/), {target: {value: '예산 범위를 확인해 주세요'}});
        fireEvent.change(within(form).getByLabelText(/협업 종류/), {target: {value: 'required'}});
        fireEvent.click(within(form).getByRole('button', {name: '마케팅팀으로서 요청 보내기'}));
        await waitFor(() => expect(fakeApi.requestCollab).toHaveBeenCalledWith('prj-1', expect.objectContaining({
            fromWorkItemId: 'w1', toDepartmentId: 'fin', question: '예산 범위를 확인해 주세요', requirement: 'required'})));
    });

    it('needs a reason to release a required request and offers a replacement after a decline', async () => {
        const declined = request({status: 'declined', decisionReason: '인력이 없습니다', decidedBy: 'user'});
        fakeApi.workCollab.mockResolvedValue({requests: [declined], incoming: [], blockers: [declined], checks: []});
        fakeApi.setCollabRequirement.mockResolvedValue(request({requirement: 'reference'}));
        render(<Shell><WorkCollaboration project={project} item={plan} departments={departments} onOpenWork={() => {}} onChanged={() => {}}/></Shell>);
        const card = await screen.findByRole('article', {name: '마케팅팀 → 개발팀 협업 요청'});
        expect(within(card).getByText('거절 이유: 인력이 없습니다')).toBeInTheDocument();
        expect(within(card).queryByRole('button', {name: '개발팀으로서 수락'})).toBeNull();
        expect(within(card).getByRole('button', {name: '다른 요청으로 대체'})).toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: '책임자로서 필수 해제'}));
        const submit = within(card).getByRole('button', {name: '책임자로서 필수 해제'});
        expect(submit).toBeDisabled();
        fireEvent.change(within(card).getByLabelText('필수 조건을 해제하는 이유'), {target: {value: '내부에서 처리'}});
        fireEvent.click(submit);
        await waitFor(() => expect(fakeApi.setCollabRequirement).toHaveBeenCalledWith('prj-1', 'c1', 'reference', '내부에서 처리'));
    });

    it('leads from the helping work back to the request and shows changed required work', async () => {
        const open = vi.fn();
        fakeApi.workCollab.mockResolvedValue({requests: [], blockers: [], incoming: [request({status: 'accepted', resultWorkItemId: 'w9'})],
            checks: [{id: 'imp1', projectId: 'prj-1', workItemId: 'w9', cause: 'required_reopened', refId: 'c1', documentId: '',
                summary: '필수 협업 업무가 다시 열렸습니다.', status: 'open', note: '', createdAt: ''}]});
        render(<Shell><WorkCollaboration project={project} item={{...plan, id: 'w9', departmentId: 'dev'}} departments={departments}
                                         onOpenWork={open} onChanged={() => {}}/></Shell>);
        expect(await screen.findByText(/마케팅팀의 협업 요청으로 맡은 업무/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '요청한 업무 열기'}));
        expect(open).toHaveBeenCalledWith('w1');
        fireEvent.click(screen.getByRole('button', {name: '확인함'}));
        await waitFor(() => expect(fakeApi.closeImpactCheck).toHaveBeenCalledWith('prj-1', 'imp1', 'resolved', ''));
    });

    it('names the department a person acts for with the right particle', () => {
        expect(acting('개발팀')).toBe('개발팀으로서');
        expect(acting('디자인 그룹')).toBe('디자인 그룹으로서');
        expect(acting('재무')).toBe('재무로서');
        expect(acting('법률')).toBe('법률로서');
        expect(acting('QA')).toBe('QA(으)로서');
    });

    it('explains every unmet state the same way the app refuses completion', () => {
        expect(blockReason(request({status: 'question'}))).toBe('개발팀: 받는 부서의 질문에 답해야 함');
        expect(blockReason(request({status: 'cancelled', overdue: true}))).toBe('개발팀: 취소됨, 필수 해제나 대체가 필요, 기한 지남');
        expect(blockReason(request({status: 'accepted', resultWorkTitle: '검토', resultWorkStatus: 'in_progress'}))).toBe('개발팀: 수행 업무 ‘검토’ 미완료');
    });
});

describe('AI collaboration decisions', () => {
    it('lets the person keep or revert what the lead AI decided', async () => {
        const decided = request({requirement: 'reference', status: 'accepted', decidedBy: 'ai', decisionJobId: 'aij-1', decisionReason: '우리 일이다',
            resultWorkItemId: 'w9', resultWorkTitle: '랜딩 검토', resultWorkStatus: 'received',
            history: [{action: 'accepted', body: '새 업무 ‘랜딩 검토’ 등록', asDepartmentId: 'dev', author: 'ai:aij-1', createdAt: ''}]});
        fakeApi.reviewCollabDecision.mockResolvedValue({...decided, reviewedAt: 'now'});
        fakeApi.revertCollabDecision.mockResolvedValue(request({}));
        render(<Shell><InboxCollaboration collabs={[]} notices={[]} aiDecisions={[decided]} onDone={() => {}}/></Shell>);
        const card = screen.getByRole('article', {name: '마케팅팀 → 개발팀 협업 요청'});
        expect(within(card).getByText('AI 결정')).toBeInTheDocument();
        expect(within(card).getByText(/개발팀 대표 AI/)).toBeInTheDocument();
        fireEvent.click(within(card).getByRole('button', {name: 'AI 결정 확인'}));
        await waitFor(() => expect(fakeApi.reviewCollabDecision).toHaveBeenCalledWith('prj-1', 'c1'));
        fireEvent.click(within(card).getByRole('button', {name: 'AI 결정 되돌리기'}));
        const submit = within(card).getByRole('button', {name: '책임자로서 AI 결정 되돌리기'});
        expect(submit).toBeDisabled();
        fireEvent.change(within(card).getByLabelText('되돌리는 이유'), {target: {value: '직접 정하겠다'}});
        fireEvent.click(submit);
        await waitFor(() => expect(fakeApi.revertCollabDecision).toHaveBeenCalledWith('prj-1', 'c1', '직접 정하겠다'));
    });

    it('lists automatic starts that need the person', () => {
        render(<Shell><InboxCollaboration collabs={[]} notices={[]} onDone={() => {}} autoReviews={[{id: 'as-1', projectId: 'prj-1', projectName: '캠페인',
            workItemId: 'w9', workTitle: '랜딩 검토', departmentId: 'dev', causeSequence: 3, status: 'review', planJobId: '', runId: '',
            reason: '예산 때문에 자동으로 시작하지 않았습니다', note: '', createdAt: '', updatedAt: ''}]}/></Shell>);
        expect(screen.getByText(/캠페인 · ‘랜딩 검토’/)).toBeInTheDocument();
        expect(screen.getByText('예산 때문에 자동으로 시작하지 않았습니다')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '직접 진행'}));
        expect(fakeApi.dismissAutoStart).toHaveBeenCalledWith('prj-1', 'as-1', '사람이 직접 진행');
    });
});

describe('inbox collaboration', () => {
    it('lets the asking department answer a question from the inbox', async () => {
        fakeApi.answerCollab.mockResolvedValue(request({}));
        const notice: RequiredNotice = {id: 'imp1', projectId: 'prj-1', workItemId: 'g1', cause: 'required_reopened', refId: 'c1', documentId: '',
            summary: '완료한 목표는 그대로 두었습니다.', status: 'open', note: '', createdAt: '', projectName: '캠페인', workTitle: '신규 고객 캠페인', workKind: 'goal'};
        render(<Shell><InboxCollaboration collabs={[request({status: 'question', followUp: '언제까지 필요한가요?'})]} notices={[notice]} onDone={() => {}}/></Shell>);
        expect(screen.getByRole('region', {name: '부서 간 협업과 자동 처리'})).toBeInTheDocument();
        expect(screen.getByText('캠페인 · 공동 목표 ‘신규 고객 캠페인’')).toBeInTheDocument();
        expect(screen.getByText('언제까지 필요한가요?')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name: '마케팅팀으로서 답하기'}));
        fireEvent.change(screen.getByLabelText('답변'), {target: {value: '다음 주 금요일'}});
        fireEvent.click(screen.getByRole('button', {name: '마케팅팀으로서 답하기'}));
        await waitFor(() => expect(fakeApi.answerCollab).toHaveBeenCalledWith('prj-1', 'c1', '다음 주 금요일'));
    });

    it('shows nothing when no collaboration waits', () => {
        const {container} = render(<Shell><InboxCollaboration collabs={[]} notices={[]} onDone={() => {}}/></Shell>);
        expect(container.querySelector('section')).toBeNull();
    });
});
