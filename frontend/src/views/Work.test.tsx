import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {type ReactNode, useState} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {Project, Department, WorkDetail, WorkFilter, WorkItem} from '../types';
import {DetailProvider} from '../ui';
import {goalProgress, overdue, Work} from './Work';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};

const dept = (id: string, name: string, status: 'active' | 'inactive' = 'active'): Department => ({
    id, projectId: 'prj-1', name, responsibility: '', leadAssignmentId: '', memberIds: [], templateId: '', automation: {}, limits: {},
    status, createdAt: '', updatedAt: '',
});

const work = (over: Partial<WorkItem>): WorkItem => ({
    id: 'w', projectId: 'prj-1', kind: 'work', parentId: '', departmentId: '', assigneeAssignmentId: '', title: '업무', description: '',
    doneCriteria: '', priority: 'normal', status: 'received', statusReason: '', categories: [], dueAt: '', createdAt: '', updatedAt: '', closedAt: '', reviewBeforeStart: false,
    ...over,
});

const goal = work({id: 'g1', kind: 'goal', title: '신규 고객 캠페인 준비', departmentId: 'mkt'});
const items = [
    goal,
    work({id: 'w1', parentId: 'g1', departmentId: 'mkt', title: '기획안 작성', status: 'done', closedAt: 'x'}),
    work({id: 'w2', parentId: 'g1', departmentId: 'dev', title: '페이지 구현 검토', priority: 'high', dueAt: '2000-01-01', categories: ['개발']}),
    work({id: 'w3', parentId: 'g1', departmentId: 'fin', title: '예산 검토', status: 'waiting', statusReason: '기획안 대기'}),
];

function Shell({children}: { children: ReactNode }) {
    const [detail, setDetail] = useState<ReactNode | null>(null);
    return (
        <LiveProvider>
            <DetailProvider value={setDetail}>
                {children}
                {detail ? <aside aria-label="상세">{detail}</aside> : null}
            </DetailProvider>
        </LiveProvider>
    );
}

const onTeam = vi.fn();
const onOpenRun = vi.fn();
const renderIt = (p = project) => render(<Shell><Work project={p} onTeam={onTeam} onOpenRun={onOpenRun}/></Shell>);

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.departments.mockResolvedValue([dept('mkt', '마케팅팀'), dept('dev', '개발팀'), dept('fin', '재무팀')]);
    fakeApi.categories.mockResolvedValue(['기획', '개발', '비용']);
    fakeApi.assignments.mockResolvedValue([]);
    fakeApi.workItems.mockImplementation(async (_p: unknown, f?: unknown) => {
        const filter = (f ?? {}) as WorkFilter;
        return items.filter((w) => (!filter.kind || w.kind === filter.kind)
            && (!filter.departmentId || w.departmentId === filter.departmentId)
            && (filter.status !== 'open' || !['done', 'cancelled'].includes(w.status)));
    });
});

describe('work helpers', () => {
    it('counts a goal’s work and finds overdue open work', () => {
        expect(goalProgress('g1', items)).toEqual({total: 3, done: 1, open: 2});
        expect(overdue(items[2], '2026-10-22')).toBe(true);
        expect(overdue({...items[2], status: 'done'}, '2026-10-22')).toBe(false);
        expect(overdue(items[3], '2026-10-22')).toBe(false);
    });
});

