import {fireEvent, render, screen, within} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {fakeApi} from '../test/fakeApi';

vi.mock('../api', () => ({api: fakeApi, errorText: (e: unknown) => String(e)}));

import {LiveProvider} from '../live';
import type {DepartmentLoad, Project} from '../types';
import {Operations} from './Operations';

const project: Project = {id: 'prj-1', organizationId: 'org-default', name: '캠페인', goal: '', instructions: '', workspacePath: '',
    budget: {}, mode: 'review', status: 'active', createdAt: '', updatedAt: ''};
const load = (over: Partial<DepartmentLoad>): DepartmentLoad => ({
    departmentId: 'dev', name: '개발팀', status: 'active', open: 0, waiting: 0, blocked: 0, resultsToCheck: 0, proposals: 0, questions: 0,
    collabIncoming: 0, autoReviews: 0, failedDeliveries: 0, heldDeliveries: 0, attention: [], ...over,
});

beforeEach(() => vi.clearAllMocks());

describe('service operations', () => {
    it('shows each department’s work and what waits, and opens blocked work', async () => {
        fakeApi.operations.mockResolvedValue({
            departments: [load({open: 3, blocked: 1, resultsToCheck: 1, proposals: 1, collabIncoming: 1, failedDeliveries: 1,
                attention: [{id: 'w1', title: '랜딩 구현', status: 'blocked', reason: 'API 문서 없음'}]}), load({departmentId: 'fin', name: '재무팀', open: 1})],
            troubles: [{id: 'd1', departmentName: '개발팀', status: 'failed', reason: '판단 결과에 이유가 없습니다', eventTitle: '캠페인 기획안 1판', updatedAt: ''}],
            troubleCount: 1,
            usage: {inputTokens: 0, outputTokens: 0, costUsd: 0.5, unknownCostAttempts: 2, attempts: 5, budget: {}, holdReason: '토큰 예산을 다 썼습니다'},
        });
        const onRoute = vi.fn();
        render(<LiveProvider><Operations project={project} onRoute={onRoute}/></LiveProvider>);
        expect(await screen.findByText('예산 때문에 새 AI 작업을 멈췄습니다')).toBeInTheDocument();
        expect(screen.getByText('비용을 알 수 없는 AI 호출 2회')).toBeInTheDocument();
        expect(screen.getByText(/판단 결과에 이유가 없습니다/)).toBeInTheDocument();
        const table = screen.getByRole('table');
        const dev = within(table).getByText('개발팀').closest('tr')!;
        expect(within(dev).getByText('실패 1 · 보류 0')).toBeInTheDocument();
        expect(within(dev).getByRole('button', {name: '2'})).toBeInTheDocument(); // a proposal and a request
        fireEvent.click(screen.getByRole('button', {name: '랜딩 구현'}));
        expect(onRoute).toHaveBeenCalledWith({view: 'project', projectId: 'prj-1', tab: 'work', workItemId: 'w1'});
    });

    it('shows no status banners when nothing is wrong', async () => {
        fakeApi.operations.mockResolvedValue({departments: [load({open: 1})], troubles: [], troubleCount: 0,
            usage: {inputTokens: 0, outputTokens: 0, costUsd: 0, unknownCostAttempts: 0, attempts: 0, budget: {}}});
        render(<LiveProvider><Operations project={project} onRoute={() => {}}/></LiveProvider>);
        expect(await screen.findByRole('region', {name: '부서별 현황'})).toBeInTheDocument();
        expect(screen.queryByRole('region', {name: '운영 상태'})).not.toBeInTheDocument();
    });
});
