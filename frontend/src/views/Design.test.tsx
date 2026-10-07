import {fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {DesignView} from '../types';
import {Design} from './Design';

const view = (over: Partial<DesignView> = {}): DesignView => ({
    id: 'dsn-1', projectId: '', goal: '게임 출시, 리뷰는 내가', mode: 'review', connectionId: 'conn-1', status: 'ready', error: '',
    createdAt: '', updatedAt: '', applied: null,
    request: {goal: '게임 출시', projectId: '', projectName: '', humanTasks: ['코드 리뷰'], mode: 'review', model: ''},
    autoIssues: [],
    result: {
        canApply: true, canStart: true, issues: [],
        proposal: {project: {name: '작은 웹 게임', goal: '출시', instructions: ''}, notes: '기획 역할 재사용',
            workflow: {schemaVersion: 1, title: '게임 출시', nodes: []}},
        diff: {
            reuseRoles: [{ref: 'planner', id: 'role-plan', name: '기획'}],
            newRoles: [{ref: 'dev', reuseRoleId: '', name: '게임 개발', mission: '', instructions: '', parentRef: ''}],
            roleChanges: [{roleId: 'role-plan', instructions: '게임 지식', reason: '게임 기획', roleName: '기획', current: ''}],
            assignments: [{ref: 'a-owner', roleRef: 'owner', actorKind: 'human', displayName: '나', connectionId: '', model: '', instructions: '',
                roleName: '리뷰어', connectionName: '', connected: true}],
            steps: [{id: 'review', title: '코드 리뷰', kind: 'review', assignee: '나', human: true}],
            missing: [],
        },
    },
    ...over,
});

const renderIt = () => render(<LiveProvider><Design onOpenWorkflow={vi.fn()} onOpenRun={vi.fn()}/></LiveProvider>);

beforeEach(() => {
    vi.clearAllMocks();
    fakeApi.connections.mockResolvedValue([{id: 'conn-1', name: '내 Claude', provider: 'claude', usable: true}]);
});

describe('Design', () => {
    it('sends the goal and my tasks only after an explicit confirmation', async () => {
        fakeApi.startDesign.mockResolvedValue(view({status: 'drafting'}));
        fakeApi.design.mockResolvedValue(view({status: 'drafting'}));
        renderIt();
        const form = await screen.findByRole('form', {name: '목표 요청'});
        fireEvent.change(within(form).getByLabelText('목표'), {target: {value: '게임을 출시하고 싶어'}});
        fireEvent.change(within(form).getByLabelText(/직접 맡을 업무/), {target: {value: '코드 리뷰, 출시 승인\n'}});
        await waitFor(() => expect(within(form).getByRole('button', {name: '설계안 만들기'})).toBeEnabled());
        fireEvent.click(within(form).getByRole('button', {name: '설계안 만들기'}));
        expect(fakeApi.startDesign).not.toHaveBeenCalled();
        fireEvent.click(within(form).getByRole('button', {name: 'AI 사용량을 써서 설계'}));
        await waitFor(() => expect(fakeApi.startDesign).toHaveBeenCalledWith(expect.objectContaining({
            goal: '게임을 출시하고 싶어', humanTasks: ['코드 리뷰', '출시 승인'], mode: 'review', connectionId: 'conn-1',
        })));
    });

    it('shows reuse, new roles, human steps and unapplied role changes', async () => {
        fakeApi.designs.mockResolvedValue([view()]);
        fakeApi.design.mockResolvedValue(view());
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: /게임 출시, 리뷰는 내가/}));
        const section = await screen.findByRole('region', {name: '설계안'}).catch(() => screen.findByLabelText('설계안'));
        await within(section).findByText('기획 · 재사용');
        expect(within(section).getByText('게임 개발 · 새 역할')).toBeInTheDocument();
        expect(within(section).getByText(/자동으로 적용되지 않음/)).toBeInTheDocument();
        expect(within(section).getAllByText('사람').length).toBeGreaterThan(0);
        expect(within(section).queryByRole('button', {name: '적용하고 자동 시작'})).not.toBeInTheDocument();
    });

    it('updates the request list when a draft finishes', async () => {
        fakeApi.designs.mockResolvedValue([view({status: 'drafting'})]);
        fakeApi.design.mockResolvedValueOnce(view({status: 'drafting'})).mockResolvedValue(view());
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: /게임 출시, 리뷰는 내가/}));
        const list = await screen.findByLabelText('설계 요청');
        await within(list).findByText('설계 중');
        fakeApi.designs.mockResolvedValue([view()]);
        await waitFor(() => expect(within(list).getByText('검토 대기')).toBeInTheDocument(), {timeout: 4000});
    });

    it('disables auto start when the scope does not allow it', async () => {
        const auto = view({mode: 'auto', autoIssues: [{code: 'auto_needs_budget', message: '예산을 정해야 합니다', severity: 'run'}]});
        fakeApi.designs.mockResolvedValue([auto]);
        fakeApi.design.mockResolvedValue(auto);
        renderIt();
        fireEvent.click(await screen.findByRole('button', {name: /게임 출시, 리뷰는 내가/}));
        expect(await screen.findByRole('button', {name: '적용하고 자동 시작'})).toBeDisabled();
        expect(screen.getByText(/예산을 정해야 합니다/)).toBeInTheDocument();
        expect(screen.getByRole('button', {name: '적용하고 편집하기'})).toBeEnabled();
    });
});