describe('Work', () => {
    it('shows goals with progress and open work with departments and reasons', async () => {
        renderIt();
        const card = await screen.findByRole('button', {name: '신규 고객 캠페인 준비 공동 목표 보기'});
        expect(card).toHaveTextContent('책임: 마케팅팀');
        expect(card).toHaveTextContent('하위 업무 3건 · 완료 1 · 진행 중 2');
        const table = await screen.findByRole('table');
        expect(within(table).getByRole('button', {name: '페이지 구현 검토 업무 보기'})).toBeInTheDocument();
        expect(within(table).queryByText('기획안 작성')).toBeNull(); // done work is not open
        expect(within(table).getByText('대기 · 기획안 대기')).toBeInTheDocument();
        expect(within(table).getByText('2000-01-01 지남')).toBeInTheDocument();
    });

    it('filters by department through the API', async () => {
        renderIt();
        await screen.findByRole('table');
        const group = screen.getByRole('group', {name: '업무 조회 조건'});
        fireEvent.change(within(group).getByLabelText('부서'), {target: {value: 'dev'}});
        await waitFor(() => expect(fakeApi.workItems).toHaveBeenLastCalledWith('prj-1', {status: 'open', departmentId: 'dev', kind: 'work'}));
        await waitFor(() => expect(screen.queryByText('예산 검토')).toBeNull());
        fireEvent.click(within(group).getByRole('button', {name: '조건 초기화'}));
        expect(await screen.findByText('예산 검토')).toBeInTheDocument();
    });

    it('asks for a department before goals can be made', async () => {
        fakeApi.departments.mockResolvedValue([dept('old', '예전 팀', 'inactive')]);
        renderIt();
        expect(await screen.findByText('아직 이 서비스에 부서가 없습니다')).toBeInTheDocument();
        expect(screen.getByRole('button', {name: '공동 목표 추가'})).toBeDisabled();
        fireEvent.click(screen.getByRole('button', {name: '부서 만들기'}));
        expect(onTeam).toHaveBeenCalled();
    });

    it('creates a goal only with a responsible department', async () => {
        fakeApi.createWorkItem.mockResolvedValue({...goal, id: 'g2', title: '봄 행사'});
        fakeApi.workItem.mockResolvedValue({item: {...goal, id: 'g2', title: '봄 행사'}, notes: [], children: []} satisfies WorkDetail);
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: '공동 목표 추가'}));
        const form = await screen.findByRole('form', {name: '공동 목표 편집'});
        fireEvent.change(within(form).getByLabelText('제목'), {target: {value: '봄 행사'}});
        const save = within(form).getByRole('button', {name: '저장'});
        expect(save).toBeDisabled();
        fireEvent.change(within(form).getByLabelText('책임 부서'), {target: {value: 'mkt'}});
        fireEvent.click(save);
        await waitFor(() => expect(fakeApi.createWorkItem).toHaveBeenCalledWith(expect.objectContaining({
            projectId: 'prj-1', kind: 'goal', title: '봄 행사', departmentId: 'mkt', parentId: '',
        })));
        expect(await screen.findByRole('heading', {name: '봄 행사'})).toBeInTheDocument();
    });

    it('opens the work item it was sent to, e.g. from the inbox', async () => {
        fakeApi.workItem.mockResolvedValue({item: items[2], notes: [], children: []} satisfies WorkDetail);
        render(<Shell><Work project={project} workItemId="w2" onTeam={onTeam} onOpenRun={onOpenRun}/></Shell>);
        const pane = await screen.findByRole('complementary', {name: '상세'});
        expect(await within(pane).findByRole('heading', {name: '페이지 구현 검토'})).toBeInTheDocument();
        expect(fakeApi.workItem).toHaveBeenCalledWith('prj-1', 'w2');
    });

    it('says why the work started and which work came of it', async () => {
        fakeApi.workItem.mockResolvedValue({item: items[2], notes: [], children: []} satisfies WorkDetail);
        fakeApi.workLineage.mockResolvedValue({
            causes: [{via: 'proposal', eventSequence: 12, eventKind: 'document.shared', eventTitle: '캠페인 기획안 1판', eventNote: '',
                documentTitle: '캠페인 기획안', fromWorkItemId: 'w1', fromWorkTitle: '기획안 작성', departmentName: '개발팀', reason: '새 페이지가 있다',
                decidedBy: 'user'}],
            next: [{id: 'w3', title: '예산 검토', status: 'waiting', departmentName: '재무팀', via: 'collab'}],
        });
        render(<Shell><Work project={project} workItemId="w2" onTeam={onTeam} onOpenRun={onOpenRun}/></Shell>);
        const lineage = await screen.findByRole('region', {name: '시작한 이유와 이어진 업무'});
        expect(within(lineage).getByText('개발팀 제안으로 등록한 업무')).toBeInTheDocument();
        expect(within(lineage).getByText(/판단 이유: 새 페이지가 있다/)).toBeInTheDocument();
        expect(within(lineage).getByText(/재무팀 · 협업 요청/)).toBeInTheDocument();
        fakeApi.workItem.mockResolvedValue({item: items[1], notes: [], children: []} satisfies WorkDetail);
        fireEvent.click(within(lineage).getByRole('button', {name: '기획안 작성'}));
        expect(await screen.findByRole('heading', {name: '기획안 작성'})).toBeInTheDocument();
    });

    it('needs a reason to block work and keeps the history', async () => {
        const item = items[2];
        fakeApi.workItem.mockResolvedValue({
            item, children: [],
            notes: [{id: 'n1', workItemId: 'w2', kind: 'status', body: '', fromStatus: '', toStatus: 'received', author: 'local-owner', createdAt: ''}],
        } satisfies WorkDetail);
        fakeApi.setWorkStatus.mockResolvedValue({...item, status: 'blocked', statusReason: 'API 문서 없음'});
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: '페이지 구현 검토 업무 보기'}));
        const pane = await screen.findByRole('complementary', {name: '상세'});
        expect(await within(pane).findByText('접수 상태로 등록')).toBeInTheDocument();
        const form = within(pane).getByRole('form', {name: '상태 변경'});
        fireEvent.change(within(form).getByLabelText('상태'), {target: {value: 'blocked'}});
        const submit = within(form).getByRole('button', {name: '상태 변경'});
        expect(submit).toBeDisabled();
        fireEvent.change(within(form).getByLabelText(/이유/), {target: {value: 'API 문서 없음'}});
        fireEvent.click(submit);
        await waitFor(() => expect(fakeApi.setWorkStatus).toHaveBeenCalledWith('prj-1', 'w2', 'blocked', 'API 문서 없음'));
    });
});
